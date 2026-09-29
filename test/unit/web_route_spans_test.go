package unit

// A request is one span, named for its route's pattern and never its path.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/mbauer83/effect-golang-schema/schema"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

type spanNames struct {
	mu    sync.Mutex
	names []string
}

func (seen *spanNames) Observe(_ context.Context, event effect.RuntimeEvent) {
	if event.Kind == effect.EventSpanStarted {
		seen.mu.Lock()
		seen.names = append(seen.names, event.Operation)
		seen.mu.Unlock()
	}
}

func TestARequestIsASpanNamedForItsPattern(t *testing.T) {
	seen := &spanNames{}
	runtime, err := effect.NewRuntime(effect.WithObserver(seen))
	if err != nil {
		t.Fatal(err)
	}
	surface, err := web.NewRoutes(echo(http.MethodGet, "/books/{title}", web.PathParam("title", schema.Text())))
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := web.NewAdapter(runtime, effect.Unit{}, refusalStatus)
	if err != nil {
		t.Fatal(err)
	}
	handler := boundary.Handler(surface.WithMiddleware(web.RouteSpans[effect.Unit, Refusal]()).Handler())
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/books/Zionomicon", nil))

	seen.mu.Lock()
	defer seen.mu.Unlock()
	if len(seen.names) != 1 || seen.names[0] != "GET /books/{title}" {
		t.Fatalf("expected one span named for the pattern, got %v", seen.names)
	}
}
