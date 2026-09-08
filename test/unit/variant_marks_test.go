package unit

// What the marks say, and what they stop saying once a shape is derived.

import (
	"testing"

	"github.com/mbauer83/effect-golang-web/schema"
	"github.com/mbauer83/effect-golang-web/schema/dynamic"
	"github.com/mbauer83/effect-golang-web/schema/structure"
	"github.com/mbauer83/effect-golang-web/schema/variant"
)

func TestTheMarksDoNotSurviveIntoTheDerivedShape(t *testing.T) {
	// A shape a caller supplies has no identity to declare and nothing
	// computed left in it, so carrying the marks through would say something
	// untrue about it -- and a projection reading them would make a key out of
	// a field that is no longer one.
	created, err := variant.CreateWithEntities(order.Structure())
	if err != nil {
		t.Fatal(err)
	}
	object := created.(structure.Object)
	for _, field := range object.Fields {
		if field.Identity || field.Computed {
			t.Errorf("%q still carries a mark", field.Name)
		}
	}
	if object.IsEntity() {
		t.Error("a create shape is not an entity: it has no identity")
	}

	// The root has no marked field left to check -- id and placedAt were both
	// dropped -- so checking only the root proves nothing. The line's kept
	// identity is where a surviving mark would show, and a projection reading
	// it would make a key out of a field that is no longer one.
	lines := fieldNamed(t, created, "lines")
	element := lines.Node.(structure.Sequence).Element.(structure.Object)
	for _, field := range element.Fields {
		if field.Identity || field.Computed {
			t.Errorf("the line's %q still carries a mark", field.Name)
		}
	}
	if element.IsEntity() {
		t.Error("a derived line is not an entity either")
	}
}

func fieldNamed(t *testing.T, node structure.Node, name string) structure.Field {
	t.Helper()
	object, isObject := node.(structure.Object)
	if !isObject {
		t.Fatalf("expected an object, got %T", node)
	}
	for _, field := range object.Fields {
		if field.Name == name {
			return field
		}
	}
	t.Fatalf("no field named %q", name)
	return structure.Field{}
}

func TestTheDescriptionCarriesWhichDefaultWasDeclared(t *testing.T) {
	// A closed set of two, so a projection switches over it and knows it has
	// covered everything -- and a value rather than a SQL string, because a
	// string would be one dialect's spelling inside a description meant to
	// outlive the choice of dialect.
	stamped := schema.Struct[dynamic.Value]("Stamped",
		schema.DescribedField("at", schema.Time()).Computed().DefaultingToNow(),
		schema.DescribedField("status", schema.Text()).Defaulting(dynamic.OfText("new")),
	)
	object := stamped.Structure().(structure.Object)

	at := object.Fields[0]
	if _, now := at.Default.(structure.DefaultNow); !now {
		t.Errorf("expected the moment the row is written, got %#v", at.Default)
	}
	status := object.Fields[1]
	held, fixed := status.Default.(structure.DefaultTo)
	if !fixed {
		t.Fatalf("expected a fixed value, got %#v", status.Default)
	}
	if held.Value != dynamic.OfText("new") {
		t.Errorf("unexpected default: %#v", held.Value)
	}

	// A default and Computed are separate statements: a field can have a
	// default and still be the caller's to give, which is what a default is.
	if status.Computed {
		t.Error("Defaulting should not have made the field computed")
	}
	// So it survives a create shape, where the computed one does not.
	created, err := variant.Create(stamped.Structure())
	if err != nil {
		t.Fatal(err)
	}
	if got := named(t, created); len(got) != 1 || got[0] != "status" {
		t.Fatalf("unexpected fields: %v", got)
	}
}
