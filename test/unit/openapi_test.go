package unit

// The published document. It is projected from the same declarations that
// dispatch a request, and it is checked by a parser that has never seen this
// module -- because "this looks like OpenAPI to me" is not a test.

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	kin "github.com/getkin/kin-openapi/openapi3"

	"github.com/mbauer83/effect-golang-schema/schema"
	"github.com/mbauer83/effect-golang-schema/schema/jsonschema"
	"github.com/mbauer83/effect-golang-web/openapi"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

// published projects routes and returns both the document and its bytes.
func published(t *testing.T, routes ...web.Route[effect.Unit, Refusal]) (openapi.Document, []byte) {
	t.Helper()
	surface, err := web.NewRoutes(routes...)
	if err != nil {
		t.Fatal(err)
	}
	document := openapi.Describe(
		openapi.Info{Title: "Books", Version: "1.0.0", Description: "A small catalogue."},
		surface.Declarations(),
		openapi.Server{URL: "https://example.test", Description: "the live one"},
	)
	rendered, err := document.Render()
	if err != nil {
		t.Fatal(err)
	}
	return document, rendered
}

// accepted parses and validates the document with kin-openapi, which is a test
// dependency: nothing in the module needs it, and that is the point.
func accepted(t *testing.T, rendered []byte) {
	t.Helper()
	loaded, err := kin.NewLoader().LoadFromData(rendered)
	if err != nil {
		t.Fatalf("the emitted document does not load: %v\n%s", err, rendered)
	}
	if err := loaded.Validate(context.Background()); err != nil {
		t.Fatalf("the emitted document is not valid: %v\n%s", err, rendered)
	}
}

// booksSurface is a surface wide enough to exercise every part of the
// projection: a body, a path capture, a query parameter, a documented failure
// and a response with no entity.
func booksSurface(t *testing.T) []web.Route[effect.Unit, Refusal] {
	t.Helper()
	return []web.Route[effect.Unit, Refusal]{
		web.Handle(
			web.GET("/books", web.OptionalQueryParam("shelf", schema.Text()),
				web.Returns(http.StatusOK, schema.List(bookSchema))).
				Summary("List the catalogue"),
			func(*string) webEffect[[]Book] {
				return effect.For[effect.Unit, Refusal]().Succeed([]Book{})
			}),
		web.Handle(
			web.POST("/books", web.Entity(bookSchema),
				web.Returns(http.StatusCreated, bookSchema)).
				Summary("Add a book").
				Failing(http.StatusInternalServerError, "the store could not be reached").
				Failing(http.StatusConflict, "that title is already held"),
			func(book Book) webEffect[Book] {
				return effect.For[effect.Unit, Refusal]().Succeed(book)
			}),
		web.Handle(
			web.DELETE("/books/{title}", web.PathParam("title", schema.Text()),
				web.ReturnsNothing(http.StatusNoContent)),
			func(string) webEffect[effect.Unit] {
				return effect.For[effect.Unit, Refusal]().Succeed(effect.Unit{})
			}),
	}
}

func TestTheEmittedDocumentIsValidOpenAPI(t *testing.T) {
	_, rendered := published(t, booksSurface(t)...)
	accepted(t, rendered)
}

func TestOneShapeUsedTwiceBecomesOneComponent(t *testing.T) {
	// The reason the schema layer exposes its structure at all: a type used by
	// several operations is described once and referred to thereafter.
	document, rendered := published(t, booksSurface(t)...)

	if _, described := document.Components["Book"]; !described {
		t.Fatalf("expected a Book component, got %v", document.Components)
	}
	if strings.Count(string(rendered), `"#/components/schemas/Book"`) != 3 {
		t.Fatalf("expected three references to the one component:\n%s", rendered)
	}
}

func TestOperationsOnOnePathAreGatheredUnderIt(t *testing.T) {
	document, _ := published(t, booksSurface(t)...)

	if len(document.Paths) != 2 {
		t.Fatalf("expected two paths, got %#v", document.Paths)
	}
	if document.Paths[0].Path != "/books" || len(document.Paths[0].Operations) != 2 {
		t.Fatalf("expected both operations under /books, got %#v", document.Paths[0])
	}
	// Declared order, because an author's order is more use to a reader than an
	// alphabetical one, and a stable order is what makes a document diffable.
	if document.Paths[0].Operations[0].Method != http.MethodGet {
		t.Fatalf("expected the declared order, got %#v", document.Paths[0].Operations)
	}
}

func TestAnOperationSaysWhatItReadsAndWhatItAnswersWith(t *testing.T) {
	document, _ := published(t, booksSurface(t)...)
	adding := document.Paths[0].Operations[1]

	if adding.ID != "postBooks" || adding.Summary != "Add a book" {
		t.Fatalf("unexpected operation: %#v", adding)
	}
	if adding.RequestBody == nil || adding.RequestBody.MediaType != "application/json" {
		t.Fatalf("expected the entity described, got %#v", adding.RequestBody)
	}
	if len(adding.Responses) != 3 {
		t.Fatalf("expected the success and both documented failures, got %#v", adding.Responses)
	}
	// Ordered by status rather than by declaration, so the same routes always
	// render identically however the failures were written down.
	statuses := []int{}
	for _, response := range adding.Responses {
		statuses = append(statuses, response.Status)
	}
	if !slices.IsSorted(statuses) {
		t.Fatalf("expected the responses ordered by status, got %v", statuses)
	}
	if adding.Responses[1].Status != http.StatusConflict ||
		adding.Responses[1].Description != "that title is already held" {
		t.Fatalf("expected the documented reason, got %#v", adding.Responses[1])
	}
}

func TestAResponseWithNoEntitySaysSoAndStillHasADescription(t *testing.T) {
	// A description is required of every response, so one is supplied from the
	// status when the endpoint said nothing.
	document, rendered := published(t, booksSurface(t)...)
	removing := document.Paths[1].Operations[0]

	if len(removing.Responses) != 1 || removing.Responses[0].Schema != nil {
		t.Fatalf("expected one response with no entity, got %#v", removing.Responses)
	}
	if removing.Responses[0].Description != "No Content" {
		t.Fatalf("expected a description from the status, got %q", removing.Responses[0].Description)
	}
	accepted(t, rendered)
}

func TestACapturedSegmentNothingReadsIsStillDescribed(t *testing.T) {
	// A route may capture a segment its handler has no use for, and the
	// specification still requires the operation to declare it. Leaving it out
	// would make the document invalid, which the parser says so plainly.
	_, rendered := published(t,
		web.Handle(
			web.GET("/books/{title}/authors", web.Nothing(),
				web.Returns(http.StatusOK, schema.List(schema.Text()))),
			func(effect.Unit) webEffect[[]string] {
				return effect.For[effect.Unit, Refusal]().Succeed([]string{})
			}))

	if !strings.Contains(string(rendered), `"name":"title","in":"path"`) {
		t.Fatalf("expected the captured segment described:\n%s", rendered)
	}
	accepted(t, rendered)
}

func TestAWildcardIsWrittenAsThePathParameterItCanBe(t *testing.T) {
	// OpenAPI has no wildcard. Describing it as an ordinary parameter is the
	// closest honest thing, and it is why a wildcard route's document says less
	// than the route knows.
	document, rendered := published(t,
		web.Handle(
			web.GET("/files/{path...}", web.PathParam("path", schema.Text()),
				web.Returns(http.StatusOK, schema.Text())),
			func(path string) webEffect[string] {
				return effect.For[effect.Unit, Refusal]().Succeed(path)
			}))

	if document.Paths[0].Path != "/files/{path}" {
		t.Fatalf("expected the template spelled the way OpenAPI does, got %q", document.Paths[0].Path)
	}
	accepted(t, rendered)
}

func TestRenderingIsStable(t *testing.T) {
	_, first := published(t, booksSurface(t)...)
	_, second := published(t, booksSurface(t)...)

	if string(first) != string(second) {
		t.Fatalf("the same routes rendered differently:\n%s\n%s", first, second)
	}
}

func TestComponentsArePointedAtWhereTheDocumentKeepsThem(t *testing.T) {
	// A standalone schema keeps its components under $defs and an OpenAPI
	// document under #/components/schemas. The shapes are identical, so only
	// the pointer differs and only the enclosing document knows what it is.
	if pointer := openapi.ComponentPointer("Book"); pointer != "#/components/schemas/Book" {
		t.Fatalf("unexpected pointer %q", pointer)
	}

	projected, components := jsonschema.ProjectAllReferencing(
		openapi.ComponentPointer, bookSchema.Structure())
	if len(projected) != 1 || projected[0].Ref != openapi.ComponentPointer("Book") {
		t.Fatalf("expected the caller's pointer form, got %#v", projected)
	}
	if _, described := components["Book"]; !described {
		t.Fatalf("expected the component described, got %v", components)
	}
}

func TestAnOperationIsAValueAProjectionCanRead(t *testing.T) {
	// The document is a typed model, not a map of a top type: a consumer reads
	// it without re-discovering what shape it produced.
	document, _ := published(t, booksSurface(t)...)

	var listing openapi.Operation = document.Paths[0].Operations[0]
	if listing.Method != http.MethodGet || listing.ID != "getBooks" {
		t.Fatalf("unexpected operation: %#v", listing)
	}
	if len(listing.Parameters) != 1 || listing.Parameters[0].In != string(web.InQuery) {
		t.Fatalf("expected the query parameter described, got %#v", listing.Parameters)
	}
	if listing.Parameters[0].Required {
		t.Fatalf("expected an optional parameter to say so, got %#v", listing.Parameters[0])
	}
}
