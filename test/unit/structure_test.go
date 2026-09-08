package unit

// The structure package is an extension point, not an implementation detail:
// a projection to Avro, to a migration or to a form renderer is written the
// same way the built-in ones are, with no privileged access. This test writes
// one -- a compact type description -- to hold that claim up.

import (
	"strconv"
	"strings"
	"testing"

	"github.com/mbauer83/effect-golang-web/schema"
	"github.com/mbauer83/effect-golang-web/schema/structure"
)

// describe renders a node as a one-line type expression. It is the smallest
// projection worth writing, and it has to handle every node the schema layer
// can produce.
func describe(node structure.Node, visiting map[string]bool) string {
	switch shape := node.(type) {
	case structure.Scalar:
		if shape.Format != "" {
			return shape.Kind.String() + "<" + shape.Format + ">"
		}
		return shape.Kind.String()
	case structure.Sequence:
		return "[" + describe(shape.Element, visiting) + "]"
	case structure.Mapping:
		return "{" + describe(shape.Key, visiting) + ": " + describe(shape.Value, visiting) + "}"
	case structure.Object:
		return describeObject(shape, visiting)
	case structure.Union:
		return describeUnion(shape, visiting)
	case structure.Nullable:
		return describe(shape.Inner, visiting) + "|null"
	case structure.Reference:
		return describeReference(shape, visiting)
	default:
		return "?"
	}
}

func describeObject(shape structure.Object, visiting map[string]bool) string {
	if shape.Name != "" {
		if visiting[shape.Name] {
			return "&" + shape.Name
		}
		visiting[shape.Name] = true
		defer delete(visiting, shape.Name)
	}
	members := make([]string, 0, len(shape.Fields))
	for _, field := range shape.Fields {
		optional := ""
		if field.Optional {
			optional = "?"
		}
		members = append(members, field.Name+optional+": "+describe(field.Node, visiting))
	}
	return shape.Name + "(" + strings.Join(members, ", ") + ")"
}

func describeUnion(shape structure.Union, visiting map[string]bool) string {
	alternatives := make([]string, 0, len(shape.Variants))
	for _, variant := range shape.Variants {
		alternatives = append(alternatives, variant.Name+" "+describe(variant.Node, visiting))
	}
	return shape.Name + "<" + strings.Join(alternatives, " | ") + ">"
}

// A Reference is where a projection decides between expanding a shape and
// pointing at it. Resolve is how a walker reaches the referenced shape, and it
// is what makes a recursive description terminate rather than recur.
func describeReference(shape structure.Reference, visiting map[string]bool) string {
	if shape.Name != "" {
		return "&" + shape.Name
	}
	if shape.Resolve == nil {
		return "&?"
	}
	return describe(shape.Resolve(), visiting)
}

func TestAProjectionCanBeWrittenOutsideThisModule(t *testing.T) {
	rendered := describe(everyShapeSchema.Structure(), map[string]bool{})
	want := "EveryShape(" +
		"text: text<free-text>, whole: integer, wide: integer, fraction: number, " +
		"flag: boolean, opaque: bytes<byte>, moment: timestamp<date-time>, " +
		"labels: {text: text}, " +
		"missing: text|null, nested: [integer], detail: Detail(note: text))"
	if rendered != want {
		t.Fatalf("expected\n  %s\ngot\n  %s", want, rendered)
	}
}

func TestAUnionsVariantsAreReachableFromTheDescription(t *testing.T) {
	rendered := describe(shapeSchema.Structure(), map[string]bool{})
	want := "Shape<circle Circle(radius: number) | rectangle Rectangle(width: number, height: number)>"
	if rendered != want {
		t.Fatalf("expected\n  %s\ngot\n  %s", want, rendered)
	}
}

type tree struct {
	Label    string
	Children []tree
}

var treeSchema schema.Schema[tree]

func init() {
	treeSchema = schema.Struct[tree]("Tree",
		schema.FieldOf("label", schema.Text(),
			func(node tree) string { return node.Label },
			func(node *tree, label string) { node.Label = label }),
		schema.FieldOf("children",
			schema.List(schema.Deferred(func() schema.Schema[tree] { return treeSchema })),
			func(node tree) []tree { return node.Children },
			func(node *tree, children []tree) { node.Children = children }),
	)
}

func TestARecursiveDescriptionTerminatesForAnyWalker(t *testing.T) {
	// The second time a name is reached the walker gets a pointer rather than
	// the shape again, which is what a projection needs to be total.
	rendered := describe(treeSchema.Structure(), map[string]bool{})
	want := "Tree(label: text, children: [&Tree])"
	if rendered != want {
		t.Fatalf("expected\n  %s\ngot\n  %s", want, rendered)
	}
}

func TestEveryScalarKindHasAName(t *testing.T) {
	// A projection switches on Kind, so an unnamed one would be a hole in every
	// projection at once.
	for kind := structure.Text; kind <= structure.Timestamp; kind++ {
		if name := kind.String(); name == "" || strings.HasPrefix(name, "Kind(") {
			t.Errorf("kind %d has no name, got %q", kind, name)
		}
	}
}

func TestAConstraintIsReadableByAWalkerOutsideThisModule(t *testing.T) {
	// A projection has to be able to see a bound, or it describes a wider type
	// than the codec accepts. The vocabulary is sealed, so a walker switches
	// over it and knows it has covered everything.
	code := schema.Matching(schema.MinLength(schema.Text(), 2), `^[a-z]{2,8}$`)
	shape, isScalar := code.Structure().(structure.Scalar)
	if !isScalar {
		t.Fatalf("expected a scalar, got %#v", code.Structure())
	}

	narrowed := []string{}
	for _, one := range shape.Constraints {
		var constraint structure.Constraint = one
		narrowed = append(narrowed, describeConstraint(constraint))
	}
	if got := strings.Join(narrowed, ", "); got != "at least 2 characters, matching ^[a-z]{2,8}$" {
		t.Fatalf("unexpected description: %s", got)
	}
}

// describeConstraint is what a projection outside this module writes: a switch
// over the sealed vocabulary, with no privileged access to any of it.
func describeConstraint(constraint structure.Constraint) string {
	switch narrowed := constraint.(type) {
	case structure.AtLeast:
		return "at least " + strconv.FormatFloat(narrowed.Value, 'g', -1, 64)
	case structure.AtMost:
		return "at most " + strconv.FormatFloat(narrowed.Value, 'g', -1, 64)
	case structure.Above:
		return "above " + strconv.FormatFloat(narrowed.Value, 'g', -1, 64)
	case structure.Below:
		return "below " + strconv.FormatFloat(narrowed.Value, 'g', -1, 64)
	case structure.MinLength:
		return "at least " + strconv.Itoa(narrowed.Value) + " characters"
	case structure.MaxLength:
		return "at most " + strconv.Itoa(narrowed.Value) + " characters"
	case structure.Pattern:
		return "matching " + narrowed.Expression
	case structure.MinItems:
		return "at least " + strconv.Itoa(narrowed.Value) + " items"
	case structure.MaxItems:
		return "at most " + strconv.Itoa(narrowed.Value) + " items"
	default:
		return "?"
	}
}
