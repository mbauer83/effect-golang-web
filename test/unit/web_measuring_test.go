package unit

// Handing a route's phases to whoever is measuring them.
//
// Its own file because it is a different claim from naming them: the names are
// this module's business and the measuring is not, so what has to be
// established is the seam -- that each phase is opened and closed exactly once,
// that a phase which refused is closed too, and that a surface which did not
// ask for it is handed nothing.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/mbauer83/effect-golang-schema/schema"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

// sampled records what a sampler was told, in the order it was told.
type sampled struct {
	mutex  sync.Mutex
	opened []string
	closed []string
}

func (record *sampled) sampling() web.Sampling {
	return func(phase string) func() {
		record.mutex.Lock()
		record.opened = append(record.opened, phase)
		record.mutex.Unlock()
		return func() {
			record.mutex.Lock()
			record.closed = append(record.closed, phase)
			record.mutex.Unlock()
		}
	}
}

func TestMeasuringOpensAndClosesEveryPhaseOnce(t *testing.T) {
	record := &sampled{}
	surface := measured(t, record.sampling())

	answer(t, surface, "/books/Zionomicon")

	if !slices.Equal(record.opened, web.PhaseNames()) {
		t.Fatalf("expected the phases in order, got %v", record.opened)
	}
	// Closed in the order they finished, which is the order they began: the
	// phases are sequential, and a nested pair would say otherwise.
	if !slices.Equal(record.closed, web.PhaseNames()) {
		t.Fatalf("expected each phase closed once, got %v", record.closed)
	}
}

func TestMeasuringClosesAPhaseThatRefusedTheRequest(t *testing.T) {
	// A decoding that allocated a great deal and then refused is exactly the
	// window worth seeing, so the second call is a finalizer and not a
	// success path.
	record := &sampled{}
	counted, err := web.NewRoutes(web.Handle(
		web.Declare(http.MethodGet, "/books/{count}",
			web.PathParam("count", schema.Int64()),
			web.Returns(http.StatusOK, schema.Int64())),
		func(count int64) webEffect[int64] {
			return effect.For[effect.Unit, Refusal]().Succeed(count)
		}))
	if err != nil {
		t.Fatal(err)
	}
	surface := counted.Measuring(record.sampling())

	// Text where a number was declared, so the codec refuses it and the
	// handler is never reached.
	answer(t, surface, "/books/Zionomicon")

	if !slices.Contains(record.closed, web.PhaseDecoding) {
		t.Fatalf("expected the refused decoding closed, got %v", record.closed)
	}
	if slices.Contains(record.opened, web.PhaseHandling) {
		t.Fatalf("expected no handling phase for a refused request, got %v",
			record.opened)
	}
}

func TestMeasuringClosesAPhaseThatFailed(t *testing.T) {
	// The second call is a finalizer and not a success path, which is the
	// whole point of measuring at all: work that allocated a great deal and
	// then failed is the window worth seeing, and a success path would report
	// nothing about it.
	record := &sampled{}
	failing, err := web.NewRoutes(web.Handle(
		web.Declare(http.MethodGet, "/books/{title}",
			web.PathParam("title", schema.Text()),
			web.Returns(http.StatusOK, schema.Text())),
		func(string) webEffect[string] {
			return effect.For[effect.Unit, Refusal]().
				Fail[string](Refusal{Because: "no"})
		}))
	if err != nil {
		t.Fatal(err)
	}

	answer(t, failing.Measuring(record.sampling()), "/books/Zionomicon")

	if !slices.Contains(record.closed, web.PhaseHandling) {
		t.Fatalf("expected the failed handling closed, got %v", record.closed)
	}
	// The encoding never ran: there was no value to encode.
	if slices.Contains(record.opened, web.PhaseEncoding) {
		t.Fatalf("expected no encoding after a failure, got %v", record.opened)
	}
}

func TestMeasuringIsOncePerRunAndNotOncePerDescription(t *testing.T) {
	// A handler returns a description, and the runtime may interpret it once,
	// twice or never -- a retry interprets it again, and a combinator may
	// discard it. So the window has to open when the phase runs and not when
	// the effect describing it was built, or a retried request reports one
	// window and a discarded one reports a window that never happened.
	record := &sampled{}
	surface := measured(t, record.sampling())

	described := surface.Handler()(web.RequestFrom(
		httptest.NewRequest(http.MethodGet, "/books/Zionomicon", nil)).
		WithCaptures(map[string]string{"title": "Zionomicon"}))
	if len(record.opened) != 0 {
		t.Fatalf("expected nothing measured by describing, got %v", record.opened)
	}

	// The same description interpreted twice is two runs of each phase.
	for range 2 {
		effect.Run(context.Background(), effect.Unit{}, described)
	}
	if len(record.opened) != 2*len(web.PhaseNames()) {
		t.Fatalf("expected two runs of each phase, got %v", record.opened)
	}
	if len(record.closed) != 2*len(web.PhaseNames()) {
		t.Fatalf("expected each of them closed, got %v", record.closed)
	}
}

func TestASurfaceThatDidNotAskIsHandedNothing(t *testing.T) {
	record := &sampled{}
	surface, err := web.NewRoutes(
		echo(http.MethodGet, "/books/{title}", web.PathParam("title", schema.Text())))
	if err != nil {
		t.Fatal(err)
	}
	// Detailing names the phases and measures nothing: the two are separate
	// settings because they cost differently.
	answer(t, surface.Detailing(), "/books/Zionomicon")
	answer(t, surface, "/books/Zionomicon")

	if len(record.opened) != 0 {
		t.Fatalf("expected no sampling, got %v", record.opened)
	}
	// A nil sampler is Detailing, rather than a surface that panics per
	// request: a caller may pass what a flag gave it.
	if named := spansOf(t, surface.Measuring(nil), "/books/Zionomicon"); len(named) != 3 {
		t.Fatalf("expected a nil sampler to name the phases anyway, got %v", named)
	}
}

func measured(t *testing.T, sampling web.Sampling) web.Routes[effect.Unit, Refusal] {
	t.Helper()
	surface, err := web.NewRoutes(
		echo(http.MethodGet, "/books/{title}", web.PathParam("title", schema.Text())))
	if err != nil {
		t.Fatal(err)
	}
	return surface.Measuring(sampling)
}

func answer(t *testing.T, surface web.Routes[effect.Unit, Refusal], path string) {
	t.Helper()
	runtime, err := effect.NewRuntime()
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := web.NewAdapter(runtime, effect.Unit{}, refusalStatus)
	if err != nil {
		t.Fatal(err)
	}
	boundary.Handler(surface.Handler()).ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, path, nil))
}
