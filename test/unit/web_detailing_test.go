package unit

// Naming the parts of a route's own work.
//
// Decoding and encoding are the route's work as much as the handler is, and a
// trace that showed one bar for all three could not say which of them a slow
// request spent its time in. A separate setting from Wrapping, because they
// cost differently.

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mbauer83/effect-golang-schema/schema"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
	"github.com/mbauer83/effect-golang/effecttest"
)

func TestDetailingNamesTheRoutesOwnPartsAndNothingElseDoes(t *testing.T) {
	// Decoding and encoding are the route's work as much as the handler is,
	// and a trace that showed one bar for all three could not say which of
	// them a slow request spent its time in.
	//
	// Two settings rather than one, because they cost differently: three
	// spans per request instead of one is not a price to charge a surface
	// that did not ask.
	surface, err := web.NewRoutes(
		echo(http.MethodGet, "/books/{title}", web.PathParam("title", schema.Text())))
	if err != nil {
		t.Fatal(err)
	}

	if named := spansOf(t, surface, "/books/Zionomicon"); len(named) != 0 {
		t.Fatalf("expected no spans from a surface that asked for none, got %v", named)
	}

	named := spansOf(t, surface.Detailing(), "/books/Zionomicon")
	// The exported names, because a caller declaring a vocabulary uses these
	// and a test that spelled them again could drift from them.
	for _, phase := range web.PhaseNames() {
		if named[phase] != 1 {
			t.Fatalf("expected one %s span, got %v", phase, named)
		}
	}
}

func TestDetailingComposesWithWrappingSoThePhasesSitUnderTheRoute(t *testing.T) {
	// Which is what makes a waterfall readable: the route is the bar, and
	// the phases are the bars underneath it.
	surface, err := web.NewRoutes(
		echo(http.MethodGet, "/books/{title}", web.PathParam("title", schema.Text())))
	if err != nil {
		t.Fatal(err)
	}
	both := surface.Detailing().Wrapping(spanning)

	observer := &effecttest.RecordingObserver{}
	runtime, err := effect.NewRuntime(effect.WithObserver(observer))
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := web.NewAdapter(runtime, effect.Unit{}, refusalStatus)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	boundary.Handler(both.Handler()).ServeHTTP(recorder,
		httptest.NewRequest(http.MethodGet, "/books/Zionomicon", nil))

	// The wrapper's span encloses the phases, so every phase names it as its
	// parent -- which is the nesting a trace is assembled from.
	opened := map[uint64]string{}
	for _, event := range observer.Events() {
		if event.Kind == effect.EventSpanStarted {
			opened[event.SpanID] = event.Operation
		}
	}
	phases := 0
	for _, event := range observer.Events() {
		if event.Kind != effect.EventSpanStarted || event.ParentID == 0 {
			continue
		}
		if opened[event.ParentID] != "GET /books/{title}" {
			t.Fatalf("expected %s under the route, got it under %q",
				event.Operation, opened[event.ParentID])
		}
		phases++
	}
	if phases != 3 {
		t.Fatalf("expected the three phases under the route, got %d", phases)
	}
}

// spanning is a wrapper that names each route as a span, which is what an
// observer of a surface actually installs.
func spanning(
	declaration web.Declaration,
	handler web.Handler[effect.Unit, Refusal],
) web.Handler[effect.Unit, Refusal] {
	name := declaration.Method + " " + declaration.Path
	return func(request web.Request) webEffect[web.Response] {
		return handler(request).WithSpan(name)
	}
}

// spansOf sends one request through a surface and counts the spans by name.
func spansOf(t *testing.T, surface web.Routes[effect.Unit, Refusal], path string) map[string]int {
	t.Helper()
	observer := &effecttest.RecordingObserver{}
	runtime, err := effect.NewRuntime(effect.WithObserver(observer))
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := web.NewAdapter(runtime, effect.Unit{}, refusalStatus)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	boundary.Handler(surface.Handler()).ServeHTTP(recorder,
		httptest.NewRequest(http.MethodGet, path, nil))

	named := map[string]int{}
	for _, event := range observer.Events() {
		if event.Kind == effect.EventSpanStarted {
			named[event.Operation]++
		}
	}
	return named
}
