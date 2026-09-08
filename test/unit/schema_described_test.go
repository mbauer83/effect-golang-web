package unit

// A described union, and what a description publishes. Both matter for the same
// reason: a schema with no Go type has to behave and describe itself exactly as
// one with a type does, or generating a struct from it would be generating
// something else.

import (
	"strings"
	"testing"

	"github.com/mbauer83/effect-golang-web/schema"
	"github.com/mbauer83/effect-golang-web/schema/dynamic"
	"github.com/mbauer83/effect-golang-web/schema/jsonschema"
)

// shapeDescription is a union with no Go type, whose alternatives are
// themselves descriptions.
var shapeDescription = schema.Choice("Shape",
	schema.DocumentedAlternative("a circle, by its radius",
		schema.AlternativeOf("circle", schema.Record("Circle",
			schema.MemberOf("radius", schema.Above(schema.Float64(), 0))))),
	schema.AlternativeOf("rectangle", schema.Record("Rectangle",
		schema.MemberOf("width", schema.Above(schema.Float64(), 0)),
		schema.MemberOf("height", schema.Above(schema.Float64(), 0)))),
)

func TestADescribedUnionRoundTripsAndRefusesTheSameThings(t *testing.T) {
	document := `{"circle":{"radius":2}}`
	read, err := schema.DecodeJSON(shapeDescription, []byte(document))
	if err != nil {
		t.Fatal(err)
	}
	// A union's value is the one-member object its wire form is, and the
	// member's name is the variant's.
	chosen, only := read.(dynamic.Object).Only()
	if !only || chosen.Name != "circle" {
		t.Fatalf("expected the chosen variant, got %#v", read)
	}

	written, err := schema.EncodeJSON(shapeDescription, read)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(written)); got != document {
		t.Fatalf("expected %s, got %s", document, got)
	}

	for reason, refused := range map[string]string{
		"no variant":                      `{}`,
		"an unknown variant":              `{"triangle":{"base":1}}`,
		"two variants":                    `{"circle":{"radius":1},"rectangle":{"width":1,"height":1}}`,
		"a bound broken inside a variant": `{"circle":{"radius":0}}`,
	} {
		if _, err := schema.DecodeJSON(shapeDescription, []byte(refused)); err == nil {
			t.Errorf("expected %s to be refused", reason)
		}
	}
}

func TestADescriptionProjectsAsAnyOtherSchemaDoes(t *testing.T) {
	// The projections read the description and nothing else, so a schema with
	// no Go type publishes exactly as one with a type does. That is what a
	// loaded description has to do to be worth serving from.
	document := jsonschema.Project(bookDescription.Structure())

	component, described := document.Components["Book"]
	if !described {
		t.Fatalf("expected a Book component, got %v", document.ComponentNames())
	}
	if strings.Join(component.Required, ",") != "title,authors,pages,id" {
		t.Fatalf("expected the optional member excluded, got %v", component.Required)
	}
	if component.Properties[0].Schema.Bounds.MinLength == nil {
		t.Fatalf("expected the recorded bound published, got %#v", component.Properties[0])
	}
	if component.Properties[0].Schema.Description != "what the book is called" {
		t.Fatalf("expected the prose published, got %q", component.Properties[0].Schema.Description)
	}
}
