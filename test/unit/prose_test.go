package unit

// Prose is part of the description, not decoration: it is what a published
// contract shows a reader. These check it survives the trip from the
// declaration to the structure and on into a projection.

import (
	"errors"
	"strings"
	"testing"

	"github.com/mbauer83/effect-golang-web/schema"
	"github.com/mbauer83/effect-golang-web/schema/jsonschema"
	"github.com/mbauer83/effect-golang-web/schema/structure"
)

func TestProseOnAFieldReachesTheDescription(t *testing.T) {
	documented := schema.Struct[Detail]("Detail",
		schema.DocumentedField("what the note says",
			schema.FieldOf("note", schema.Text(),
				func(detail Detail) string { return detail.Note },
				func(detail *Detail, note string) { detail.Note = note })),
	)

	object, isObject := schema.Documented("one note", documented).Structure().(structure.Object)
	if !isObject {
		t.Fatalf("expected an object, got %#v", documented.Structure())
	}
	if object.Doc != "one note" {
		t.Fatalf("expected the object's prose, got %q", object.Doc)
	}
	var field structure.Field = object.Fields[0]
	if field.Doc != "what the note says" {
		t.Fatalf("expected the field's prose, got %q", field.Doc)
	}
}

func TestProseOnAUnionAndItsVariantsReachesTheDescription(t *testing.T) {
	union, isUnion := schema.Documented("a shape", shapeSchema).Structure().(structure.Union)
	if !isUnion {
		t.Fatalf("expected a union, got %#v", shapeSchema.Structure())
	}
	if union.Doc != "a shape" {
		t.Fatalf("expected the union's prose, got %q", union.Doc)
	}
	var variant structure.Variant = union.Variants[0]
	if variant.Doc != "a circle, by its radius" {
		t.Fatalf("expected the variant's prose, got %q", variant.Doc)
	}
}

func TestProseAndTheDialectReachTheRenderedDocument(t *testing.T) {
	document := jsonschema.Project(everyShapeSchema.Structure())

	var property jsonschema.Property = document.Components["Detail"].Properties[0]
	if property.Name != "note" {
		t.Fatalf("expected the property named, got %#v", property)
	}

	rendered, err := document.Render()
	if err != nil {
		t.Fatal(err)
	}
	// The dialect is declared so a validator does not have to be told which one
	// to apply; the pair test relies on exactly this.
	if !strings.Contains(string(rendered), `"$schema":"`+jsonschema.Dialect+`"`) {
		t.Fatalf("expected the dialect declared, got\n%s", rendered)
	}
}

func TestARejectionStatesItsReasonSeparatelyFromItsPath(t *testing.T) {
	// A caller that renders its own message needs the reason and the path apart,
	// not one string it has to take apart again.
	_, err := schema.DecodeJSON(bookSchema, []byte(`{"title":7}`))
	var failure *schema.Error
	if !errors.As(err, &failure) {
		t.Fatalf("expected a schema failure, got %v", err)
	}
	if failure.Reason == "" || len(failure.Path) != 1 {
		t.Fatalf("expected a reason and a path, got %#v", failure)
	}
}
