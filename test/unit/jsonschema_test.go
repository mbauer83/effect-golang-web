package unit

// The JSON Schema projection. What matters is that a named type is described
// once and referred to thereafter, that a recursive type terminates, and that
// the output is stable enough to compare.

import (
	"strings"
	"testing"

	"github.com/mbauer83/effect-golang-web/schema"
	"github.com/mbauer83/effect-golang-web/schema/jsonschema"
)

func projectBook(t *testing.T) jsonschema.Document {
	t.Helper()
	return jsonschema.Project(bookSchema.Structure())
}

func TestANamedTypeBecomesAComponentAndIsReferredTo(t *testing.T) {
	document := projectBook(t)

	if document.Root.Ref != jsonschema.ReferenceTo("Book") {
		t.Fatalf("expected the root to reference the component, got %#v", document.Root)
	}
	component, described := document.Components["Book"]
	if !described {
		t.Fatalf("expected a Book component, got %v", document.ComponentNames())
	}
	if component.Type != "object" || len(component.Properties) != 5 {
		t.Fatalf("unexpected component: %#v", component)
	}
}

func TestOnlyRequiredFieldsAreListedAsRequired(t *testing.T) {
	component := projectBook(t).Components["Book"]

	required := strings.Join(component.Required, ",")
	if required != "title,authors,pages,hasIndex" {
		t.Fatalf("expected the optional field excluded, got %q", required)
	}
}

func TestScalarRefinementsReachTheProjection(t *testing.T) {
	type stamped struct {
		ID string
	}
	stampedSchema := schema.Struct[stamped]("Stamped",
		schema.FieldOf("id", schema.Formatted("uuid"),
			func(value stamped) string { return value.ID },
			func(value *stamped, id string) { value.ID = id }),
	)

	component := jsonschema.Project(stampedSchema.Structure()).Components["Stamped"]
	if component.Properties[0].Schema.Format != "uuid" {
		t.Fatalf("expected the refinement carried through, got %#v", component.Properties[0])
	}
}

func TestARecursiveTypeTerminates(t *testing.T) {
	// A tree refers to itself. Without component references the projection
	// would not terminate, so this is the case that justifies them.
	type Node struct {
		Label    string
		Children []Node
	}
	var nodeSchema schema.Schema[Node]
	nodeSchema = schema.Struct[Node]("Node",
		schema.FieldOf("label", schema.Text(),
			func(node Node) string { return node.Label },
			func(node *Node, label string) { node.Label = label }),
		schema.FieldOf("children", schema.List(schema.Deferred(func() schema.Schema[Node] {
			return nodeSchema
		})),
			func(node Node) []Node { return node.Children },
			func(node *Node, children []Node) { node.Children = children }),
	)

	document := jsonschema.Project(nodeSchema.Structure())
	component, described := document.Components["Node"]
	if !described {
		t.Fatalf("expected a Node component, got %v", document.ComponentNames())
	}
	children := component.Properties[1].Schema
	if children.Type != "array" || children.Items == nil {
		t.Fatalf("unexpected children schema: %#v", children)
	}
	if children.Items.Ref != jsonschema.ReferenceTo("Node") {
		t.Fatalf("expected the recursion to become a reference, got %#v", children.Items)
	}
}

func TestARecursiveValueRoundTrips(t *testing.T) {
	type Node struct {
		Label    string
		Children []Node
	}
	var nodeSchema schema.Schema[Node]
	nodeSchema = schema.Struct[Node]("Node",
		schema.FieldOf("label", schema.Text(),
			func(node Node) string { return node.Label },
			func(node *Node, label string) { node.Label = label }),
		schema.FieldOf("children", schema.List(schema.Deferred(func() schema.Schema[Node] {
			return nodeSchema
		})),
			func(node Node) []Node { return node.Children },
			func(node *Node, children []Node) { node.Children = children }),
	)

	original := Node{Label: "root", Children: []Node{
		{Label: "left", Children: []Node{}},
		{Label: "right", Children: []Node{{Label: "leaf", Children: []Node{}}}},
	}}
	document, err := schema.EncodeJSON(nodeSchema, original)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := schema.DecodeJSON(nodeSchema, document)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Children[1].Children[0].Label != "leaf" {
		t.Fatalf("round trip lost the tree: %#v", decoded)
	}
}

func TestRenderingIsStableAndValidJSON(t *testing.T) {
	first, err := projectBook(t).Render()
	if err != nil {
		t.Fatal(err)
	}
	second, err := projectBook(t).Render()
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("rendering is not stable:\n  %s\n  %s", first, second)
	}

	rendered := string(first)
	for _, expected := range []string{
		`"$ref":"#/$defs/Book"`,
		`"$defs"`,
		`"title":{"type":"string"}`,
		`"authors":{"type":"array","items":{"type":"string"}}`,
		`"required":["title","authors","pages","hasIndex"]`,
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected %s in\n  %s", expected, rendered)
		}
	}
}

func TestAMapProjectsAsAdditionalProperties(t *testing.T) {
	document := jsonschema.Project(schema.Map(schema.Int()).Structure())
	if document.Root.Type != "object" || document.Root.Values == nil {
		t.Fatalf("unexpected projection: %#v", document.Root)
	}
	if document.Root.Values.Type != "integer" {
		t.Fatalf("unexpected value schema: %#v", document.Root.Values)
	}
}

func TestSeveralStructuresShareOneComponentSet(t *testing.T) {
	// An OpenAPI document needs this: every endpoint's shapes are described
	// against one component set, so a type used twice appears once.
	projected, components := jsonschema.ProjectAll(
		bookSchema.Structure(),
		schema.List(bookSchema).Structure(),
	)

	if len(components) != 1 {
		t.Fatalf("expected one shared component, got %v", components)
	}
	if projected[0].Ref == "" || projected[1].Items == nil || projected[1].Items.Ref == "" {
		t.Fatalf("expected both projections to reference it, got %#v", projected)
	}
}

func TestAUnionProjectsAsTheWireFormItActuallyWrites(t *testing.T) {
	// The codec names the variant as an object's single member, so each
	// alternative must project as that object. A document describing the
	// unwrapped variant would describe something this module never writes.
	component := jsonschema.Project(shapeSchema.Structure()).Components["Shape"]

	if len(component.OneOf) != 2 {
		t.Fatalf("expected one alternative per variant, got %#v", component.OneOf)
	}
	for index, name := range []string{"circle", "rectangle"} {
		alternative := component.OneOf[index]
		if alternative.Type != "object" || len(alternative.Properties) != 1 {
			t.Fatalf("expected a single-member object, got %#v", alternative)
		}
		if alternative.Properties[0].Name != name ||
			strings.Join(alternative.Required, ",") != name {
			t.Fatalf("expected %q named and required, got %#v", name, alternative)
		}
	}
}

func TestAVariantsShapeIsItselfAComponent(t *testing.T) {
	document := jsonschema.Project(shapeSchema.Structure())

	circle := document.Components["Shape"].OneOf[0].Properties[0].Schema
	if circle.Ref != jsonschema.ReferenceTo("Circle") {
		t.Fatalf("expected the variant to reference its component, got %#v", circle)
	}
	if _, described := document.Components["Circle"]; !described {
		t.Fatalf("expected a Circle component, got %v", document.ComponentNames())
	}
}

func TestAVariantsDocumentationReachesItsAlternative(t *testing.T) {
	component := jsonschema.Project(shapeSchema.Structure()).Components["Shape"]

	if component.OneOf[0].Description != "a circle, by its radius" {
		t.Fatalf("expected the variant's prose carried through, got %#v", component.OneOf[0])
	}
}
