package unit

// What the settings cost, measured rather than asserted.
//
// Two questions a caller is entitled to ask. What does detailing a route's
// phases cost, and does that cost go away when it is turned off? The second
// matters more: a setting whose absence still charged for something would be a
// setting nobody could safely leave on in one service and off in another.
//
//	BenchmarkPlainRoute      2.04µs  3112 B  39 allocs
//	BenchmarkDetailedRoute   5.51µs  5704 B  79 allocs
//
// So three spans cost about 3.5µs and forty allocations per request, and none
// of it is paid by a surface that did not ask: the plain figures are what the
// route cost before the setting existed. The body is chosen once at assembly,
// which is what keeps it that way -- expressing both as the phased one cost
// 0.26µs and six allocations on the plain path.

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mbauer83/effect-golang-schema/schema"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

// benchSurface is one route, with or without its phases named.
func benchSurface(b *testing.B, detailing bool) http.Handler {
	surface, err := web.NewRoutes(
		echo(http.MethodGet, "/books/{title}", web.PathParam("title", schema.Text())))
	if err != nil {
		b.Fatal(err)
	}
	if detailing {
		surface = surface.Detailing()
	}
	runtime, err := effect.NewRuntime()
	if err != nil {
		b.Fatal(err)
	}
	boundary, err := web.NewAdapter(runtime, effect.Unit{}, refusalStatus)
	if err != nil {
		b.Fatal(err)
	}
	return boundary.Handler(surface.Handler())
}

func BenchmarkPlainRoute(b *testing.B) {
	handler := benchSurface(b, false)
	request := httptest.NewRequest(http.MethodGet, "/books/Zionomicon", nil)
	b.ReportAllocs()
	for b.Loop() {
		handler.ServeHTTP(httptest.NewRecorder(), request)
	}
}

func BenchmarkDetailedRoute(b *testing.B) {
	handler := benchSurface(b, true)
	request := httptest.NewRequest(http.MethodGet, "/books/Zionomicon", nil)
	b.ReportAllocs()
	for b.Loop() {
		handler.ServeHTTP(httptest.NewRecorder(), request)
	}
}
