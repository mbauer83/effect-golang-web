package unit

// What has no derived shape, as opposed to one whose derived shape is empty.

import (
	"testing"

	"github.com/mbauer83/effect-golang-web/schema"
	"github.com/mbauer83/effect-golang-web/schema/dynamic"
	"github.com/mbauer83/effect-golang-web/schema/structure"
	"github.com/mbauer83/effect-golang-web/schema/variant"
)

func TestADescriptionWithNothingToDeriveFromSaysSo(t *testing.T) {
	// Each refusal is a description that cannot have a derived shape, rather
	// than one whose derived shape happens to be empty.
	if _, err := variant.Create(schema.Text().Structure()); err == nil {
		t.Error("expected a scalar to be refused: it has one shape in every role")
	}

	// Every field computed, so the create shape would hold nothing. That is
	// almost always a mistake in the marks rather than a shape worth
	// publishing, so it is named.
	allComputed := schema.Struct[dynamic.Value]("Stamped",
		schema.DescribedField("at", schema.Time()).Computed(),
		schema.DescribedField("by", schema.Text()).Computed(),
	)
	if _, err := variant.Create(allComputed.Structure()); err == nil {
		t.Error("expected a shape with nothing left to be refused")
	}

	// A name with nothing behind it: a derived shape has to be built, so
	// passing the reference through would publish a shape nobody can see into.
	unresolved := structure.Object{Name: "Holder", Fields: []structure.Field{
		{Name: "inner", Node: structure.Reference{Name: "Elsewhere"}},
	}}
	if _, err := variant.Create(structure.Reference{Name: "Elsewhere"}); err == nil {
		t.Error("expected an unresolved reference at the root to be refused")
	}
	// Not at a field, though: a reference to a value object is a shape this
	// walk leaves alone, so only an entity behind an unresolvable name is a
	// problem -- and this one is not reached into.
	if _, err := variant.Create(unresolved); err != nil {
		t.Errorf("expected a referenced value object to pass through, got %v", err)
	}
}
