package unit

// What the proto3 projection refuses, and why each refusal is the right answer
// rather than a limitation.
//
// Protobuf is the one projection here where the contract is the number and the
// name rather than the shape: renaming a Go field is safe and renumbering it is
// not. So the things this refuses are the things that would have made a file
// which compiles today and means something else next release.

import (
	"strings"
	"testing"

	"github.com/mbauer83/effect-golang-web/schema"
	"github.com/mbauer83/effect-golang-web/schema/dynamic"
	"github.com/mbauer83/effect-golang-web/schema/protobuf"
)

func TestTheProjectionRefusesWhatWouldNotStaySaid(t *testing.T) {
	for named, expected := range map[string]struct {
		shape  schema.Schema[dynamic.Value]
		reason string
	}{
		"a field with no number": {
			shape: schema.Struct[dynamic.Value]("Unnumbered",
				schema.DescribedField("weight", schema.Float64())),
			reason: "would change when the declaration was reordered",
		},
		"an object with no name": {
			shape: schema.Struct[dynamic.Value]("Holder",
				schema.DescribedField("inner",
					schema.Struct[dynamic.Value]("", schema.DescribedField("x", schema.Int32()).Numbered(1))).
					Numbered(1)),
			reason: "would change when that field did",
		},
		"a list of lists": {
			shape: schema.Struct[dynamic.Value]("Nested",
				schema.DescribedField("rows",
					schema.List(schema.List(schema.Int32()))).Numbered(1)),
			reason: "no repeated repeated",
		},
		"a variant that is a list": {
			shape: schema.OneOf[dynamic.Value]("Choice",
				schema.DescribedVariant("many", schema.List(schema.Int32())).Numbered(1)),
			reason: "cannot be repeated",
		},
	} {
		_, err := protobuf.Project(expected.shape.Structure(), "test.v1")
		if err == nil {
			t.Errorf("expected %s to be refused", named)
			continue
		}
		if !strings.Contains(err.Error(), expected.reason) {
			t.Errorf("%s: expected the reason to mention %q, got %v",
				named, expected.reason, err)
		}
	}
}

func TestAUnionWithADiscriminatingFieldIsRefused(t *testing.T) {
	// Protobuf identifies the chosen member of a oneof by its field number. A
	// discriminating field would encode the same choice a second way, and the
	// two could contradict each other -- so the untagged form is the one that
	// projects, and this says so rather than emitting a message with a
	// redundant field in it.
	tagged := schema.OneOfBy[dynamic.Value]("Tolerance", "type",
		schema.DescribedVariant("iso2768", schema.Struct[dynamic.Value]("Iso2768",
			schema.DescribedField("grade", schema.Text()).Numbered(1))).Numbered(1),
	)

	_, err := protobuf.Project(tagged.Structure(), "test.v1")
	if err == nil {
		t.Fatal("expected a discriminated union to be refused")
	}
	if !strings.Contains(err.Error(), "encode that choice twice") {
		t.Fatalf("expected the reason to say so, got %v", err)
	}
}

func TestAScalarAtTheRootIsRefusedBecauseAMessageIsTheUnit(t *testing.T) {
	// Protobuf transfers messages. A bare string is not one, and there is no
	// message this could invent that would not need a name of its own.
	_, err := protobuf.Project(schema.Text().Structure(), "test.v1")
	if err == nil {
		t.Fatal("expected a scalar root to be refused")
	}
	if !strings.Contains(err.Error(), "wrap it in a named struct") {
		t.Fatalf("expected the reason to say what to do, got %v", err)
	}
}
