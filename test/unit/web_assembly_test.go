package unit

// Assembling a surface. Every mistake here is a construction mistake, reported
// where the routes are put together rather than by a request that went
// somewhere unexpected.

import (
	"net/http"
	"strings"
	"testing"

	"github.com/mbauer83/effect-golang-web/schema"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

func assembled(routes ...web.Route[effect.Unit, Refusal]) error {
	_, err := web.NewRoutes(routes...)
	return err
}

func TestTwoRoutesThatCouldMatchTheSameRequestAreRefused(t *testing.T) {
	err := assembled(
		naming(http.MethodGet, "/books/{title}", "first"),
		naming(http.MethodGet, "/books/{title}", "second"))
	if err == nil {
		t.Fatal("expected two identical patterns to be refused")
	}
	if !strings.Contains(err.Error(), "could match the same request") {
		t.Fatalf("expected the reason to say so, got %v", err)
	}
}

func TestTwoRoutesCapturingOneSegmentUnderDifferentNamesAreRefused(t *testing.T) {
	// They share one node in the tree, so one of the two names would silently
	// never be bound. Renaming quietly is worse than refusing.
	err := assembled(
		naming(http.MethodGet, "/books/{title}", "by-title"),
		naming(http.MethodGet, "/books/{id}/authors", "by-id"))
	if err == nil {
		t.Fatal("expected two names for one captured segment to be refused")
	}
	if !strings.Contains(err.Error(), "captures a segment as") {
		t.Fatalf("expected the reason to say so, got %v", err)
	}
}

func TestTheSameNameAtTheSamePositionIsNotAmbiguous(t *testing.T) {
	// Different methods, or different paths beneath one capture, are not a
	// conflict: the tree distinguishes them.
	if err := assembled(
		naming(http.MethodGet, "/books/{title}", "get"),
		naming(http.MethodDelete, "/books/{title}", "delete"),
		naming(http.MethodGet, "/books/{title}/authors", "authors"),
	); err != nil {
		t.Fatalf("expected these to coexist, got %v", err)
	}
}

func TestAPathMistakeIsReportedWhereThePathIsWritten(t *testing.T) {
	cases := map[string]string{
		"an unrooted path":            "books/{title}",
		"an unclosed capture":         "/books/{title",
		"a nameless capture":          "/books/{}",
		"a capture sharing a segment": "/books/x{title}",
		"a wildcard that is not last": "/files/{path...}/name",
		"two captures with one name":  "/books/{title}/{title}",
	}
	for mistake, path := range cases {
		if err := web.ValidateEndpoint(
			web.Declare(http.MethodGet, path, web.Nothing(),
				web.Returns(http.StatusOK, schema.Text())),
		); err == nil {
			t.Errorf("expected %s to be reported", mistake)
		}
	}
}

func TestEndpointDeclarationMistakesAreReported(t *testing.T) {
	cases := map[string]error{
		"a path parameter the path does not capture": web.ValidateEndpoint(
			web.GET("/books/{title}", web.PathParam("titel", schema.Text()),
				web.Returns(http.StatusOK, schema.Text()))),
		"no method": web.ValidateEndpoint(
			web.Declare("", "/books", web.Nothing(), web.Returns(http.StatusOK, schema.Text()))),
		"no output": web.ValidateEndpoint(
			web.Endpoint[effect.Unit, string]{}),
		"a status outside the range": web.ValidateEndpoint(
			web.GET("/books", web.Nothing(), web.Returns(42, schema.Text()))),
		"an output schema that is unusable": web.ValidateEndpoint(
			web.GET("/books", web.Nothing(),
				web.Returns(http.StatusOK, schema.Schema[string]{}))),
		"an input codec that is unusable": web.ValidateEndpoint(
			web.GET("/books", web.QueryParam("", schema.Text()),
				web.Returns(http.StatusOK, schema.Text()))),
	}
	for mistake, err := range cases {
		if err == nil {
			t.Errorf("expected %s to be reported", mistake)
		}
	}
}

func TestASurfaceWithoutRoutesOrWithoutARejectionIsRefused(t *testing.T) {
	if err := assembled(); err == nil {
		t.Error("expected a surface with no routes to be refused")
	}
	if _, err := web.NewRoutesRejecting[effect.Unit, Refusal](nil,
		naming(http.MethodGet, "/books", "get")); err == nil {
		t.Error("expected a missing rejection format to be refused")
	}
}

func TestARouteCarriesItsFaultIntoTheAssembly(t *testing.T) {
	// A broken endpoint must not become a route that answers oddly; the fault
	// travels to where the surface is put together and stops it there.
	broken := web.Handle(
		web.GET("/books/{title}", web.PathParam("titel", schema.Text()),
			web.Returns(http.StatusOK, schema.Text())),
		func(string) webEffect[string] {
			return effect.For[effect.Unit, Refusal]().Succeed("never")
		},
	)
	if err := assembled(broken); err == nil {
		t.Fatal("expected the endpoint's fault to stop the assembly")
	}
	if broken.Declaration().Path != "/books/{title}" {
		t.Fatalf("expected the declaration to survive for reporting, got %#v", broken.Declaration())
	}
}

func TestARouteWithoutAHandlerIsRefused(t *testing.T) {
	route := web.Handle[effect.Unit, Refusal, effect.Unit, string](
		web.GET("/books", web.Nothing(), web.Returns(http.StatusOK, schema.Text())), nil)
	if err := assembled(route); err == nil {
		t.Fatal("expected a route with no handler to be refused")
	}
}
