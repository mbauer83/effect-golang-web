package unit

// Dispatch. What matters is that the most specific pattern that can match does,
// that a path which matches with the wrong method is told apart from one that
// matches nothing, and that two routes which could serve the same request are
// refused when the surface is assembled rather than surprising someone later.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mbauer83/effect-golang-web/schema"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

// echo is an endpoint that answers with the text it was given, which is enough
// to see which route a request reached.
func echo(method string, path string, input web.Codec[string]) web.Route[effect.Unit, Refusal] {
	return web.Handle(
		web.Declare(method, path, input, web.Returns(http.StatusOK, schema.Text())),
		func(said string) webEffect[string] {
			return effect.For[effect.Unit, Refusal]().Succeed(said)
		},
	)
}

// naming is an endpoint whose answer says which pattern served it.
func naming(method string, path string, name string) web.Route[effect.Unit, Refusal] {
	return web.Handle(
		web.Declare(method, path, web.Nothing(), web.Returns(http.StatusOK, schema.Text())),
		func(effect.Unit) webEffect[string] {
			return effect.For[effect.Unit, Refusal]().Succeed(name)
		},
	)
}

// answered is the response body, trimmed: a JSON document is written with a
// trailing newline and the tests are about what was said, not how it ended.
func answered(t *testing.T, response *http.Response) string {
	t.Helper()
	return strings.TrimSpace(bodyOf(t, response))
}

// dispatched assembles the routes and sends one request through them.
func dispatched(t *testing.T, request *http.Request, routes ...web.Route[effect.Unit, Refusal]) *http.Response {
	t.Helper()
	surface, err := web.NewRoutes(routes...)
	if err != nil {
		t.Fatal(err)
	}
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

func TestALiteralIsPreferredToACapture(t *testing.T) {
	received := dispatched(t, httptest.NewRequest(http.MethodGet, "/books/latest", nil),
		naming(http.MethodGet, "/books/latest", "literal"),
		naming(http.MethodGet, "/books/{title}", "capture"))

	if body := answered(t, received); body != `"literal"` {
		t.Fatalf("expected the literal route, got %s", body)
	}
}

func TestACaptureIsPreferredToAWildcard(t *testing.T) {
	received := dispatched(t, httptest.NewRequest(http.MethodGet, "/files/one", nil),
		naming(http.MethodGet, "/files/{name}", "capture"),
		naming(http.MethodGet, "/files/{path...}", "wildcard"))

	if body := answered(t, received); body != `"capture"` {
		t.Fatalf("expected the capture route, got %s", body)
	}
}

func TestALiteralThatLeadsNowhereDoesNotShadowACaptureThatMatches(t *testing.T) {
	// Without backtracking, /books/latest would win the first segment pair and
	// the request would then find nothing -- which is the classic router bug
	// this tree exists to avoid.
	received := dispatched(t, httptest.NewRequest(http.MethodGet, "/books/latest/authors", nil),
		naming(http.MethodGet, "/books/latest", "literal"),
		naming(http.MethodGet, "/books/{title}/authors", "capture"))

	if body := answered(t, received); body != `"capture"` {
		t.Fatalf("expected the capture route, got %s", body)
	}
}

func TestACaptureReachesTheHandlerAsAValue(t *testing.T) {
	received := dispatched(t, httptest.NewRequest(http.MethodGet, "/books/Zionomicon", nil),
		echo(http.MethodGet, "/books/{title}", web.PathParam("title", schema.Text())))

	if body := answered(t, received); body != `"Zionomicon"` {
		t.Fatalf("expected the captured segment, got %s", body)
	}
}

func TestAWildcardCapturesTheRestOfThePath(t *testing.T) {
	received := dispatched(t, httptest.NewRequest(http.MethodGet, "/files/deep/inside/here.txt", nil),
		echo(http.MethodGet, "/files/{path...}", web.PathParam("path", schema.Text())))

	if body := answered(t, received); body != `"deep/inside/here.txt"` {
		t.Fatalf("expected the rest of the path, got %s", body)
	}
}

func TestAMethodMismatchOnAMatchedPathIsToldApartFromNothingBeingThere(t *testing.T) {
	routes := []web.Route[effect.Unit, Refusal]{
		naming(http.MethodGet, "/books", "get"),
		naming(http.MethodDelete, "/books", "delete"),
	}
	mismatched := dispatched(t, httptest.NewRequest(http.MethodPut, "/books", nil), routes...)

	if mismatched.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", mismatched.StatusCode)
	}
	// The specification requires the client to be told what it could have used,
	// and the order is sorted so the same mismatch always answers the same way.
	if allowed := mismatched.Header.Get("Allow"); allowed != "DELETE, GET" {
		t.Fatalf("expected the allowed methods, got %q", allowed)
	}

	missing := dispatched(t, httptest.NewRequest(http.MethodGet, "/shelves", nil), routes...)
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", missing.StatusCode)
	}
}

func TestAMethodServedByALessSpecificRouteIsStillFound(t *testing.T) {
	// The literal branch matches the path but not the method. Giving up there
	// would answer 405 when a capture route serves the method perfectly well.
	received := dispatched(t, httptest.NewRequest(http.MethodDelete, "/books/latest", nil),
		naming(http.MethodGet, "/books/latest", "literal-get"),
		naming(http.MethodDelete, "/books/{title}", "capture-delete"))

	if received.StatusCode != http.StatusOK {
		t.Fatalf("expected the capture route to serve it, got %d", received.StatusCode)
	}
	if body := answered(t, received); body != `"capture-delete"` {
		t.Fatalf("expected the capture route, got %s", body)
	}
}

func TestARequestTheCodecsRefuseNeverReachesTheHandler(t *testing.T) {
	reached := false
	route := web.Handle(
		web.GET("/books", web.QueryParam("page", schema.Int()),
			web.Returns(http.StatusOK, schema.Int())),
		func(page int) webEffect[int] {
			reached = true
			return effect.For[effect.Unit, Refusal]().Succeed(page)
		},
	)
	received := dispatched(t, httptest.NewRequest(http.MethodGet, "/books?page=many", nil), route)

	if received.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", received.StatusCode)
	}
	if reached {
		t.Fatal("the handler was reached with a request that had been refused")
	}
}

func TestARouteThatAnswersNothingSendsNoEntity(t *testing.T) {
	route := web.Handle(
		web.DELETE("/books/{title}", web.PathParam("title", schema.Text()),
			web.ReturnsNothing(http.StatusNoContent)),
		func(string) webEffect[effect.Unit] {
			return effect.For[effect.Unit, Refusal]().Succeed(effect.Unit{})
		},
	)
	received := dispatched(t, httptest.NewRequest(http.MethodDelete, "/books/T", nil), route)

	if received.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", received.StatusCode)
	}
	if body := answered(t, received); body != "" {
		t.Fatalf("expected no entity, got %q", body)
	}
}
