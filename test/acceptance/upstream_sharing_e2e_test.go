package acceptance

// Ten callers wanting the same answer at the same moment ask once.
//
// The concern neither keeping nor pacing covers. The answer is not in the
// cache -- that is why they are all asking -- and each of them is entitled to
// a turn, so the terms are satisfied while the service is asked ten times for
// one thing.

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
	"github.com/mbauer83/effect-golang/effect/cache"
	"github.com/mbauer83/effect-golang/effect/rate"
)

// heldOpen is a service that will not answer until it is told to, counting
// what it was asked under a lock because the whole point is to be asked
// concurrently.
type heldOpen struct {
	server  *httptest.Server
	mutex   sync.Mutex
	asked   int
	arrived chan struct{}
	release chan struct{}
}

func waitingToAnswer(t *testing.T, entity string) *heldOpen {
	t.Helper()
	service := &heldOpen{
		arrived: make(chan struct{}, 64),
		release: make(chan struct{}),
	}
	service.server = httptest.NewServer(http.HandlerFunc(
		func(writer http.ResponseWriter, _ *http.Request) {
			service.mutex.Lock()
			service.asked++
			service.mutex.Unlock()
			service.arrived <- struct{}{}
			<-service.release
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusOK)
			_, _ = writer.Write([]byte(entity))
		}))
	t.Cleanup(service.server.Close)
	return service
}

func (service *heldOpen) timesAsked() int {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	return service.asked
}

// readingFrom is the held-open service under terms generous enough that the
// pacing is not what this test is measuring.
func readingFrom(t *testing.T, service *heldOpen) (*web.UpstreamClient, cache.Store) {
	t.Helper()
	keeping := cache.NewMemoryStore(64, time.Now)
	upstream, err := web.NewUpstreamClient(
		web.Dial(http.DefaultClient, service.server.URL),
		web.UpstreamTerms{
			Name:       "a service",
			Allowance:  rate.Allowance{Name: "a service", Most: 100, Every: time.Second},
			TimeToLive: time.Minute,
		},
		keeping,
		rate.NewMemoryLimiter(time.Now),
	)
	if err != nil {
		t.Fatal(err)
	}
	return upstream, keeping
}

func TestTenCallersWantingOneAnswerAskOnce(t *testing.T) {
	service := waitingToAnswer(t, `{"title":"Heat"}`)
	upstream, _ := readingFrom(t, service)
	const callers = 10

	// Every caller asks for the same key while none of them has an answer.
	together := effect.ForEachPar(make([]int, callers), func(int) effect.Effect[effect.Unit, web.Fault, web.ClientResponse] {
		return web.FetchUpstream[effect.Unit](upstream, http.MethodGet, "/film/603", aboutOne())
	})
	waiting := make(chan effect.Exit[web.Fault, []web.ClientResponse], 1)
	go func() { waiting <- called(t, together) }()

	// One of them reaches the service. Nine are waiting on that one.
	<-service.arrived
	close(service.release)

	exit := <-waiting
	answers, succeeded := exit.Value()
	if !succeeded {
		t.Fatalf("unexpected exit: %+v", exit)
	}
	if len(answers) != callers {
		t.Fatalf("expected every caller answered, got %d of %d", len(answers), callers)
	}
	for at, answer := range answers {
		if string(answer.Entity) != `{"title":"Heat"}` {
			t.Fatalf("caller %d got %q", at, answer.Entity)
		}
	}
	if asked := service.timesAsked(); asked != 1 {
		t.Fatalf("expected the service asked once for one answer, asked %d times", asked)
	}
}

func TestACallerWhoGoesAwayDoesNotTakeTheReadingWithThem(t *testing.T) {
	// The reason the reading is forked detached. It has taken a turn at
	// somebody else's service and other callers are waiting on it, so tying it
	// to whoever happened to ask first would mean one abandoned request
	// interrupting the rest -- and would throw away a turn already spent.
	service := waitingToAnswer(t, `{"title":"Heat"}`)
	upstream, keeping := readingFrom(t, service)

	// A caller that starts the reading and is then interrupted while waiting.
	abandoned := effect.Race(
		web.FetchUpstream[effect.Unit](upstream, http.MethodGet, "/film/603", aboutOne()),
		givenUpOn(t),
	)
	go func() { _ = called(t, abandoned) }()
	<-service.arrived

	// Its caller is gone by the time the service answers.
	close(service.release)

	// The answer still arrives and is still kept, so the next caller pays
	// nothing -- and the service was asked exactly once for the whole affair.
	keptWithin(t, keeping, 2*time.Second)
	if asked := service.timesAsked(); asked != 1 {
		t.Fatalf("expected the abandoned reading to have finished its one ask, asked %d times", asked)
	}
}

// givenUpOn is a caller giving up, which is what wins the race above.
func givenUpOn(t *testing.T) effect.Effect[effect.Unit, web.Fault, web.ClientResponse] {
	t.Helper()
	return effect.Sleep[effect.Unit, web.Fault](20 * time.Millisecond).
		As(web.ClientResponse{})
}

// keptWithin waits for the store to hold the answer, which is what says the
// detached reading ran to completion.
func keptWithin(t *testing.T, keeping cache.Store, within time.Duration) {
	t.Helper()
	giveUp := time.Now().Add(within)
	for time.Now().Before(giveUp) {
		exit := called(t, effect.Fold(
			cache.Read[effect.Unit](keeping, "a service:film:603:GET /film/603").
				MapError(func(cache.Fault) web.Fault { return web.Fault{} }),
			func(effect.Cause[web.Fault]) bool { return false },
			func(cached cache.Lookup) bool { return cached.Found },
		).MapError(func(effect.Never) web.Fault { return web.Fault{} }))
		if found, ok := exit.Value(); ok && found {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected the detached reading to have kept its answer")
}
