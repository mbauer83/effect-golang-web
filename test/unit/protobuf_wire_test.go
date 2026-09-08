package unit

// The protobuf wire codec, checked against the canonical implementation.
//
// A round trip through itself would only say the two halves agree with each
// other. What matters is that what this writes is what protobuf reads, and that
// what protobuf writes is what this reads -- so every case here goes through
// google.golang.org/protobuf in one direction or the other, against a message
// built from the descriptor this module's own projection produced.

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/mbauer83/effect-golang-web/schema/protobuf"
)

func consigned() consignment {
	note := "handle with care"
	return consignment{
		Reference: "8f14e45f-ceea-467a-a4fb-1a9c73d0f2b1",
		Weight:    12.5,
		Parcels:   3,
		Fragile:   true,
		Labels:    []string{"fragile", "this way up"},
		Note:      &note,
	}
}

// canonical reads bytes with the reference implementation, against the
// descriptor this module projected.
func canonical(t *testing.T, message []byte) *dynamicpb.Message {
	t.Helper()
	document, err := protobuf.Project(consignmentSchema.Structure(), "logistics.v1")
	if err != nil {
		t.Fatal(err)
	}
	read := dynamicpb.NewMessage(protoCompiled(t, document))
	if err := proto.Unmarshal(message, read); err != nil {
		t.Fatalf("protobuf could not read what this wrote: %v", err)
	}
	return read
}

func TestWhatThisWritesIsWhatProtobufReads(t *testing.T) {
	written, err := protobuf.Encode(consignmentSchema, consigned())
	if err != nil {
		t.Fatal(err)
	}
	read := canonical(t, written)
	fields := read.Descriptor().Fields()

	if got := read.Get(fields.ByName("reference")).String(); got != consigned().Reference {
		t.Errorf("unexpected reference: %q", got)
	}
	if got := read.Get(fields.ByName("weight")).Float(); got != 12.5 {
		t.Errorf("unexpected weight: %v", got)
	}
	if got := read.Get(fields.ByName("parcels")).Int(); got != 3 {
		t.Errorf("unexpected parcels: %v", got)
	}
	if !read.Get(fields.ByName("fragile")).Bool() {
		t.Error("expected fragile")
	}
	labels := read.Get(fields.ByName("labels")).List()
	if labels.Len() != 2 || labels.Get(0).String() != "fragile" {
		t.Errorf("unexpected labels: %v", read.Get(fields.ByName("labels")))
	}
	// The optional field has explicit presence, so protobuf can say it is
	// there -- which is the difference between absent and the empty string.
	note := fields.ByName("note")
	if !read.Has(note) || read.Get(note).String() != "handle with care" {
		t.Errorf("unexpected note: present=%v %q", read.Has(note), read.Get(note).String())
	}
}

func TestWhatProtobufWritesIsWhatThisReads(t *testing.T) {
	document, err := protobuf.Project(consignmentSchema.Structure(), "logistics.v1")
	if err != nil {
		t.Fatal(err)
	}
	descriptor := protoCompiled(t, document)
	built := dynamicpb.NewMessage(descriptor)
	fields := descriptor.Fields()

	built.Set(fields.ByName("reference"),
		protoreflect.ValueOfString("c9f0f895-fb98-4b1e-9f1e-6a1c9d0e2b34"))
	built.Set(fields.ByName("weight"), protoreflect.ValueOfFloat64(0.5))
	built.Set(fields.ByName("parcels"), protoreflect.ValueOfInt32(1))
	built.Set(fields.ByName("fragile"), protoreflect.ValueOfBool(false))
	labels := built.Mutable(fields.ByName("labels")).List()
	labels.Append(protoreflect.ValueOfString("bulk"))

	message, err := proto.Marshal(built)
	if err != nil {
		t.Fatal(err)
	}

	read, err := protobuf.Decode(consignmentSchema, message)
	if err != nil {
		t.Fatal(err)
	}
	if read.Reference != "c9f0f895-fb98-4b1e-9f1e-6a1c9d0e2b34" || read.Weight != 0.5 {
		t.Errorf("unexpected value: %#v", read)
	}
	if read.Parcels != 1 || read.Fragile {
		t.Errorf("unexpected value: %#v", read)
	}
	if len(read.Labels) != 1 || read.Labels[0] != "bulk" {
		t.Errorf("unexpected labels: %#v", read.Labels)
	}
	// Not written, so not there: proto3 would say the empty string, and this
	// layer says absent so the description decides what that means.
	if read.Note != nil {
		t.Errorf("expected no note, got %q", *read.Note)
	}
}

func TestAValueTheDescriptionRefusesIsRefusedOnTheWayInAsOnTheWayOut(t *testing.T) {
	// The wire has no minimum, so a zero page count is a message protobuf is
	// perfectly happy with. The description is not, and it is the description
	// that decides -- which is the reason proto3's defaults are not substituted
	// here: doing that would admit exactly this.
	document, err := protobuf.Project(consignmentSchema.Structure(), "logistics.v1")
	if err != nil {
		t.Fatal(err)
	}
	descriptor := protoCompiled(t, document)
	built := dynamicpb.NewMessage(descriptor)
	fields := descriptor.Fields()
	built.Set(fields.ByName("reference"),
		protoreflect.ValueOfString("c9f0f895-fb98-4b1e-9f1e-6a1c9d0e2b34"))
	built.Set(fields.ByName("weight"), protoreflect.ValueOfFloat64(1))
	built.Set(fields.ByName("parcels"), protoreflect.ValueOfInt32(0))

	message, err := proto.Marshal(built)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := protobuf.Decode(consignmentSchema, message); err == nil {
		t.Fatal("expected the description to refuse a zero parcel count")
	}

	// And a bad reference on the way out, before a byte is written.
	broken := consigned()
	broken.Reference = "not a uuid"
	if _, err := protobuf.Encode(consignmentSchema, broken); err == nil {
		t.Fatal("expected the description to refuse the reference")
	}
}
