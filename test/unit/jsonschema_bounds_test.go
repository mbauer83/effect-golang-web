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
