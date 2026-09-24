package web

// Reading a service this program does not own.
//
// Doing so has four concerns that have nothing to do with what is being read:
// not asking twice for an answer that has not changed, not asking faster than
// the service agreed to be asked, asking again when the answer was that
// nobody could answer, and asking once when ten callers want the same answer
// at the same moment. Every program that reads somebody else's service meets
// all four and writes them again.
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

// UpstreamClient reads one service this program does not own, on the terms
// that service is read under.
//
// Named for what it reads rather than for how it behaves: an upstream is by
// definition somebody else's, which is the whole reason the terms exist. The
// plain Client beside it is the transport; this is the transport plus what
// this program owes whoever is being asked.
type UpstreamClient struct {
	client   *Client
	terms    UpstreamTerms
	store    cache.Store
	limiter  rate.Limiter
	inFlight *inFlightReads
}

// UpstreamTerms are what a service is read under: how long its answers are
// worth keeping, how often it may be asked, how long this caller will queue
// for a turn, and how it is asked again when it could not answer.
type UpstreamTerms struct {
	// Name identifies the service in the keys its answers are kept under.
	// Two services read by one program must not share it, or one would be
	// served the other's answer to the same path.
	Name string
	// Allowance is how often this program may ask.
	Allowance rate.Allowance
	// MaxWait is how long this program will queue for a turn before refusing.
	// A request that would wait four minutes is one whose caller has long
	// since gone. Zero waits however long the turn is.
	MaxWait time.Duration
	// TimeToLive is how long one of this service's answers is worth keeping.
	TimeToLive time.Duration
	// Retry is how a service that could not answer is asked again.
	Retry RetryPolicy
}

// IsStated reports whether these terms say enough to read a service by.
func (terms UpstreamTerms) IsStated() bool {
	return terms.Name != "" && terms.Allowance.IsStated() && terms.TimeToLive > 0
}

// RetryPolicy is how a service that could not answer is asked again: a first wait
// that doubles up to a bound, and a number of attempts after the first.
//
// Zero is no asking again, which is the right policy for a service being read
// while somebody waits for the answer.
type RetryPolicy struct {
	Base    time.Duration
	Max     time.Duration
	Retries uint64
}

// IsEnabled reports whether this policy would try again at all.
func (policy RetryPolicy) IsEnabled() bool {
	return policy.Retries > 0 && policy.Base > 0
}

// NewUpstreamClient is a service read under these terms, or a refusal to read
// one whose terms are not stated.
//
// A refusal here rather than at the first request, because a service asked at
// an unstated rate is a service that eventually blocks this program, and that
// is a mistake worth catching where it is made.
func NewUpstreamClient(
	client *Client,
	terms UpstreamTerms,
	store cache.Store,
	limiter rate.Limiter,
) (*UpstreamClient, error) {
	switch {
	case client == nil || client.client == nil:
		return nil, errNoHTTPClient
	case !terms.IsStated():
		return nil, ErrUnstatedTerms
	case store == nil || limiter == nil:
		return nil, ErrUnstatedTerms
	}
	return &UpstreamClient{
		client:   client,
		terms:    terms,
		store:    store,
		limiter:  limiter,
		inFlight: newInFlightReads(),
	}, nil
}

// Name is the service's own name, as the terms gave it.
func (upstream *UpstreamClient) Name() string { return upstream.terms.Name }

// FetchUpstream reads a path: the answer this program already has while it is
// worth keeping, and the service's otherwise.
//
// Fetch with the three concerns applied, and it answers the same way. The
// status is data, and a service that refused after every attempt is a
// ClientResponse carrying that refusal rather than a failure -- so a caller decides what a
// status means about what it asked for, exactly as it does with Fetch.
//
// Only a successful answer is kept. A path that was briefly a 500 must not be
// a 500 for the next hour.
//
// One reading serves every caller that wants the same key at the same moment.
// The keeping is inside that, not around it, so the caller that started the
// reading is not the only one whose answer got kept.
func FetchUpstream[R any](
	upstream *UpstreamClient,
	method string,
	path string,
	request ClientRequest,
) effect.Effect[R, Fault, ClientResponse] {
	key := upstream.cacheKey(method, path, request)
	return effect.Gen(func(do *effect.Do[R, Fault]) ClientResponse {
		lookup := do.Await(lookupCache[R](upstream, key))
		if response, replayed := responseFrom(lookup); replayed {
			return response
		}
		ask := askUpstream[R](upstream, method, path, request).
			FlatMap(writeCache[R](upstream, key, request.About))
		return do.Await(readOnce[R](upstream, key, ask))
	}).WithName("upstream read")
}

// CallUpstream reads an endpoint under the same terms, and reads the answer
// the endpoint declared.
//
// Everything Call says holds here: the method, the path, the status and the
// shape all come from the declaration, and a status it did not declare is a
// Fault carrying a Refusal.
func CallUpstream[R, In, Out any](
	upstream *UpstreamClient,
	endpoint Endpoint[In, Out],
	request ClientRequest,
) effect.Effect[R, Fault, Out] {
	if fault := ValidateEndpoint(endpoint); fault != nil {
		return effect.Fail[R, Out](asFault("call an endpoint", fault))
	}
	path, err := fillPattern(endpoint.segments, request.Path)
	if err != nil {
		return effect.Fail[R, Out](asFault("build the path", err))
	}
	return FetchUpstream[R](upstream, endpoint.method, path, request).
		FlatMap(func(response ClientResponse) effect.Effect[R, Fault, Out] {
			return decodeResponse[R](endpoint.output, response, endpoint.method+" "+path,
				upstream.client.strategy)
		})
}

// askUpstream takes a turn and asks, and asks again under the terms' retry
// policy.
//
// The turn is inside the retry rather than around it, so a second attempt
// waits for its own turn: asking again past the rate a service agreed to is
// how a program that meant to stay within the terms gets itself blocked.
//
// A status worth asking again about is carried as a failure while the retry is
// running and handed back as the ClientResponse it came from once the retries
// are spent, because a schedule retries a failure and this function answers
// with a status. Both are true of the same response.
func askUpstream[R any](
	upstream *UpstreamClient,
	method string,
	path string,
	request ClientRequest,
) effect.Effect[R, Fault, ClientResponse] {
	attempt := effect.Gen(func(do *effect.Do[R, Fault]) ClientResponse {
		do.Await(awaitTurn[R](upstream))
		response := do.Await(Fetch[R](upstream.client, method, path, request))
		return do.Await(failUnsuccessful[R](method + " " + path)(response))
	})
	return attempt.
		Retry(retrySchedule(upstream.terms.Retry)).
		CatchAll(recoverRefusal[R])
}

// awaitTurn is this program's turn to ask.
//
// A pace this program cannot consult is a request this program does not make,
// and that asymmetry with the cache is deliberate: an unreadable cache costs
// latency, and an unenforced rate limit costs a service's goodwill and this
// program its access.
func awaitTurn[R any](upstream *UpstreamClient) effect.Effect[R, Fault, effect.Unit] {
	return rate.AwaitTurn[R](upstream.limiter, upstream.terms.Allowance, upstream.terms.MaxWait).
		MapError(func(fault rate.Fault) Fault {
			return Fault{Op: "wait for a turn at " + upstream.terms.Name, Err: fault}
		})
}

// retrySchedule is the terms' retry policy as a schedule over what went wrong.
func retrySchedule(policy RetryPolicy) effect.Schedule[Fault, effect.Unit] {
	if !policy.IsEnabled() {
		return effect.Stop[Fault]()
	}
	return effect.IntersectSchedules(
		effect.Exponential[Fault](policy.Base, policy.Max),
		effect.Recurs[Fault](policy.Retries),
	).
		MapOutput(func(effect.Product[time.Duration, uint64]) effect.Unit { return effect.Unit{} }).
		WhileInput(isRetryable)
}

// isRetryable reports whether what went wrong might not go wrong again.
//
// A service that could not be reached, and a service that answered that it
// could not answer. Every other status is an answer: it will be the same
// answer next time, and asking again would spend an allowance to be told it
// twice.
func isRetryable(fault Fault) bool {
	var refusal Refusal
	if !errors.As(fault, &refusal) {
		return true
	}
	return refusal.Status >= http.StatusInternalServerError
}
