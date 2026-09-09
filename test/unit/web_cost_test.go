package unit

// What the settings cost, measured rather than asserted.
//
// Three questions a caller is entitled to ask. What does detailing a route's
// phases cost, what does handing them to a sampler add on top, and does either
// cost go away when it is turned off? The last matters most: a setting whose
// absence still charged for something would be a setting nobody could safely
// leave on in one service and off in another.
//
//	BenchmarkPlainRoute      2.04µs  3112 B  39 allocs
//	BenchmarkDetailedRoute   5.51µs  5704 B  79 allocs
//	BenchmarkMeasuredRoute   7.10µs  6729 B  119 allocs
//
// So three spans cost about 3.5µs and forty allocations per request, and none
// of it is paid by a surface that did not ask: the plain figures are what the
// route cost before the setting existed. The body is chosen once at assembly,
// which is what keeps it that way -- expressing both as the phased one cost
// 0.26µs and six allocations on the plain path.
//
// The seam itself is 1.5µs and forty allocations on top of detailing: three
// suspensions and three finalizers, one pair per phase. The sampler here does
// nothing, deliberately -- what this measures is the seam and not somebody's
// measuring, and what the measuring costs belongs where the counters are read,
// which is not this module.
//
// The plain figures are unchanged to the byte by both settings existing, which
// is the answer to the third question.

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mbauer83/effect-golang-schema/schema"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

// benchSurface is one route under one setting.
//
// The setting is a function rather than a flag, so a third arrangement is a
// third argument at the call and not a second boolean here.
func benchSurface(
	b *testing.B,
	setting func(web.Routes[effect.Unit, Refusal]) web.Routes[effect.Unit, Refusal],
) http.Handler {
	surface, err := web.NewRoutes(
		echo(http.MethodGet, "/books/{title}", web.PathParam("title", schema.Text())))
	if err != nil {
		b.Fatal(err)
	}
	if setting != nil {
		surface = setting(surface)
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
	handler := benchSurface(b, nil)
	request := httptest.NewRequest(http.MethodGet, "/books/Zionomicon", nil)
	b.ReportAllocs()
	for b.Loop() {
		handler.ServeHTTP(httptest.NewRecorder(), request)
	}
}

func BenchmarkDetailedRoute(b *testing.B) {
	handler := benchSurface(b, func(
		surface web.Routes[effect.Unit, Refusal],
	) web.Routes[effect.Unit, Refusal] {
		return surface.Detailing()
	})
	request := httptest.NewRequest(http.MethodGet, "/books/Zionomicon", nil)
	b.ReportAllocs()
	for b.Loop() {
		handler.ServeHTTP(httptest.NewRecorder(), request)
	}
}

func BenchmarkMeasuredRoute(b *testing.B) {
	handler := benchSurface(b, func(
		surface web.Routes[effect.Unit, Refusal],
	) web.Routes[effect.Unit, Refusal] {
		return surface.Measuring(func(string) func() { return func() {} })
	})
	request := httptest.NewRequest(http.MethodGet, "/books/Zionomicon", nil)
	b.ReportAllocs()
	for b.Loop() {
		handler.ServeHTTP(httptest.NewRecorder(), request)
	}
}
