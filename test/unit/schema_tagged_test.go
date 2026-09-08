package unit

// A union told apart by a field inside it, which is the common REST shape.
// What matters is that the field may arrive anywhere in the object, that the
// union writes it rather than the variant carrying it, and that a format which
// cannot read an object whole says so instead of guessing.

import (
	"strings"
	"testing"

	"github.com/mbauer83/effect-golang-web/schema"
	"github.com/mbauer83/effect-golang-web/schema/dynamic"
	"github.com/mbauer83/effect-golang-web/schema/structure"
)

// Tolerance is a sum whose wire form names the variant in a field.
type Tolerance interface{ tolerance() }

type ISO2768 struct{ Grade string }

type ISO10800 struct{ Class int }

func (ISO2768) tolerance()  {}
func (ISO10800) tolerance() {}

// The Go types carry no tag: which variant a value is, is the type it is, and
// the field is the union's business.
var iso2768Schema = schema.Struct[ISO2768]("ISO2768",
	schema.FieldOf("grade", schema.Text(),
		func(value ISO2768) string { return value.Grade },
		func(value *ISO2768, grade string) { value.Grade = grade }),
)

var iso10800Schema = schema.Struct[ISO10800]("ISO10800",
	schema.FieldOf("class", schema.Int(),
		func(value ISO10800) int { return value.Class },
		func(value *ISO10800, class int) { value.Class = class }),
)

var toleranceSchema = schema.OneOfBy[Tolerance]("Tolerance", "type",
	schema.VariantOf("iso2768", iso2768Schema,
		func(value Tolerance) (ISO2768, bool) { held, is := value.(ISO2768); return held, is },
		func(held ISO2768) Tolerance { return held }),
	schema.VariantOf("iso10800", iso10800Schema,
		func(value Tolerance) (ISO10800, bool) { held, is := value.(ISO10800); return held, is },
		func(held ISO10800) Tolerance { return held }),
)

func TestATaggedUnionWritesTheNameIntoTheVariantsObject(t *testing.T) {
	document, err := schema.EncodeJSON[Tolerance](toleranceSchema, ISO2768{Grade: "medium"})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"type":"iso2768","grade":"medium"}`
	if got := strings.TrimSpace(string(document)); got != want {
		t.Fatalf("expected\n  %s\ngot\n  %s", want, got)
	}
}

func TestATaggedUnionRoundTripsEachVariant(t *testing.T) {
	for _, original := range []Tolerance{ISO2768{Grade: "fine"}, ISO10800{Class: 3}} {
		document, err := schema.EncodeJSON[Tolerance](toleranceSchema, original)
		if err != nil {
			t.Fatalf("%#v: %v", original, err)
		}
		decoded, err := schema.DecodeJSON(toleranceSchema, document)
		if err != nil {
			t.Fatalf("%#v: %v", original, err)
		}
		if decoded != original {
			t.Fatalf("round trip changed the value:\n  before %#v\n  after  %#v", original, decoded)
		}
	}
}

func TestTheNameMayArriveAfterTheFieldsItSettles(t *testing.T) {
	// The whole difficulty of this wire form: a decoder cannot know what
	// "grade" means until it has seen "type", and a producer is free to put
	// them in either order. Reading the object first is what makes it work.
	for _, order := range []string{
		`{"type":"iso2768","grade":"medium"}`,
		`{"grade":"medium","type":"iso2768"}`,
	} {
		decoded, err := schema.DecodeJSON(toleranceSchema, []byte(order))
		if err != nil {
			t.Fatalf("%s: %v", order, err)
		}
		if decoded != (Tolerance)(ISO2768{Grade: "medium"}) {
			t.Fatalf("%s: unexpected value %#v", order, decoded)
		}
	}
}

func TestATaggedUnionRefusesWhatItCannotPlace(t *testing.T) {
	for reason, document := range map[string]string{
		"no naming field":           `{"grade":"medium"}`,
		"a name no variant has":     `{"type":"iso286","grade":"medium"}`,
		"a name that is not text":   `{"type":7,"grade":"medium"}`,
		"a field the variant lacks": `{"type":"iso2768","class":3}`,
		"not an object at all":      `"iso2768"`,
	} {
		if _, err := schema.DecodeJSON(toleranceSchema, []byte(document)); err == nil {
			t.Errorf("expected %s to be refused", reason)
		}
	}
}

func TestATaggedUnionSaysWhichFieldNamesTheVariant(t *testing.T) {
	shape, isUnion := toleranceSchema.Structure().(structure.Union)
	if !isUnion {
		t.Fatalf("expected a union, got %#v", toleranceSchema.Structure())
	}
	if shape.Discriminator != "type" {
		t.Fatalf("expected the field recorded, got %q", shape.Discriminator)
	}
	// The other form records none, which is how the two are told apart.
	if plain := shapeSchema.Structure().(structure.Union); plain.Discriminator != "" {
		t.Fatalf("expected no field on the other form, got %q", plain.Discriminator)
	}
}

func TestTaggedUnionDeclarationMistakesAreReported(t *testing.T) {
	circle := schema.VariantOf("circle", circleSchema,
		func(shape Shape) (Circle, bool) { held, is := shape.(Circle); return held, is },
		func(held Circle) Shape { return held })

	cases := map[string]schema.Schema[Shape]{
		"no naming field": schema.OneOfBy[Shape]("Shape", "", circle),
		// A variant that already has the field would have two of them, and
		// which one won is not something to leave to chance.
		"a variant that already has the field": schema.OneOfBy[Shape]("Shape", "radius", circle),
		"no variants":                          schema.OneOfBy[Shape]("Shape", "type"),
	}
	for mistake, declared := range cases {
		if err := schema.Validate(declared); err == nil {
			t.Errorf("expected %s to be reported", mistake)
		}
	}
}

func TestAFormatThatCannotReadAnObjectWholeSaysSo(t *testing.T) {
	// The trace format streams: it answers what the schema asks for and cannot
	// hand over a value it was not told the shape of. A tagged union needs
	// exactly that, so it is refused rather than half-decoded.
	source := &traceSource{tokens: []string{
		"{", "k:type", "s:iso2768", "k:grade", "s:medium", "}",
	}}
	_, err := schema.Decode(toleranceSchema, source)
	if err == nil {
		t.Fatal("expected the format to refuse a shape it cannot read whole")
	}
	if !strings.Contains(err.Error(), "cannot read a union told apart by a field") {
		t.Fatalf("expected the reason to say so, got %v", err)
	}
}

func TestTheBufferingCapabilityIsTheOneASourceIsAskedFor(t *testing.T) {
	// The capability is named so a format author can implement it deliberately
	// rather than discovering by a refusal that something was expected.
	var buffering schema.Buffering = &bufferingSource{}
	held, err := buffering.Buffer()
	if err != nil {
		t.Fatal(err)
	}
	if held != (dynamic.Text{Value: "read whole"}) {
		t.Fatalf("unexpected value: %#v", held)
	}
}

// bufferingSource is the smallest thing that can hand over a value it was not
// told the shape of, which is all the capability asks.
type bufferingSource struct{ traceSource }

func (*bufferingSource) Buffer() (dynamic.Value, error) {
	return dynamic.Text{Value: "read whole"}, nil
}
