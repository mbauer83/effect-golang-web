package unit

// What an assembled surface offers besides dispatch: one wording for a
// rejected request, the declarations a published document is projected from,
// and middleware that composes around the whole of it.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mbauer83/effect-golang-web/schema"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

func TestTheRejectionFormatIsAPropertyOfTheWholeSurface(t *testing.T) {
	// A client meets one API, not a collection of separately-worded ones.
	surface, err := web.NewRoutesRejecting(
		func(error) web.Response {
			return web.Text(http.StatusUnprocessableEntity, "we could not read that")
		},
		web.Handle(
			web.GET("/books", web.QueryParam("page", schema.Int()),
				web.Returns(http.StatusOK, schema.Int())),
			func(page int) webEffect[int] {
				return effect.For[effect.Unit, Refusal]().Succeed(page)
			}),
	)
	if err != nil {
		t.Fatal(err)
	}
	received := servedRequest(t, surface.Handler(),
		httptest.NewRequest(http.MethodGet, "/books?page=many", nil))

	if received.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected the surface's own status, got %d", received.StatusCode)
	}
	if body := bodyOf(t, received); body != "we could not read that" {
		t.Fatalf("expected the surface's own wording, got %q", body)
	}
}

func TestDeclarationsSurviveAssemblyInDeclaredOrder(t *testing.T) {
	// A published document is projected from these, so their order and content
	// are part of the surface rather than an implementation detail.
	surface, err := web.NewRoutes(
		naming(http.MethodGet, "/books", "list"),
		web.Handle(
			web.POST("/books", web.Entity(bookSchema),
				web.Returns(http.StatusCreated, bookSchema)).
				Summary("Add a book").
				Describe("The entity is the book to add.").
				Failing(http.StatusConflict, "a book with that title is already held"),
			func(book Book) webEffect[Book] {
				return effect.For[effect.Unit, Refusal]().Succeed(book)
			}),
	)
	if err != nil {
		t.Fatal(err)
	}

	declarations := surface.Declarations()
	if len(declarations) != 2 || declarations[0].Method != http.MethodGet {
		t.Fatalf("unexpected declarations: %#v", declarations)
	}
	added := declarations[1]
	if added.Summary != "Add a book" || added.Status != http.StatusCreated {
		t.Fatalf("unexpected declaration: %#v", added)
	}
	if added.Entity == nil || added.Entity.MediaType != "application/json" {
		t.Fatalf("expected the request body described, got %#v", added.Entity)
	}
	if len(added.Failures) != 1 || added.Failures[0].Status != http.StatusConflict {
		t.Fatalf("expected the failure documented, got %#v", added.Failures)
	}
}

func TestMiddlewareSeesTheRequestFirstAndTheResponseLast(t *testing.T) {
	order := []string{}
	mark := func(name string) web.Middleware[effect.Unit, Refusal] {
		return func(next web.Handler[effect.Unit, Refusal]) web.Handler[effect.Unit, Refusal] {
			return func(request web.Request) webEffect[web.Response] {
				order = append(order, "before "+name)
				return next(request).Map(func(response web.Response) web.Response {
					order = append(order, "after "+name)
					return response
				})
			}
		}
	}

	wrapped := web.Wrap(
		web.Respond[effect.Unit, Refusal](web.Text(http.StatusOK, "handled")),
		mark("outer"), nil, mark("inner"))
	if received := served(t, wrapped); received.StatusCode != http.StatusOK {
		t.Fatalf("expected the handler to answer, got %d", received.StatusCode)
	}

	want := "before outer,before inner,after inner,after outer"
	if got := strings.Join(order, ","); got != want {
		t.Fatalf("expected\n  %s\ngot\n  %s", want, got)
	}
}

func TestARequestReachesTheRouteThroughMiddleware(t *testing.T) {
	surface, err := web.NewRoutes(naming(http.MethodGet, "/books", "list"))
	if err != nil {
		t.Fatal(err)
	}
	stamping := func(next web.Handler[effect.Unit, Refusal]) web.Handler[effect.Unit, Refusal] {
		return web.Transform(next, func(response web.Response) web.Response {
			return response.WithHeader("X-Stamped", "yes")
		})
	}
	recorder := httptest.NewRecorder()
	runtime, err := effect.NewRuntime()
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := web.NewAdapter(runtime, effect.Unit{}, refusalStatus)
	if err != nil {
		t.Fatal(err)
	}
	boundary.Handler(web.Wrap(surface.Handler(), stamping)).
		ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/books", nil))

	received := recorder.Result()
	if received.Header.Get("X-Stamped") != "yes" {
		t.Fatalf("expected the middleware's header, got %v", received.Header)
	}
	if body := strings.TrimSpace(bodyOf(t, received)); body != `"list"` {
		t.Fatalf("expected the route's answer, got %s", body)
	}
}
