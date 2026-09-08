package unit

// The proto3 projection, checked against a real protobuf compiler.
//
// A golden string would only say that the projection still emits what it
// emitted last week. What matters is that protobuf accepts it, and that the
// descriptor it produces says what the description said -- so the file is
// compiled here, and the assertions are made against the compiled descriptor
// rather than against the text.

import (
	"context"
	"strings"
	"testing"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/linker"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/mbauer83/effect-golang-web/schema"
	"github.com/mbauer83/effect-golang-web/schema/protobuf"
)

// consignment is the shape the protobuf tests project and encode.
type consignment struct {
	Reference string
	Weight    float64
	Parcels   int32
	Fragile   bool
	Labels    []string
	Note      *string
}

var consignmentSchema = schema.Struct[consignment]("Consignment",
	schema.FieldOf("reference", schema.UUID(),
		func(held consignment) string { return held.Reference },
		func(held *consignment, value string) { held.Reference = value }).
		Numbered(1).
		Documented("Reference identifies the consignment."),
	schema.FieldOf("weight", schema.Above(schema.Float64(), 0),
		func(held consignment) float64 { return held.Weight },
		func(held *consignment, value float64) { held.Weight = value }).
		Numbered(2),
	schema.FieldOf("parcels", schema.AtLeast(schema.Int32(), 1),
		func(held consignment) int32 { return held.Parcels },
		func(held *consignment, value int32) { held.Parcels = value }).
		Numbered(3),
	schema.FieldOf("fragile", schema.Bool(),
		func(held consignment) bool { return held.Fragile },
		func(held *consignment, value bool) { held.Fragile = value }).
		Numbered(4),
	schema.FieldOf("labels", schema.List(schema.Text()),
		func(held consignment) []string { return held.Labels },
		func(held *consignment, value []string) { held.Labels = value }).
		Numbered(5),
	schema.OptionalFieldOf("note", schema.Text(),
		func(held consignment) (string, bool) {
			if held.Note == nil {
				return "", false
			}
			return *held.Note, true
		},
		func(held *consignment, value string) { held.Note = &value }).
		Numbered(6),
).Documented("Consignment is one shipment.")

// compiled compiles a projected document and returns its root message.
func protoCompiled(t *testing.T, document protobuf.Document) protoreflect.MessageDescriptor {
	t.Helper()
	rendered := document.Render()
	compiler := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
			Accessor: protocompile.SourceAccessorFromMap(
				map[string]string{"projected.proto": rendered}),
		}),
	}
	files, err := compiler.Compile(context.Background(), "projected.proto")
	if err != nil {
		t.Fatalf("the projected file did not compile: %v\n%s", err, rendered)
	}
	return rootOf(t, files, document.Root)
}

func rootOf(
	t *testing.T,
	files linker.Files,
	name string,
) protoreflect.MessageDescriptor {
	t.Helper()
	messages := files[0].Messages()
	for index := range messages.Len() {
		if string(messages.Get(index).Name()) == name {
			return messages.Get(index)
		}
	}
	t.Fatalf("the root message %q is not in the file", name)
	return nil
}

func TestTheProjectedFileCompilesAndSaysWhatTheDescriptionSaid(t *testing.T) {
	document, err := protobuf.Project(consignmentSchema.Structure(), "logistics.v1")
	if err != nil {
		t.Fatal(err)
	}
	message := protoCompiled(t, document)

	// Numbers, because a number is what protobuf's compatibility rests on and
	// the description is where it lives.
	for name, expected := range map[string]protoreflect.FieldNumber{
		"reference": 1, "weight": 2, "parcels": 3, "fragile": 4, "labels": 5, "note": 6,
	} {
		field := message.Fields().ByName(protoreflect.Name(name))
		if field == nil {
			t.Errorf("%s is not in the descriptor", name)
			continue
		}
		if field.Number() != expected {
			t.Errorf("%s is %d, expected %d", name, field.Number(), expected)
		}
	}

	// Widths, because int32 and int64 are different types on the wire: a
	// projection that widened would change what every other language generates.
	for name, expected := range map[string]protoreflect.Kind{
		"reference": protoreflect.StringKind,
		"weight":    protoreflect.DoubleKind,
		"parcels":   protoreflect.Int32Kind,
		"fragile":   protoreflect.BoolKind,
		"labels":    protoreflect.StringKind,
		"note":      protoreflect.StringKind,
	} {
		if field := message.Fields().ByName(protoreflect.Name(name)); field.Kind() != expected {
			t.Errorf("%s is %v, expected %v", name, field.Kind(), expected)
		}
	}

	// A list is repeated and has no explicit presence; an optional field has
	// presence and is not repeated. Both at once would be the description
	// saying the same thing twice.
	labels := message.Fields().ByName("labels")
	if !labels.IsList() || labels.HasPresence() {
		t.Errorf("expected labels repeated without presence, got list=%v presence=%v",
			labels.IsList(), labels.HasPresence())
	}
	note := message.Fields().ByName("note")
	if note.IsList() || !note.HasPresence() {
		t.Errorf("expected note optional and not repeated, got list=%v presence=%v",
			note.IsList(), note.HasPresence())
	}
}

func TestTheProjectionCarriesWhatProto3CannotState(t *testing.T) {
	// Protobuf has no validation keywords, so the constraints are comments.
	// A comment is honest about not being enforced by the wire, and the server
	// does enforce them, through the same schema.
	document, err := protobuf.Project(consignmentSchema.Structure(), "logistics.v1")
	if err != nil {
		t.Fatal(err)
	}
	rendered := document.Render()

	for _, said := range []string{
		"// above 0",
		"// at least 1",
		"// format: uuid",
		"// Reference identifies the consignment.",
		"// Consignment is one shipment.",
	} {
		if !strings.Contains(rendered, said) {
			t.Errorf("expected %q in the file:\n%s", said, rendered)
		}
	}
	if !strings.Contains(rendered, "package logistics.v1;") {
		t.Errorf("expected the package declared:\n%s", rendered)
	}
}
