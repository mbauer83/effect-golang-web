package unit

// Wrapping every route with something that knows which route it is.
//
// The seam a cross-cutting concern needs when it has to name the route.
// Ordinary Middleware wraps the surface's handler, and by then the only thing
// left of the route is the path the client asked for -- which is an unbounded
// value, so naming anything after it turns one metric label or span name into
// one series per request.

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mbauer83/effect-golang-schema/schema"

	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

// through assembles the routes, wraps them, and sends one request.
func through(
	t *testing.T,
	request *http.Request,
	each web.Matched[effect.Unit, Refusal],
	routes ...web.Route[effect.Unit, Refusal],
) *http.Response {
	t.Helper()
	surface, err := web.NewRoutes(routes...)
	if err != nil {
		t.Fatal(err)
	}
	return reaching(t, surface.Wrapping(each), request)
}

// reaching sends one request through an assembled surface.
func reaching(
	t *testing.T,
	surface web.Routes[effect.Unit, Refusal],
	request *http.Request,
) *http.Response {
	t.Helper()
	runtime, err := effect.NewRuntime()
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := web.NewAdapter(runtime, effect.Unit{}, refusalStatus)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	boundary.Handler(surface.Handler()).ServeHTTP(recorder, request)
	return recorder.Result()
}

// noting records the route each request reached, by its pattern.
func noting(seen *[]string) web.Matched[effect.Unit, Refusal] {
	return func(
		declaration web.Declaration,
		handler web.Handler[effect.Unit, Refusal],
	) web.Handler[effect.Unit, Refusal] {
		name := declaration.Method + " " + declaration.Path
		return func(request web.Request) webEffect[web.Response] {
			*seen = append(*seen, name)
			return handler(request)
		}
	}
}

func TestEveryRouteIsWrappedAndEachIsToldWhichItIs(t *testing.T) {
	// What makes this a setting rather than a convention: one call wraps the
	// whole surface, so a route nobody remembered is wrapped too.
	seen := []string{}
	received := through(t, httptest.NewRequest(http.MethodGet, "/books/Zionomicon", nil),
		noting(&seen),
		naming(http.MethodGet, "/books", "listing"),
		naming(http.MethodGet, "/books/{title}", "finding"))

	if body := answered(t, received); body != `"finding"` {
		t.Fatalf("expected the route still to answer, got %s", body)
	}
	// The pattern, which is what a bounded name is made from, and never the
	// path the client asked for.
	if len(seen) != 1 || seen[0] != "GET /books/{title}" {
		t.Fatalf("expected the pattern, got %v", seen)
	}
}

func TestWrappingLeavesTheDeclarationsAndThePrecedenceAlone(t *testing.T) {
	// A wrapper may not change what the surface is: the same declarations
	// project into the same document, and the tree still prefers a literal to
	// a capture.
	surface, err := web.NewRoutes(
		naming(http.MethodGet, "/books/latest", "literal"),
		naming(http.MethodGet, "/books/{title}", "capture"))
	if err != nil {
		t.Fatal(err)
	}
	seen := []string{}
	wrapped := surface.Wrapping(noting(&seen))

	before, after := surface.Declarations(), wrapped.Declarations()
	if len(before) != len(after) {
		t.Fatalf("expected the same declarations, got %d and %d", len(before), len(after))
	}
	for index := range before {
		if before[index].Path != after[index].Path {
			t.Fatalf("declaration %d changed: %q against %q",
				index, before[index].Path, after[index].Path)
		}
	}
	if body := answered(t, reaching(t, wrapped,
		httptest.NewRequest(http.MethodGet, "/books/latest", nil))); body != `"literal"` {
		t.Fatalf("expected the literal route still preferred, got %s", body)
	}
	// A method the surface does not serve is still 405 rather than 404, so the
	// tree was rebuilt and not flattened.
	answered := reaching(t, wrapped, httptest.NewRequest(http.MethodDelete, "/books/latest", nil))
	if answered.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", answered.StatusCode)
	}
}

func TestWrappingWithNothingIsTheSurfaceItself(t *testing.T) {
	// So a caller turns observation off with a nil rather than by assembling
	// a different surface.
	surface, err := web.NewRoutes(naming(http.MethodGet, "/books", "listing"))
	if err != nil {
		t.Fatal(err)
	}
	if wrapped := surface.Wrapping(nil); len(wrapped.Declarations()) != 1 {
		t.Fatal("expected the surface unchanged")
	}
	// And a zero Routes wraps nothing rather than panicking.
	var none web.Routes[effect.Unit, Refusal]
	seen := []string{}
	if wrapped := none.Wrapping(noting(&seen)); len(wrapped.Declarations()) != 0 {
		t.Fatal("expected a zero surface to stay zero")
	}
}

func TestAWrapperSeesTheCodecsAndNotOnlyTheHandler(t *testing.T) {
	// The useful boundary. A route whose response is expensive to encode is
	// expensive to serve, whatever its handler cost -- and a request the
	// codecs refuse never reaches the handler at all, so a wrapper around the
	// handler alone would not have seen it.
	statuses := []int{}
	watching := func(
		_ web.Declaration,
		handler web.Handler[effect.Unit, Refusal],
	) web.Handler[effect.Unit, Refusal] {
		return func(request web.Request) webEffect[web.Response] {
			return handler(request).Map(func(said web.Response) web.Response {
				statuses = append(statuses, said.Status())
				return said
			})
		}
	}

	received := through(t, httptest.NewRequest(http.MethodGet, "/books/x", nil),
		watching,
		echo(http.MethodGet, "/books/{title}",
			web.PathParam("title", schema.MinLength(schema.Text(), 4))))

	if received.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected the codec to refuse the short title, got %d", received.StatusCode)
	}
	if len(statuses) != 1 || statuses[0] != http.StatusBadRequest {
		t.Fatalf("expected the wrapper to see the rejection, saw %v", statuses)
	}
}
