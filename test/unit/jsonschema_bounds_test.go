package unit

// Constraints and formats in the published document. The projection and the
// codec have to agree about which values a shape admits as they agree about the
// shape itself, so the external validator is asked about both sides of every
// bound.

import (
	"strings"
	"testing"

	"github.com/mbauer83/effect-golang-web/schema"
	"github.com/mbauer83/effect-golang-web/schema/jsonschema"
)

func TestConstraintsBecomeTheKeywordsThatSayTheSameThing(t *testing.T) {
	// The projection and the codec must agree about a bound as they agree about
	// a shape, so the validator is asked about both sides of it.
	type constrained struct {
		Code  string
		Pages int
		Tags  []string
	}
	bounded := schema.Struct[constrained]("Reading",
		schema.FieldOf("code", schema.Matching(schema.MaxLength(schema.Text(), 7), `^[A-Z]{2}-[0-9]{4}$`),
			func(value constrained) string { return value.Code },
			func(value *constrained, code string) { value.Code = code }),
		schema.FieldOf("pages", schema.AtMost(schema.AtLeast(schema.Int(), 1), 100),
			func(value constrained) int { return value.Pages },
			func(value *constrained, pages int) { value.Pages = pages }),
		schema.FieldOf("tags", schema.MinItems(schema.List(schema.Text()), 1),
			func(value constrained) []string { return value.Tags },
			func(value *constrained, tags []string) { value.Tags = tags }),
	)

	rendered, err := jsonschema.Project(bounded.Structure()).Render()
	if err != nil {
		t.Fatal(err)
	}
	for _, keyword := range []string{
		`"maxLength":7`, `"pattern":"^[A-Z]{2}-[0-9]{4}$"`,
		`"minimum":1`, `"maximum":100`, `"minItems":1`,
	} {
		if !strings.Contains(string(rendered), keyword) {
			t.Errorf("expected %s in the document:\n%s", keyword, rendered)
		}
	}

	emitted := compiled(t, bounded.Structure())
	admitted := `{"code":"AB-1234","pages":50,"tags":["one"]}`
	if err := emitted.Validate(instance(t, admitted)); err != nil {
		t.Errorf("the projection refuses what the codec admits: %v", err)
	}
	if _, err := schema.DecodeJSON(bounded, []byte(admitted)); err != nil {
		t.Errorf("the codec refuses what the projection admits: %v", err)
	}

	for description, document := range map[string]string{
		"a code that does not match":   `{"code":"ab-1234","pages":1,"tags":["one"]}`,
		"a page count below the bound": `{"code":"AB-1234","pages":0,"tags":["one"]}`,
		"a page count above the bound": `{"code":"AB-1234","pages":101,"tags":["one"]}`,
		"no tags at all":               `{"code":"AB-1234","pages":1,"tags":[]}`,
	} {
		if err := emitted.Validate(instance(t, document)); err == nil {
			t.Errorf("the projection admits %s, which the codec refuses", description)
		}
		if _, err := schema.DecodeJSON(bounded, []byte(document)); err == nil {
			t.Errorf("the codec admits %s, which the projection refuses", description)
		}
	}
}

func TestAFormatReachesTheDocumentAsAnnotationAndAsRule(t *testing.T) {
	// A format is two claims. The document carries the name for a reader that
	// acts on it, and the expression for a validator that does not -- and the
	// external validator, which checks pattern and ignores format, agrees with
	// the codec because of the second.
	type identified struct{ ID string }
	shape := schema.Struct[identified]("Identified",
		schema.FieldOf("id", schema.UUID(),
			func(value identified) string { return value.ID },
			func(value *identified, id string) { value.ID = id }),
	)

	rendered, err := jsonschema.Project(shape.Structure()).Render()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rendered), `"format":"uuid"`) {
		t.Errorf("expected the format annotated:\n%s", rendered)
	}
	if !strings.Contains(string(rendered), `"pattern":`) {
		t.Errorf("expected the rule expressed as well:\n%s", rendered)
	}

	emitted := compiled(t, shape.Structure())
	admitted := `{"id":"123e4567-e89b-12d3-a456-426614174000"}`
	refused := `{"id":"not-a-uuid"}`
	if err := emitted.Validate(instance(t, admitted)); err != nil {
		t.Errorf("the projection refuses what the codec admits: %v", err)
	}
	if err := emitted.Validate(instance(t, refused)); err == nil {
		t.Error("the projection admits what the codec refuses")
	}
	if _, err := schema.DecodeJSON(shape, []byte(refused)); err == nil {
		t.Error("the codec admits what the projection refuses")
	}
}

func TestATaggedUnionProjectsAsTheWireFormItWrites(t *testing.T) {
	// Each alternative is the variant's own shape and the field that names it,
	// which is what allOf is for: the variant is a component, and a reference
	// has nothing to add a property to. The const is what validates.
	rendered, err := jsonschema.Project(toleranceSchema.Structure()).Render()
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`"allOf"`, `"$ref":"#/$defs/ISO2768"`, `"const":"iso2768"`,
		`"discriminator":{"propertyName":"type"}`,
	} {
		if !strings.Contains(string(rendered), expected) {
			t.Errorf("expected %s in the document:\n%s", expected, rendered)
		}
	}
}

func TestTheEmittedTaggedUnionAgreesWithTheCodec(t *testing.T) {
	emitted := compiled(t, toleranceSchema.Structure())

	admitted := map[string]string{
		"a variant and its field": `{"type":"iso2768","grade":"medium"}`,
		"the field written last":  `{"grade":"medium","type":"iso2768"}`,
		"the other variant":       `{"type":"iso10800","class":3}`,
	}
	for description, document := range admitted {
		if err := emitted.Validate(instance(t, document)); err != nil {
			t.Errorf("the projection refuses %s: %v", description, err)
		}
		if _, err := schema.DecodeJSON(toleranceSchema, []byte(document)); err != nil {
			t.Errorf("the codec refuses %s: %v", description, err)
		}
	}

	refused := map[string]string{
		"no naming field":           `{"grade":"medium"}`,
		"a name no variant has":     `{"type":"iso286","grade":"medium"}`,
		"a field the variant lacks": `{"type":"iso2768","class":3}`,
	}
	for description, document := range refused {
		if err := emitted.Validate(instance(t, document)); err == nil {
			t.Errorf("the projection admits %s, which the codec refuses", description)
		}
		if _, err := schema.DecodeJSON(toleranceSchema, []byte(document)); err == nil {
			t.Errorf("the codec admits %s, which the projection refuses", description)
		}
	}
}

func TestADescribedTaggedUnionBehavesTheSame(t *testing.T) {
	// The field is part of the description, so a schema used without its Go
	// type reads and writes the same wire form.
	described := schema.Dynamic(toleranceSchema.Structure())
	document := `{"grade":"medium","type":"iso2768"}`

	value, err := schema.DecodeJSON(described, []byte(document))
	if err != nil {
		t.Fatal(err)
	}
	written, err := schema.EncodeJSON(described, value)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(written)); got != `{"type":"iso2768","grade":"medium"}` {
		t.Fatalf("unexpected document: %s", got)
	}
}

func TestAConstantIsCarriedAsATypedFieldOfTheDocument(t *testing.T) {
	// The document is a typed model, so a consumer reads the value pinning a
	// variant to its name without parsing anything.
	// A named union is a component, and the root refers to it.
	alternatives := jsonschema.Project(toleranceSchema.Structure()).
		Components["Tolerance"].OneOf
	if len(alternatives) != 2 {
		t.Fatalf("expected one alternative per variant, got %#v", alternatives)
	}
	naming := alternatives[0].AllOf[1]
	var pinned string = naming.Properties[0].Schema.Const
	if pinned != "iso2768" {
		t.Fatalf("expected the variant's name pinned, got %q", pinned)
	}
}
