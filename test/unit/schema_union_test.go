package unit

// A union: how a Go sum is described, what it puts on the wire, and what it
// refuses. The wire form names the chosen variant as an object's single member,
// which is what lets a one-pass decoder know what it is reading before it
// reads it.

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mbauer83/effect-golang-web/schema"
)

// Shape is a sum modelled the way Go models one: an interface with a private
// marker and a concrete type per alternative.
type Shape interface{ shape() }

type Circle struct{ Radius float64 }

type Rectangle struct {
	Width  float64
	Height float64
}

func (Circle) shape()    {}
func (Rectangle) shape() {}

var circleSchema = schema.Struct[Circle]("Circle",
	schema.FieldOf("radius", schema.Float64(),
		func(circle Circle) float64 { return circle.Radius },
		func(circle *Circle, radius float64) { circle.Radius = radius }),
)

var rectangleSchema = schema.Struct[Rectangle]("Rectangle",
	schema.FieldOf("width", schema.Float64(),
		func(rectangle Rectangle) float64 { return rectangle.Width },
		func(rectangle *Rectangle, width float64) { rectangle.Width = width }),
	schema.FieldOf("height", schema.Float64(),
		func(rectangle Rectangle) float64 { return rectangle.Height },
		func(rectangle *Rectangle, height float64) { rectangle.Height = height }),
)

var shapeSchema = schema.OneOf[Shape]("Shape",
	schema.DocumentedVariant("a circle, by its radius",
		schema.VariantOf("circle", circleSchema,
			func(shape Shape) (Circle, bool) { circle, is := shape.(Circle); return circle, is },
			func(circle Circle) Shape { return circle })),
	schema.VariantOf("rectangle", rectangleSchema,
		func(shape Shape) (Rectangle, bool) { rectangle, is := shape.(Rectangle); return rectangle, is },
		func(rectangle Rectangle) Shape { return rectangle }),
)

func TestAUnionRoundTripsEachVariant(t *testing.T) {
	for _, original := range []Shape{Circle{Radius: 2.5}, Rectangle{Width: 3, Height: 4}} {
		decoded, err := roundTripShape(t, original)
		if err != nil {
			t.Fatalf("%#v: %v", original, err)
		}
		if !reflect.DeepEqual(decoded, original) {
			t.Fatalf("round trip changed the value:\n  before %#v\n  after  %#v", original, decoded)
		}
	}
}

func roundTripShape(t *testing.T, original Shape) (Shape, error) {
	t.Helper()
	document, err := schema.EncodeJSON(shapeSchema, original)
	if err != nil {
		return nil, err
	}
	return schema.DecodeJSON(shapeSchema, document)
}

func TestAUnionNamesTheVariantAsTheSingleMember(t *testing.T) {
	document, err := schema.EncodeJSON[Shape](shapeSchema, Circle{Radius: 2})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"circle":{"radius":2}}`
	if got := strings.TrimSpace(string(document)); got != want {
		t.Fatalf("expected\n  %s\ngot\n  %s", want, got)
	}
}

func TestAValueNoVariantHoldsIsReportedRatherThanWrittenEmpty(t *testing.T) {
	_, err := schema.EncodeJSON[Shape](shapeSchema, unknownShape{})
	if err == nil {
		t.Fatal("expected a value outside the union to be rejected")
	}
	if !strings.Contains(err.Error(), "no variant matches") {
		t.Fatalf("expected the reason to say the value is outside the union, got %v", err)
	}
}

type unknownShape struct{}

func (unknownShape) shape() {}

func TestAnUnknownVariantIsRefusedRatherThanSkipped(t *testing.T) {
	// Unlike an unknown field, an unknown variant leaves nothing to build, so
	// tolerating it would produce a zero value that the document never named.
	_, err := schema.DecodeJSON(shapeSchema, []byte(`{"triangle":{"base":1}}`))
	if err == nil {
		t.Fatal("expected an unknown variant to be refused")
	}
	if !strings.Contains(err.Error(), "no variant is named triangle") {
		t.Fatalf("expected the reason to name the variant, got %v", err)
	}
}

func TestADocumentNamingTwoVariantsIsRefused(t *testing.T) {
	_, err := schema.DecodeJSON(shapeSchema,
		[]byte(`{"circle":{"radius":1},"rectangle":{"width":1,"height":1}}`))
	if err == nil {
		t.Fatal("expected two variants to be refused")
	}
	if !strings.Contains(err.Error(), "both circle and rectangle are present") {
		t.Fatalf("expected the reason to name both, got %v", err)
	}
}

func TestADocumentNamingNoVariantIsRefused(t *testing.T) {
	_, err := schema.DecodeJSON(shapeSchema, []byte(`{}`))
	if err == nil {
		t.Fatal("expected an empty object to be refused")
	}
	if !strings.Contains(err.Error(), "none is present") {
		t.Fatalf("expected the reason to say no variant was named, got %v", err)
	}
}

func TestAFailureInsideAVariantCarriesItsPath(t *testing.T) {
	_, err := schema.DecodeJSON(shapeSchema, []byte(`{"rectangle":{"width":1}}`))
	if err == nil {
		t.Fatal("expected a missing field inside a variant to be rejected")
	}
	path, _ := schema.PathOf(err)
	if !reflect.DeepEqual(path, []string{"rectangle", "height"}) {
		t.Fatalf("expected the path to reach through the variant, got %v from %v", path, err)
	}
}

func TestUnionDeclarationMistakesAreReportedRatherThanPanicking(t *testing.T) {
	circle := schema.VariantOf("circle", circleSchema,
		func(shape Shape) (Circle, bool) { circle, is := shape.(Circle); return circle, is },
		func(circle Circle) Shape { return circle })

	cases := map[string]schema.Schema[Shape]{
		"no variants":  schema.OneOf[Shape]("Shape"),
		"two circles":  schema.OneOf[Shape]("Shape", circle, circle),
		"nameless":     schema.OneOf[Shape]("Shape", schema.VariantOf[Shape, Circle]("", circleSchema, nil, nil)),
		"faulted case": schema.OneOf[Shape]("Shape", schema.VariantOf[Shape, Circle]("circle", schema.Schema[Circle]{}, nil, nil)),
	}
	for mistake, declared := range cases {
		if err := schema.Validate(declared); err == nil {
			t.Errorf("expected %s to be reported by Validate", mistake)
		}
	}
}
