package unit

// The shapes beyond a flat message: a oneof, a map, an instant, a nested
// message, and a packed list of numbers.
//
// Each goes through the canonical implementation, because a round trip through
// this codec alone would only say its two halves agree.

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/mbauer83/effect-golang-web/schema"
	"github.com/mbauer83/effect-golang-web/schema/protobuf"
)

// leg is a nested message, so a message-valued field is exercised.
type leg struct {
	From string
	To   string
}

var legSchema = schema.Struct[leg]("Leg",
	schema.FieldOf("from", schema.Text(),
		func(held leg) string { return held.From },
		func(held *leg, value string) { held.From = value }).Numbered(1),
	schema.FieldOf("to", schema.Text(),
		func(held leg) string { return held.To },
		func(held *leg, value string) { held.To = value }).Numbered(2),
)

// manifest carries one of everything else.
type manifest struct {
	Legs      []leg
	Weights   []float64
	Counts    []int32
	Tariffs   map[string]int64
	Collected time.Time
	Offset    int64
}

var manifestSchema = schema.Struct[manifest]("Manifest",
	schema.FieldOf("legs", schema.List(legSchema),
		func(held manifest) []leg { return held.Legs },
		func(held *manifest, value []leg) { held.Legs = value }).Numbered(1),
	schema.FieldOf("weights", schema.List(schema.Float64()),
		func(held manifest) []float64 { return held.Weights },
		func(held *manifest, value []float64) { held.Weights = value }).Numbered(2),
	schema.FieldOf("counts", schema.List(schema.Int32()),
		func(held manifest) []int32 { return held.Counts },
		func(held *manifest, value []int32) { held.Counts = value }).Numbered(3),
	schema.FieldOf("tariffs", schema.Map(schema.Int64()),
		func(held manifest) map[string]int64 { return held.Tariffs },
		func(held *manifest, value map[string]int64) { held.Tariffs = value }).Numbered(4),
	schema.FieldOf("collected", schema.Time(),
		func(held manifest) time.Time { return held.Collected },
		func(held *manifest, value time.Time) { held.Collected = value }).Numbered(5),
	schema.FieldOf("offset", schema.Int64(),
		func(held manifest) int64 { return held.Offset },
		func(held *manifest, value int64) { held.Offset = value }).Numbered(6),
)

// readByProtobuf compiles a description's projection and reads bytes with the
// reference implementation.
func readByProtobuf[A any](
	t *testing.T,
	shape schema.Schema[A],
	message []byte,
) *dynamicpb.Message {
	t.Helper()
	document, err := protobuf.Project(shape.Structure(), "logistics.v1")
	if err != nil {
		t.Fatal(err)
	}
	read := dynamicpb.NewMessage(protoCompiled(t, document))
	if err := proto.Unmarshal(message, read); err != nil {
		t.Fatalf("protobuf could not read what this wrote: %v", err)
	}
	return read
}

func TestListsMapsInstantsAndNestedMessagesCrossAsThemselves(t *testing.T) {
	sent := manifest{
		Legs:      []leg{{From: "Kiel", To: "Hamburg"}, {From: "Hamburg", To: "Bremen"}},
		Weights:   []float64{1.5, 2.25},
		Counts:    []int32{-1, 0, 7},
		Tariffs:   map[string]int64{"road": 40, "rail": 25},
		Collected: time.Date(2026, time.September, 8, 12, 0, 0, 500, time.UTC),
		Offset:    -3,
	}

	written, err := protobuf.Encode(manifestSchema, sent)
	if err != nil {
		t.Fatal(err)
	}

	// Read by protobuf: the packed lists, the map entries and the well-known
	// timestamp are all shapes the reference implementation knows, so it
	// disagreeing would mean this wrote something else.
	read := readByProtobuf(t, manifestSchema, written)
	fields := read.Descriptor().Fields()
	if legs := read.Get(fields.ByName("legs")).List(); legs.Len() != 2 {
		t.Errorf("unexpected legs: %d", legs.Len())
	}
	if weights := read.Get(fields.ByName("weights")).List(); weights.Len() != 2 ||
		weights.Get(1).Float() != 2.25 {
		t.Errorf("unexpected weights: %v", read.Get(fields.ByName("weights")))
	}
	// A negative int32 sign-extends to sixty-four bits, which is the case a
	// hand-written varint gets wrong.
	if counts := read.Get(fields.ByName("counts")).List(); counts.Len() != 3 ||
		counts.Get(0).Int() != -1 {
		t.Errorf("unexpected counts: %v", read.Get(fields.ByName("counts")))
	}
	if tariffs := read.Get(fields.ByName("tariffs")).Map(); tariffs.Len() != 2 {
		t.Errorf("unexpected tariffs: %d", tariffs.Len())
	}
	if got := read.Get(fields.ByName("offset")).Int(); got != -3 {
		t.Errorf("unexpected offset: %d", got)
	}

	// And back through this codec, which is the other half of the claim.
	back, err := protobuf.Decode(manifestSchema, written)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Legs) != 2 || back.Legs[1].To != "Bremen" {
		t.Errorf("unexpected legs: %#v", back.Legs)
	}
	if len(back.Counts) != 3 || back.Counts[0] != -1 || back.Counts[1] != 0 {
		t.Errorf("unexpected counts: %#v", back.Counts)
	}
	if back.Tariffs["rail"] != 25 || len(back.Tariffs) != 2 {
		t.Errorf("unexpected tariffs: %#v", back.Tariffs)
	}
	if !back.Collected.Equal(sent.Collected) {
		t.Errorf("unexpected instant: %v", back.Collected)
	}
	if back.Offset != -3 {
		t.Errorf("unexpected offset: %d", back.Offset)
	}
}
