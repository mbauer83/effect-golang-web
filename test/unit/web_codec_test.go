package unit

// Reading a request. Each part is described once, by the same Schema that
// describes a field of a body, and combining two parts keeps both.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mbauer83/effect-golang-web/schema"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

// read decodes a codec against a request, which is what a route does before it
// reaches a handler.
func read[A any](t *testing.T, codec web.Codec[A], request *http.Request, captures map[string]string) (A, error) {
	t.Helper()
	return web.Decode(codec, web.RequestFrom(request).WithCaptures(captures))
}

func TestAPathParameterIsDecodedByItsSchema(t *testing.T) {
	// A path segment arrives as text and the schema says what it means, so a
	// route does not parse and a handler does not convert.
	codec := web.PathParam("pages", schema.Int())
	request := httptest.NewRequest(http.MethodGet, "/books/632", nil)

	pages, err := read(t, codec, request, map[string]string{"pages": "632"})
	if err != nil || pages != 632 {
		t.Fatalf("expected 632, got %v (%v)", pages, err)
	}

	if _, err := read(t, codec, request, map[string]string{"pages": "many"}); err == nil {
		t.Fatal("expected text that is not a number to be refused")
	} else if !strings.Contains(err.Error(), "path parameter pages") ||
		!strings.Contains(err.Error(), "whole number") {
		t.Fatalf("expected the parameter and the reason named, got %v", err)
	}
}

func TestARequiredQueryParameterThatIsMissingIsRefusedAndNamed(t *testing.T) {
	codec := web.QueryParam("shelf", schema.Text())

	if _, err := read(t, codec, httptest.NewRequest(http.MethodGet, "/books", nil), nil); err == nil {
		t.Fatal("expected a missing required parameter to be refused")
	} else if !strings.Contains(err.Error(), "query parameter shelf") {
		t.Fatalf("expected the parameter named, got %v", err)
	}
}

func TestAnOptionalParameterTellsAbsentFromEmpty(t *testing.T) {
	// ?shelf= carries an empty value and no shelf at all carries none. A codec
	// that could not tell them apart would make one of the two unexpressible.
	codec := web.OptionalQueryParam("shelf", schema.Text())

	absent, err := read(t, codec, httptest.NewRequest(http.MethodGet, "/books", nil), nil)
	if err != nil || absent != nil {
		t.Fatalf("expected nil for an absent parameter, got %v (%v)", absent, err)
	}
	empty, err := read(t, codec, httptest.NewRequest(http.MethodGet, "/books?shelf=", nil), nil)
	if err != nil || empty == nil || *empty != "" {
		t.Fatalf("expected an empty value to be present, got %v (%v)", empty, err)
	}
}

func TestAHeaderIsReadCaseInsensitivelyAsHTTPRequires(t *testing.T) {
	codec := web.HeaderParam("X-Shelf", schema.Text())
	request := httptest.NewRequest(http.MethodGet, "/books", nil)
	request.Header.Set("x-shelf", "one")

	shelf, err := read(t, codec, request, nil)
	if err != nil || shelf != "one" {
		t.Fatalf("expected the header read, got %q (%v)", shelf, err)
	}
}

func TestTheEntityIsDecodedByItsSchemaStraightFromTheBody(t *testing.T) {
	codec := web.Entity(bookSchema)
	document := `{"title":"T","authors":["A"],"pages":1,"hasIndex":false}`
	request := httptest.NewRequest(http.MethodPost, "/books", strings.NewReader(document))

	book, err := read(t, codec, request, nil)
	if err != nil || book.Title != "T" {
		t.Fatalf("expected the decoded entity, got %#v (%v)", book, err)
	}

	broken := httptest.NewRequest(http.MethodPost, "/books", strings.NewReader(`{"title":7}`))
	if _, err := read(t, codec, broken, nil); err == nil {
		t.Fatal("expected a mis-shapen entity to be refused")
	} else if !strings.Contains(err.Error(), "request body") ||
		!strings.Contains(err.Error(), "title") {
		t.Fatalf("expected the body and the field named, got %v", err)
	}
}

func TestTwoCodecsCombineIntoOneThatKeepsBothParts(t *testing.T) {
	type query struct {
		Shelf string
		Page  int
	}
	codec := web.Convert(
		web.Both(web.QueryParam("shelf", schema.Text()), web.QueryParam("page", schema.Int())),
		func(parts effect.Product[string, int]) (query, error) {
			return query{Shelf: parts.First, Page: parts.Second}, nil
		},
	)

	decoded, err := read(t, codec, httptest.NewRequest(http.MethodGet, "/books?shelf=one&page=2", nil), nil)
	if err != nil || decoded != (query{Shelf: "one", Page: 2}) {
		t.Fatalf("unexpected value: %#v (%v)", decoded, err)
	}
	// Both parts are described, in the order they were declared, because a
	// document is projected from the same declaration that reads the request.
	parameters := codec.Parameters()
	if len(parameters) != 2 || parameters[0].Name != "shelf" || parameters[1].Name != "page" {
		t.Fatalf("unexpected parameters: %#v", parameters)
	}
}

func TestCodecDeclarationMistakesAreReportedRatherThanPanicking(t *testing.T) {
	cases := map[string]error{
		"the zero codec": web.ValidateCodec(web.Codec[string]{}),
		"a nameless parameter": web.ValidateCodec(
			web.QueryParam("", schema.Text())),
		"a parameter with an unusable schema": web.ValidateCodec(
			web.QueryParam("shelf", schema.Schema[string]{})),
		"a compound schema as a parameter": func() error {
			_, err := read(t, web.QueryParam("shelf", bookSchema),
				httptest.NewRequest(http.MethodGet, "/books?shelf=one", nil), nil)
			return err
		}(),
		"two codecs reading the entity": web.ValidateCodec(
			web.Both(web.Entity(bookSchema), web.Entity(bookSchema))),
		"two codecs reading one parameter": web.ValidateCodec(
			web.Both(web.QueryParam("shelf", schema.Text()), web.QueryParam("shelf", schema.Text()))),
		"prose for more than one parameter": web.ValidateCodec(
			web.Describing(web.Both(
				web.QueryParam("shelf", schema.Text()),
				web.QueryParam("page", schema.Int())), "both of them")),
	}
	for mistake, err := range cases {
		if err == nil {
			t.Errorf("expected %s to be reported", mistake)
		}
	}
}

func TestProseOnAParameterReachesItsDeclaration(t *testing.T) {
	codec := web.Describing(web.QueryParam("shelf", schema.Text()), "which shelf to list")

	if err := web.ValidateCodec(codec); err != nil {
		t.Fatal(err)
	}
	if doc := codec.Parameters()[0].Doc; doc != "which shelf to list" {
		t.Fatalf("expected the prose carried, got %q", doc)
	}
}
