package web

// A client that is careful with somebody else's service.
//
// Reading a service this program does not own has three concerns that have
// nothing to do with what is being read: not asking twice for an answer that
// has not changed, not asking faster than the service agreed to be asked, and
// asking again when the answer was that nobody could answer. Every program
// that reads one meets all three and writes them again.
//
// So they are here, beside Fetch and Call, over the two ports in the runtime
// that already say what keeping and pacing are. Both are ports because both
// are shared: two instances of a program that each kept their own count would
// together ask at twice the rate one of them agreed to. In one process the
// runtime's own adapters are the right answer; between processes something
// else is, and neither this file nor a caller's code changes to say so.
//
// What is not here is what a status means. Fetch treats a status as data and
// so does this: a 404 is an absence to one caller and a failure to another,
// and that is a decision for whoever knows what was being asked for.

import (
	"errors"
	"net/http"
	"time"

	"github.com/mbauer83/effect-golang/effect"
	"github.com/mbauer83/effect-golang/effect/cache"
	"github.com/mbauer83/effect-golang/effect/rate"
)

// Careful is a client read under terms.
type Careful struct {
	client  *Client
	terms   Terms
	keeping cache.Store
	pacing  rate.Limiter
}

// Terms are what a service is read under.
type Terms struct {
	// Named identifies the service in the keys its answers are kept under.
	// Two services read by one program must not share it, or one would be
	// served the other's answer to the same path.
	Named string
	// Allowed is how often this program may ask.
	Allowed rate.Allowance
	// Longest is how long this program will queue for a turn before refusing.
	// A request that would wait four minutes is one whose caller has long
	// since gone. Zero waits however long the turn is.
	Longest time.Duration
	// Fresh is how long one of this service's answers is worth keeping.
	Fresh time.Duration
	// Patience is how a service that could not answer is asked again.
	Patience Patience
}

// IsStated reports whether these terms say enough to read a service by.
func (terms Terms) IsStated() bool {
	return terms.Named != "" && terms.Allowed.IsStated() && terms.Fresh > 0
}

// Patience is how a service that could not answer is asked again: a first wait
// that doubles up to a bound, and a number of attempts after the first.
//
// Zero is no asking again, which is the right policy for a service being read
// while somebody waits for the answer.
type Patience struct {
	First   time.Duration
	Longest time.Duration
	Retries uint64
}

// IsPatient reports whether this policy would try again at all.
func (patience Patience) IsPatient() bool {
	return patience.Retries > 0 && patience.First > 0
}

// Carefully is a client read under terms, or a refusal to read one whose terms
// are not stated.
//
// A refusal here rather than at the first request, because a service asked at
// an unstated rate is a service that eventually blocks this program, and that
// is a mistake worth catching where it is made.
func Carefully(client *Client, terms Terms, keeping cache.Store, pacing rate.Limiter) (*Careful, error) {
	switch {
	case client == nil || client.client == nil:
		return nil, errNoHTTPClient
	case !terms.IsStated():
		return nil, ErrUnstatedTerms
	case keeping == nil || pacing == nil:
		return nil, ErrUnstatedTerms
	}
	return &Careful{client: client, terms: terms, keeping: keeping, pacing: pacing}, nil
}

// Named is the service's own name, as the terms gave it.
func (careful *Careful) Named() string { return careful.terms.Named }

// FetchCarefully reads a path: the answer this program already has while it is
// worth keeping, and the service's otherwise.
//
// Fetch with the three concerns applied, and it answers the same way. The
// status is data, and a service that refused after every attempt is a Received
// carrying that refusal rather than a failure -- so a caller decides what a
// status means about what it asked for, exactly as it does with Fetch.
//
// Only a successful answer is kept. A path that was briefly a 500 must not be
// a 500 for the next hour.
func FetchCarefully[R any](
	careful *Careful,
	method string,
	path string,
	requesting Requesting,
) effect.Effect[R, Fault, Received] {
	filed := careful.filed(method, path, requesting)
	return recalling[R](careful, filed).
		FlatMap(func(kept cache.Kept) effect.Effect[R, Fault, Received] {
			if received, replayed := replayed(kept); replayed {
				return effect.Succeed[R, Fault](received)
			}
			return asking[R](careful, method, path, requesting).
				FlatMap(filing[R](careful, filed, requesting.About))
		}).
		Named("fetch carefully")
}

// CallCarefully reads an endpoint under the same terms, and reads the answer
// the endpoint declared.
//
// Everything Call says holds here: the method, the path, the status and the
// shape all come from the declaration, and a status it did not declare is a
// Fault carrying a Refusal.
func CallCarefully[R, In, Out any](
	careful *Careful,
	endpoint Endpoint[In, Out],
	requesting Requesting,
) effect.Effect[R, Fault, Out] {
	if fault := ValidateEndpoint(endpoint); fault != nil {
		return effect.Fail[R, Out](asFault("calling an endpoint", fault))
	}
	path, err := fillPattern(endpoint.segments, requesting.Path)
	if err != nil {
		return effect.Fail[R, Out](asFault("building the path", err))
	}
	return FetchCarefully[R](careful, endpoint.method, path, requesting).
		FlatMap(func(received Received) effect.Effect[R, Fault, Out] {
			return answered[R](endpoint.output, received, endpoint.method+" "+path)
		})
}

// asking takes a turn and asks, again on the terms' patience.
//
// The turn is inside the retry rather than around it, so a second attempt
// waits for its own turn: asking again past the rate a service agreed to is
// how a program that meant to be careful gets itself blocked.
//
// A status worth asking again about is carried as a failure while the retry is
// running and handed back as the Received it came from once the patience is
// spent, because a schedule retries a failure and this function answers with a
// status. Both are true of the same response.
func asking[R any](
	careful *Careful,
	method string,
	path string,
	requesting Requesting,
) effect.Effect[R, Fault, Received] {
	return waiting[R](careful).
		FlatMap(func(effect.Unit) effect.Effect[R, Fault, Received] {
			return Fetch[R](careful.client, method, path, requesting).
				FlatMap(refusing[R](method + " " + path))
		}).
		Retry(retrying(careful.terms.Patience)).
		CatchAll(recovered[R])
}

// waiting is this program's turn to ask.
//
// A pace this program cannot consult is a request this program does not make,
// and that asymmetry with the cache is deliberate: an unreadable cache costs
// latency, and an unenforced rate limit costs a service's goodwill and this
// program its access.
func waiting[R any](careful *Careful) effect.Effect[R, Fault, effect.Unit] {
	return rate.Waiting[R](careful.pacing, careful.terms.Allowed, careful.terms.Longest).
		MapError(func(failed rate.Fault) Fault {
			return Fault{Doing: "waiting for a turn at " + careful.terms.Named, Err: failed}
		})
}

// retrying is the terms' patience as a schedule over what went wrong.
func retrying(patience Patience) effect.Schedule[Fault, effect.Unit] {
	if !patience.IsPatient() {
		return effect.Stop[Fault]()
	}
	return effect.AndSchedules(
		effect.Exponential[Fault](patience.First, patience.Longest),
		effect.Recurs[Fault](patience.Retries),
	).
		MapOutput(func(effect.Product[time.Duration, uint64]) effect.Unit { return effect.Unit{} }).
		WhileInput(worthAskingAgain)
}

// worthAskingAgain reports whether what went wrong might not go wrong again.
//
// A service that could not be reached, and a service that answered that it
// could not answer. Every other status is an answer: it will be the same
// answer next time, and asking again would spend an allowance to be told it
// twice.
func worthAskingAgain(failed Fault) bool {
	var refusal Refusal
	if !errors.As(failed, &refusal) {
		return true
	}
	return refusal.Status >= http.StatusInternalServerError
}
