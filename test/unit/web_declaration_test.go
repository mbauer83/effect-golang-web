package unit

// What an endpoint says about itself. A published document is projected from
// exactly this, so a parameter's location, a documented failure and the method
// each helper declares are part of the surface rather than incidental.

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mbauer83/effect-golang-schema/schema"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

func TestAParameterSaysWhereTheRequestCarriesIt(t *testing.T) {
	codec := web.Convert(
		web.Both(
			web.Both(web.PathParam("title", schema.Text()), web.QueryParam("shelf", schema.Text())),
			web.OptionalHeaderParam("X-Trace", schema.Text()),
		),
		func(effect.Product[effect.Product[string, string], *string]) (effect.Unit, error) {
			return effect.Unit{}, nil
		},
	)

	wanted := []web.Parameter{
		{Name: "title", In: web.InPath, Required: true},
		{Name: "shelf", In: web.InQuery, Required: true},
		{Name: "X-Trace", In: web.InHeader},
	}
	parameters := codec.Parameters()
	if len(parameters) != len(wanted) {
		t.Fatalf("expected %d parameters, got %#v", len(wanted), parameters)
	}
	for index, want := range wanted {
		var found web.Parameter = parameters[index]
		var where web.Location = found.In
		if found.Name != want.Name || where != want.In || found.Required != want.Required {
			t.Errorf("expected %#v, got name %q in %q required %v",
				want, found.Name, found.In, found.Required)
		}
		if found.Node == nil {
			t.Errorf("%s carries no shape for a projection to walk", found.Name)
		}
	}
}

func TestAnOptionalHeaderIsNilWhenTheRequestDoesNotCarryIt(t *testing.T) {
	codec := web.OptionalHeaderParam("X-Trace", schema.Text())
	request := httptest.NewRequest(http.MethodGet, "/books", nil)

	absent, err := read(t, codec, request, nil)
	if err != nil || absent != nil {
		t.Fatalf("expected nil for an absent header, got %v (%v)", absent, err)
	}
	request.Header.Set("X-Trace", "abc")
	present, err := read(t, codec, request, nil)
	if err != nil || present == nil || *present != "abc" {
		t.Fatalf("expected the header read, got %v (%v)", present, err)
	}
}

func TestEachHelperDeclaresItsOwnMethod(t *testing.T) {
	declared := map[string]web.Declaration{
		http.MethodGet: web.GET("/books", web.Nothing(),
			web.ReturnsNothing(http.StatusOK)).Declaration(),
		http.MethodPost: web.POST("/books", web.Nothing(),
			web.ReturnsNothing(http.StatusCreated)).Declaration(),
		http.MethodPut: web.PUT("/books", web.Nothing(),
			web.ReturnsNothing(http.StatusOK)).Declaration(),
		http.MethodPatch: web.PATCH("/books", web.Nothing(),
			web.ReturnsNothing(http.StatusOK)).Declaration(),
		http.MethodDelete: web.DELETE("/books", web.Nothing(),
			web.ReturnsNothing(http.StatusNoContent)).Declaration(),
	}
	for method, declaration := range declared {
		if declaration.Method != method {
			t.Errorf("expected %s, got %s", method, declaration.Method)
		}
		if declaration.Path != "/books" {
			t.Errorf("%s: expected the path rendered, got %q", method, declaration.Path)
		}
	}
}

func TestADocumentedFailureIsCarriedWithoutBeingPerformed(t *testing.T) {
	// Failing says what the boundary's mapping will produce. It does not do the
	// mapping, and nothing about the route changes because it was written.
	endpoint := web.GET("/books/{title}", web.PathParam("title", schema.Text()),
		web.Returns(http.StatusOK, bookSchema)).
		Failing(http.StatusNotFound, "no book with that title is held").
		Failing(http.StatusGone, "it was withdrawn")

	failures := endpoint.Declaration().Failures
	if len(failures) != 2 {
		t.Fatalf("expected both failures documented, got %#v", failures)
	}
	var first web.FailureResponse = failures[0]
	if first.Status != http.StatusNotFound || first.Doc == "" {
		t.Fatalf("unexpected failure: %#v", first)
	}
	if failures[1].Status != http.StatusGone {
		t.Fatalf("expected the order kept, got %#v", failures)
	}
}

func TestDocumentingAnEndpointLeavesTheOriginalAlone(t *testing.T) {
	original := web.GET("/books", web.Nothing(), web.ReturnsNothing(http.StatusOK))
	_ = original.Summary("List").Describe("Everything").Failing(http.StatusGone, "gone")

	if declared := original.Declaration(); declared.Summary != "" ||
		declared.Doc != "" || len(declared.Failures) != 0 {
		t.Fatalf("the original endpoint was changed: %#v", declared)
	}
}
