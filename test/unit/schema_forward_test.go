package unit

// Naming a shape before it exists.
//
// This is the forward reference: Deferred contributes a structure.Reference,
// which is a name and a way to reach what it names later. It is what lets a
// description mention a shape that is not built yet -- itself, or another
// description whose turn has not come.

import (
	"strings"
	"testing"

	"github.com/mbauer83/effect-golang-web/schema"
	"github.com/mbauer83/effect-golang-web/schema/dynamic"
	"github.com/mbauer83/effect-golang-web/schema/structure"
)

// folder and file refer to each other. Neither exists when the other is
// written, and Go permits it because the reference is a closure rather than a
// value: nothing is resolved until something reads it.
var folder schema.Schema[dynamic.Value]
var file schema.Schema[dynamic.Value]

func init() {
	folder = schema.Struct[dynamic.Value]("Folder",
		schema.DescribedField("name", schema.Text()),
		schema.DescribedField("files",
			schema.List(schema.Deferred(func() schema.Schema[dynamic.Value] { return file }))),
	)
	file = schema.Struct[dynamic.Value]("File",
		schema.DescribedField("name", schema.Text()),
		schema.DescribedField("parent",
			schema.Nullable(schema.Deferred(func() schema.Schema[dynamic.Value] { return folder }))),
	)
}

func TestTwoDescriptionsMayNameEachOther(t *testing.T) {
	document := `{"name":"root","files":[{"name":"notes.txt","parent":null}]}`

	value, err := schema.DecodeJSON(folder, []byte(document))
	if err != nil {
		t.Fatal(err)
	}
	files, _ := value.(dynamic.Object).Member("files")
	held := files.(dynamic.List).Elements[0].(dynamic.Object)
	if name, _ := held.Member("name"); name != (dynamic.Text{Value: "notes.txt"}) {
		t.Fatalf("unexpected member: %#v", name)
	}

	written, err := schema.EncodeJSON(folder, value)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(written)); got != document {
		t.Fatalf("expected\n  %s\ngot\n  %s", document, got)
	}
}

func TestAForwardReferenceIsANameInTheDescription(t *testing.T) {
	// What the reference contributes is a name and a way to reach the shape,
	// which is what makes a projection of a cycle terminate: the second time a
	// name is reached, a pointer is written rather than the shape again.
	object := folder.Structure().(structure.Object)
	element := object.Fields[1].Node.(structure.Sequence).Element

	reference, isReference := element.(structure.Reference)
	if !isReference {
		t.Fatalf("expected a reference, got %#v", element)
	}
	if reference.Resolve == nil {
		t.Fatal("expected a way to reach what it names")
	}
	if named := reference.Resolve().(structure.Object); named.Name != "File" {
		t.Fatalf("expected it to name File, got %q", named.Name)
	}
}
