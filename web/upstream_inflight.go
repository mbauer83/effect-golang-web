package web

// One upstream reading for however many callers want it.
//
// The concern the other three did not cover. Caching stops this program asking
// twice for an answer it already has, and pacing stops it asking faster than
// agreed -- but neither says anything about ten callers who all want the same
// answer at the same moment, before any of them has one to cache. Each takes a
// turn, each asks, nine of them throw their answer away, and the service that
// agreed to be asked forty times a minute was asked ten times for one thing.
//
// A shared cache does not help: the answer is not in it yet, which is why they
// are all asking. What is needed is for the first caller to ask and the rest to
// wait for that answer, and that is what this is.
//
// In this process rather than between them, deliberately. A shared registry of
// what is in flight would need a lock somebody else's crashed instance could
// be holding, and would trade a bounded waste -- one request per instance --
// for an unbounded failure. Four instances asking once each is the honest cost
// of running four, and the shared cache means the second instance to want an
// answer usually finds it already kept.

import (
	"sync"

	"github.com/mbauer83/effect-golang/effect"
)

// inFlightReads is what each key's waiting callers are waiting on: the
// reading that was started for them, whatever its outcome turns out to be.
type inFlightReads struct {
	mutex   sync.Mutex
	pending map[string]effect.Deferred[Fault, ClientResponse]
}

func newInFlightReads() *inFlightReads {
	return &inFlightReads{pending: map[string]effect.Deferred[Fault, ClientResponse]{}}
}

// readOnce is one reading of a key, however many callers ask for it.
//
// Every caller awaits; none of them performs the reading. The reading itself
// is forked detached, which is what makes this safe rather than merely
// cheaper:
//
// A caller who goes away must not take the reading with them. It has taken a
// turn at somebody else's service and every other caller is waiting on it, so
// tying it to the first caller's lifetime would mean one abandoned request
// interrupting nine live ones -- and would throw away a turn already spent.
// Detached, an abandoned caller simply stops awaiting, and the answer still
// arrives and is still kept for whoever asks next.
//
// It is not an orphan: detached work is owned by the runtime, so closing the
// runtime interrupts it like anything else.
func readOnce[R any](
	upstream *UpstreamClient,
	key string,
	read effect.Effect[R, Fault, ClientResponse],
) effect.Effect[R, Fault, ClientResponse] {
	operations := effect.For[R, Fault]()
	return operations.WidenError(joinOrStart[R](upstream, key)).
		FlatMap(func(ticket claim) effect.Effect[R, Fault, ClientResponse] {
			if !ticket.leader {
				return ticket.answer.Await[R]()
			}
			return forkRead[R](upstream, key, ticket.answer, read)
		})
}

// claim is the deferred a caller will await, and whether this caller is the one
// that has to start the reading it will be fulfilled by.
type claim struct {
	answer effect.Deferred[Fault, ClientResponse]
	leader bool
}

// joinOrStart is the whole of the coordination: under one lock, either join the
// reading already under way for this key or become the one that starts it.
//
// Creating the deferred is an effect, so it is created before the lock is
// taken and discarded if somebody else got there first. A discarded
// unfulfilled deferred is a value nobody holds and costs nothing.
func joinOrStart[R any](upstream *UpstreamClient, key string) effect.Effect[R, effect.Never, claim] {
	return effect.NewDeferred[R, Fault, ClientResponse]().
		Map(func(fresh effect.Deferred[Fault, ClientResponse]) claim {
			upstream.inFlight.mutex.Lock()
			defer upstream.inFlight.mutex.Unlock()
			if already, waiting := upstream.inFlight.pending[key]; waiting {
				return claim{answer: already}
			}
			upstream.inFlight.pending[key] = fresh
			return claim{answer: fresh, leader: true}
		})
}

// forkRead forks the reading and awaits it like everybody else.
//
// The outcome is handed to the deferred whole, including a defect or an
// interruption, so a reading that died in a way nobody anticipated is a
// failure the waiters see rather than a wait that never ends. The key is
// released in the same step: it is released on every outcome, because a key
// left behind by a failed reading would be a key nobody ever asks for again.
func forkRead[R any](
	upstream *UpstreamClient,
	key string,
	answer effect.Deferred[Fault, ClientResponse],
	read effect.Effect[R, Fault, ClientResponse],
) effect.Effect[R, Fault, ClientResponse] {
	operations := effect.For[R, Fault]()
	return operations.WidenError(effect.ForkDaemon[R, Fault, ClientResponse](
		read.OnExit(func(outcome effect.Exit[Fault, ClientResponse]) effect.Effect[R, effect.Never, effect.Unit] {
			return forgetKey[R](upstream, key).
				FlatMap(func(effect.Unit) effect.Effect[R, effect.Never, effect.Unit] {
					return answer.Complete[R](outcome).As(effect.Unit{})
				})
		}),
	)).
		FlatMap(func(effect.Fiber[Fault, ClientResponse]) effect.Effect[R, Fault, ClientResponse] {
			return answer.Await[R]()
		})
}

// forgetKey forgets the key, so the next caller starts a new reading rather
// than awaiting one that is over.
func forgetKey[R any](upstream *UpstreamClient, key string) effect.Effect[R, effect.Never, effect.Unit] {
	return effect.Succeed[R, effect.Never](effect.Unit{}).
		Map(func(effect.Unit) effect.Unit {
			upstream.inFlight.mutex.Lock()
			defer upstream.inFlight.mutex.Unlock()
			delete(upstream.inFlight.pending, key)
			return effect.Unit{}
		})
}
