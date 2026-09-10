package acceptance

// Whether a refusal that became a status left anything behind.
//
// It did not, and that was the single worst thing about debugging a program
// built on this: a typed failure turned into a 404 or a 400 and vanished, so
// the only account of why a client got one was the client's, and anybody
// asking why had to reproduce it.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

// recorded is where a boundary's reports go, kept so a test can read them.
type recorded struct {
	mutex sync.Mutex
	said  []string
}

func (sink *recorded) note(_ context.Context, err error) {
	sink.mutex.Lock()
	defer sink.mutex.Unlock()
	sink.said = append(sink.said, err.Error())
}

func (sink *recorded) all() []string {
	sink.mutex.Lock()
	defer sink.mutex.Unlock()
	return append([]string(nil), sink.said...)
}

// surfaceThatRefuses is a surface whose one route refuses, and the sink its boundary
// reports to.
func surfaceThatRefuses(t *testing.T, quiet bool) (string, *recorded) {
	t.Helper()
	runtime, err := effect.NewRuntime()
	if err != nil {
		t.Fatal(err)
	}
	sink := &recorded{}
	boundary, err := web.NewAdapter(runtime, effect.Unit{},
		func(refused error) web.Response {
			return web.Text(http.StatusNotFound, refused.Error())
		})
	if err != nil {
		t.Fatal(err)
	}
	boundary = boundary.WithReport(sink.note)
	if quiet {
		boundary = boundary.Quietly()
	}

	route := web.Handle(
		web.GET("/thing", web.Nothing(), web.ReturnsNothing(http.StatusOK)).
			Summary("Read a thing"),
		func(effect.Unit) effect.Effect[effect.Unit, error, effect.Unit] {
			return effect.Fail[effect.Unit, effect.Unit](errors.New("no such thing"))
		},
	)
	surface, err := web.NewRoutes(route)
	if err != nil {
		t.Fatal(err)
	}
	// Detailing so each phase of a request is a named span, which is what
	// puts something other than a line in the record. A surface that also
	// names its requests -- inspect.Observing -- puts the route there
	// instead, which is better still and is a deployment's choice.
	front := httptest.NewServer(boundary.Handler(surface.Detailing().Handler()))
	t.Cleanup(front.Close)
	return front.URL, sink
}

func TestARefusalThatBecameAStatusIsRecorded(t *testing.T) {
	address, sink := surfaceThatRefuses(t, false)

	response, err := http.Get(address + "/thing")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("expected the refusal answered as a status, got %d", response.StatusCode)
	}

	said := sink.all()
	if len(said) != 1 {
		t.Fatalf("expected the refusal recorded once, got %v", said)
	}
	// What it says, and the two things that make recording it worth doing:
	// the line that raised it and the span it was raised inside.
	if !strings.Contains(said[0], "no such thing") {
		t.Errorf("expected the refusal itself, got %q", said[0])
	}
	if !strings.Contains(said[0], "refusal_recorded_e2e_test.go:") {
		t.Errorf("expected the line that raised it, got %q", said[0])
	}
	if !strings.Contains(said[0], "handling") {
		t.Errorf("expected the phase it was raised inside, got %q", said[0])
	}
}

func TestABoundaryToldToBeQuietRecordsNothing(t *testing.T) {
	// For a surface where refusing is the ordinary case and the volume would
	// bury everything else. Off by default, because a refusal nobody recorded
	// is a question nobody can answer afterwards.
	address, sink := surfaceThatRefuses(t, true)

	response, err := http.Get(address + "/thing")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()

	if said := sink.all(); len(said) != 0 {
		t.Fatalf("expected nothing recorded, got %v", said)
	}
}
