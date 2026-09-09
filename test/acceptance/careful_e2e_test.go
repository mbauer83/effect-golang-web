package acceptance

// A client that is careful with somebody else's service.
//
// Against the in-process adapters in the runtime, which is the point of them:
// what is being stated here is that an answer already held is not asked for
// again, that a turn is waited for, that a refusal is not kept and that a
// service which could not answer is asked again -- and none of that needs a
// server to keep the answers in.

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
	"github.com/mbauer83/effect-golang/effect/cache"
	"github.com/mbauer83/effect-golang/effect/rate"
)

// serving is a service this test owns, counting what it was asked.
type asked struct {
	server *httptest.Server
	asked  int
	status int
	entity string
}

func answers(t *testing.T, status int, entity string) *asked {
	t.Helper()
	service := &asked{status: status, entity: entity}
	service.server = httptest.NewServer(http.HandlerFunc(
		func(writer http.ResponseWriter, _ *http.Request) {
			service.asked++
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(service.status)
			_, _ = writer.Write([]byte(service.entity))
		}))
	t.Cleanup(service.server.Close)
	return service
}

// carefully is the service under stated terms, keeping and pacing in this
// process.
func carefully(t *testing.T, service *asked, patience web.Patience) (*web.Careful, cache.Store) {
	t.Helper()
	keeping := cache.Holding(64, time.Now)
	careful, err := web.Carefully(
		web.Dial(http.DefaultClient, service.server.URL),
		web.Terms{
			Named:    "a service",
			Allowed:  rate.Allowance{Name: "a service", Most: 10, Every: time.Second},
			Fresh:    time.Minute,
			Patience: patience,
		},
		keeping,
		rate.Holding(time.Now),
	)
	if err != nil {
		t.Fatal(err)
	}
	return careful, keeping
}

func fetched(t *testing.T, careful *web.Careful, requesting web.Requesting) web.Received {
	t.Helper()
	exit := called(t, web.FetchCarefully[effect.Unit](careful, http.MethodGet, "/film/603", requesting))
	received, ok := exit.Value()
	if !ok {
		t.Fatalf("unexpected exit: %+v", exit)
	}
	return received
}

func aboutOne() web.Requesting {
	return web.Requesting{About: "film:603"}
}

func TestAnAnswerAlreadyHeldIsNotAskedForAgain(t *testing.T) {
	service := answers(t, http.StatusOK, `{"said":"once"}`)
	careful, _ := carefully(t, service, web.Patience{})

	first := fetched(t, careful, aboutOne())
	second := fetched(t, careful, aboutOne())

	if string(first.Entity) != string(second.Entity) {
		t.Fatalf("expected the same answer twice, got %q then %q", first.Entity, second.Entity)
	}
	if service.asked != 1 {
		t.Fatalf("expected the service asked once, got %d", service.asked)
	}
	// The whole response is kept, not just the body: a caller that read the
	// content type the first time reads it the second.
	if second.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("expected the headers kept with it, got %v", second.Header)
	}
	if second.Status != http.StatusOK {
		t.Fatalf("expected the status kept with it, got %d", second.Status)
	}
}

func TestTwoQuestionsAreTwoAnswers(t *testing.T) {
	// The request is part of what an answer is kept under, so a search for one
	// thing cannot be served the answer for another.
	service := answers(t, http.StatusOK, `{}`)
	careful, _ := carefully(t, service, web.Patience{})

	for _, words := range []string{"alien", "aliens"} {
		_ = fetched(t, careful, web.Requesting{
			About: "search", Query: url.Values{"q": {words}},
		})
	}

	if service.asked != 2 {
		t.Fatalf("expected two questions asked, got %d", service.asked)
	}
}

func TestEverythingKeptAboutOneThingIsDroppedAtOnce(t *testing.T) {
	// What somebody asking for a thing to be looked up again means, and the
	// reason a request says what it is about.
	service := answers(t, http.StatusOK, `{}`)
	careful, keeping := carefully(t, service, web.Patience{})
	_ = fetched(t, careful, aboutOne())

	if _, ok := called(t, cache.Drop[effect.Unit](keeping, "film:603").
		MapError(func(cache.Fault) web.Fault { return web.Fault{} })).Value(); !ok {
		t.Fatal("expected what was kept about it to be dropped")
	}
	_ = fetched(t, careful, aboutOne())

	if service.asked != 2 {
		t.Fatalf("expected the service asked again, got %d", service.asked)
	}
}

func TestARefusalIsAnAnswerAndIsNotKept(t *testing.T) {
	// The status is data, exactly as it is for Fetch: a 404 is an absence to
	// one caller and a failure to another. And a path that was briefly a 404
	// must not be a 404 for the next minute.
	service := answers(t, http.StatusNotFound, `{"why":"no such film"}`)
	careful, _ := carefully(t, service, web.Patience{})

	received := fetched(t, careful, aboutOne())
	if received.Status != http.StatusNotFound {
		t.Fatalf("expected the status as data, got %d", received.Status)
	}
	if string(received.Entity) != `{"why":"no such film"}` {
		t.Fatalf("expected the body it refused with, got %q", received.Entity)
	}

	_ = fetched(t, careful, aboutOne())
	if service.asked != 2 {
		t.Fatalf("expected the refusal not to have been kept, asked %d times", service.asked)
	}
}

func TestAServiceThatCouldNotAnswerIsAskedAgain(t *testing.T) {
	service := answers(t, http.StatusInternalServerError, `{}`)
	careful, _ := carefully(t, service, web.Patience{
		First: time.Millisecond, Longest: 5 * time.Millisecond, Retries: 2,
	})

	received := fetched(t, careful, aboutOne())

	// One attempt and two more, and then the status handed back as the answer
	// it is rather than as a failure.
	if service.asked != 3 {
		t.Fatalf("expected three attempts, got %d", service.asked)
	}
	if received.Status != http.StatusInternalServerError {
		t.Fatalf("expected the last status answered with, got %d", received.Status)
	}
}

func TestAnAnswerIsNotAskedForTwice(t *testing.T) {
	// Every other status is an answer: it will be the same answer next time,
	// and asking again would spend an allowance to be told it twice.
	service := answers(t, http.StatusTooManyRequests, `{}`)
	careful, _ := carefully(t, service, web.Patience{
		First: time.Millisecond, Longest: 5 * time.Millisecond, Retries: 2,
	})

	_ = fetched(t, careful, aboutOne())

	if service.asked != 1 {
		t.Fatalf("expected one attempt, got %d", service.asked)
	}
}

func TestATurnIsWaitedForBeforeAsking(t *testing.T) {
	// Three every three hundred milliseconds: the burst goes at once and the
	// fourth waits a spacing.
	service := answers(t, http.StatusOK, `{}`)
	careful, err := web.Carefully(
		web.Dial(http.DefaultClient, service.server.URL),
		web.Terms{
			Named:   "a slow service",
			Allowed: rate.Allowance{Name: "a slow service", Most: 3, Every: 300 * time.Millisecond},
			Fresh:   time.Minute,
		},
		cache.Holding(64, time.Now),
		rate.Holding(time.Now),
	)
	if err != nil {
		t.Fatal(err)
	}

	started := time.Now()
	for turn := range 4 {
		_ = fetched(t, careful, web.Requesting{
			About: "film:603", Query: url.Values{"turn": {string(rune('a' + turn))}},
		})
	}
	took := time.Since(started)

	if took < 90*time.Millisecond {
		t.Fatalf("expected the fourth turn to be waited for, took %v", took)
	}
}

func TestTermsThatSayNothingAreRefusedWhereTheyAreWritten(t *testing.T) {
	// A service asked at an unstated rate is one that eventually blocks this
	// program, and that is a mistake worth catching where it is made rather
	// than at the first request.
	service := answers(t, http.StatusOK, `{}`)
	client := web.Dial(http.DefaultClient, service.server.URL)

	for _, terms := range []web.Terms{
		{Named: "a service", Fresh: time.Minute},
		{Allowed: rate.Allowance{Name: "a service", Most: 1, Every: time.Second}, Fresh: time.Minute},
		{Named: "a service", Allowed: rate.Allowance{Name: "a service", Most: 1, Every: time.Second}},
	} {
		if _, err := web.Carefully(client, terms, cache.Holding(8, time.Now), rate.Holding(time.Now)); err == nil {
			t.Fatalf("expected %+v to be refused", terms)
		}
	}
}
