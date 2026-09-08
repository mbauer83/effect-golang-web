package unit

// A oneof, and proto3's presence rules.
//
// The presence rules are the subtle part of the format and the part a codec
// gets wrong: an ordinary field writes no bytes for its zero, so absent and
// zero are the same message, and only `optional` and a message field can say
// they are not there.

import (
	"testing"

	"github.com/mbauer83/effect-golang-web/schema"
	"github.com/mbauer83/effect-golang-web/schema/dynamic"
	"github.com/mbauer83/effect-golang-web/schema/protobuf"
)

// shipped is a union with no Go type, so a oneof is exercised without a second
// Go hierarchy to declare.
var shipped = schema.OneOf[dynamic.Value]("Shipped",
	schema.DescribedVariant("byRoad", schema.Struct[dynamic.Value]("ByRoad",
		schema.DescribedField("plate", schema.Text()).Numbered(1))).Numbered(1),
	schema.DescribedVariant("byRail", schema.Struct[dynamic.Value]("ByRail",
		schema.DescribedField("wagon", schema.Text()).Numbered(1))).Numbered(2),
)

func TestAUnionCrossesAsAOneofAndProtobufAgreesWhichWasChosen(t *testing.T) {
	// Protobuf identifies the chosen member by its field number, so the choice
	// is in the wire itself and needs no discriminating field -- which is why
	// the projection refuses a description carrying one.
	var chosen dynamic.Value = dynamic.Object{Fields: []dynamic.Field{
		{Name: "byRail", Value: dynamic.Object{Fields: []dynamic.Field{
			{Name: "wagon", Value: dynamic.OfText("4711")},
		}}},
	}}

	written, err := protobuf.Encode(shipped, chosen)
	if err != nil {
		t.Fatal(err)
	}

	read := readByProtobuf(t, shipped, written)
	which := read.WhichOneof(read.Descriptor().Oneofs().ByName("value"))
	if which == nil || string(which.Name()) != "byRail" {
		t.Fatalf("expected protobuf to see byRail chosen, got %v", which)
	}

	back, err := protobuf.Decode(shipped, written)
	if err != nil {
		t.Fatal(err)
	}
	only, single := back.(dynamic.Object).Only()
	if !single || only.Name != "byRail" {
		t.Fatalf("unexpected value: %#v", back)
	}
}

func TestAFieldHoldingItsZeroCostsNoBytesAndComesBackAsTheZero(t *testing.T) {
	// The heart of proto3's presence rules. An ordinary field writes nothing
	// for its zero, so absent and zero are the same message -- and a reader
	// that treated absent as missing would find every false bool missing.
	plain := consigned()
	plain.Fragile = false
	plain.Note = nil

	written, err := protobuf.Encode(consignmentSchema, plain)
	if err != nil {
		t.Fatal(err)
	}
	read := readByProtobuf(t, consignmentSchema, written)
	fields := read.Descriptor().Fields()
	if read.Has(fields.ByName("fragile")) {
		t.Error("expected the false bool to cost no bytes")
	}
	// The optional field is the one that can say it is not there.
	if read.Has(fields.ByName("note")) {
		t.Error("expected the absent optional field to be absent")
	}

	back, err := protobuf.Decode(consignmentSchema, written)
	if err != nil {
		t.Fatal(err)
	}
	if back.Fragile {
		t.Error("expected the zero back as the zero")
	}
	if back.Note != nil {
		t.Errorf("expected no note, got %q", *back.Note)
	}
}

func TestAFieldTheDescriptionDoesNotKnowIsSkipped(t *testing.T) {
	// The property protobuf is chosen for: a message written by a newer program
	// stays readable by an older one. Field 99 is not in the description, so it
	// is passed over rather than being an error.
	written, err := protobuf.Encode(consignmentSchema, consigned())
	if err != nil {
		t.Fatal(err)
	}
	// Field 99, wire type 2, four bytes: what a newer program would have added.
	// It goes *first*, which is where it has to go for this to be a test of
	// anything: at the end, a skip of the wrong length would be invisible,
	// because there would be nothing after it to corrupt. Field order on the
	// wire is not significant, so a writer may put it there.
	unknown := []byte{0x9a, 0x06, 0x04, 'm', 'o', 'r', 'e'}
	newer := append(unknown, written...)

	back, err := protobuf.Decode(consignmentSchema, newer)
	if err != nil {
		t.Fatalf("expected the unknown field to be skipped, got %v", err)
	}
	// Every field after it, because a skip of the wrong length desynchronises
	// the reader and the damage shows up here rather than at the skip.
	if back.Reference != consigned().Reference {
		t.Errorf("unexpected value: %#v", back)
	}
	if back.Parcels != 3 || !back.Fragile || back.Weight != 12.5 {
		t.Errorf("the reader lost its place: %#v", back)
	}
	if len(back.Labels) != 2 || back.Note == nil || *back.Note != "handle with care" {
		t.Errorf("the reader lost its place: %#v", back)
	}
}
