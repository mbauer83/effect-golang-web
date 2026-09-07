package acceptance

// The contract the bookstore publishes. It is the projection of the same
// declarations that dispatch a request, so a client that reads it and a client
// that calls the API are reading one description.

import (
	"context"
	"net/http"
	"testing"

	kin "github.com/getkin/kin-openapi/openapi3"

	"github.com/mbauer83/effect-golang-web/examples/bookstore"
)

func TestTheSurfaceDescribesItselfForAPublishedContract(t *testing.T) {
	// Dispatch and the document come from the same declarations, which is why
	// separating an endpoint from its handler was worth doing.
	surface, err := bookstore.Surface(bookstore.NewStore())
	if err != nil {
		t.Fatal(err)
	}

	declarations := surface.Declarations()
	if len(declarations) != 3 {
		t.Fatalf("expected three routes described, got %d", len(declarations))
	}
	for _, declared := range declarations {
		if declared.Summary == "" {
			t.Errorf("%s %s has no summary", declared.Method, declared.Path)
		}
		if declared.Status == 0 {
			t.Errorf("%s %s does not say what it answers with", declared.Method, declared.Path)
		}
	}
	found := declarations[2]
	if found.Path != "/books/{title}" || len(found.Parameters) != 1 ||
		found.Parameters[0].Doc == "" {
		t.Fatalf("expected the path parameter described, got %#v", found)
	}
}

func TestTheServedContractIsValidAndDescribesTheRoutesThatServeIt(t *testing.T) {
	// The document a client fetches is the projection of the same declarations
	// that dispatch its requests, and it is checked by a parser that has never
	// seen this module.
	base := running(t, bookstore.NewStore())
	response := get(t, base+"/openapi.json")

	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.StatusCode)
	}
	if kind := response.Header.Get("Content-Type"); kind != "application/json" {
		t.Fatalf("unexpected content type %q", kind)
	}

	document := read(t, response)
	loaded, err := kin.NewLoader().LoadFromData(document)
	if err != nil {
		t.Fatalf("the served contract does not load: %v\n%s", err, document)
	}
	if err := loaded.Validate(context.Background()); err != nil {
		t.Fatalf("the served contract is not valid: %v\n%s", err, document)
	}
	for _, path := range []string{"/books", "/books/{title}"} {
		if loaded.Paths.Find(path) == nil {
			t.Errorf("the contract does not describe %s", path)
		}
	}
	// The contract describes the API and not the route that serves it.
	if loaded.Paths.Find("/openapi.json") != nil {
		t.Error("the contract describes its own route")
	}
}
