package unit

// The application-property boundary, in both directions.
//
// It looks like the AMQP 0-9-1 field-table boundary and is deliberately not
// shared with it: the two protocols permit different value sets, and this one
// has kinds -- unsigned integers, a UUID -- that 0-9-1 has no encoding for. The
// round trip is the claim worth testing, and so is what happens to the kinds
// only this protocol sends.

import (
	"reflect"
	"strings"
	"testing"
	"time"

	broker "github.com/Azure/go-amqp"

	"github.com/mbauer83/effect-golang-schema/schema/dynamic"
	"github.com/mbauer83/effect-golang-web/amqp10"
)

func TestEveryKindAPropertyMayHoldSurvivesTheRoundTrip(t *testing.T) {
	moment := time.Date(2026, time.September, 8, 12, 0, 0, 0, time.UTC)
	named := dynamic.Object{Fields: []dynamic.Field{
		{Name: "absent", Value: dynamic.Absent{}},
		{Name: "boolean", Value: dynamic.OfBoolean(true)},
		{Name: "bytes", Value: dynamic.OfBytes([]byte{1, 2})},
		{Name: "integer", Value: dynamic.OfInteger(7)},
		{Name: "list", Value: dynamic.List{Elements: []dynamic.Value{
			dynamic.OfInteger(1), dynamic.OfText("two"),
		}}},
		{Name: "nested", Value: dynamic.Object{Fields: []dynamic.Field{
			{Name: "deeper", Value: dynamic.OfText("held")},
		}}},
		{Name: "number", Value: dynamic.OfNumber(1.5)},
		{Name: "text", Value: dynamic.OfText("held")},
		{Name: "timestamp", Value: dynamic.OfTimestamp(moment)},
	}}

	properties, err := amqp10.Properties(named)
	if err != nil {
		t.Fatal(err)
	}
	read, err := amqp10.Named(properties)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(read, named) {
		t.Fatalf("expected the properties unchanged, got %#v", read)
	}
}

func TestNoPropertiesIsNoMapRatherThanAnEmptyOne(t *testing.T) {
	properties, err := amqp10.Properties(dynamic.Object{})
	if err != nil {
		t.Fatal(err)
	}
	if properties != nil {
		t.Fatalf("expected no map, got %#v", properties)
	}
	// And the same for annotations, which are the other direction of the same
	// boundary and a separate function because their keys are not strings.
	annotations, err := amqp10.Annotations(dynamic.Object{})
	if err != nil {
		t.Fatal(err)
	}
	if annotations != nil {
		t.Fatalf("expected no annotations, got %#v", annotations)
	}
}

func TestTheKindsOnlyThisProtocolSendsAreCarried(t *testing.T) {
	// 1.0's unsigned integers and its UUID, neither of which AMQP 0-9-1 has an
	// encoding for. A UUID becomes text, because the representation has no case
	// for one and its string form is what a schema's uuid format reads.
	identity := broker.UUID{
		0x8f, 0x14, 0xe4, 0x5f, 0xce, 0xea, 0x46, 0x7a,
		0xa4, 0xfb, 0x1a, 0x9c, 0x73, 0xd0, 0xf2, 0xb1,
	}
	read, err := amqp10.Named(map[string]any{
		"tiny":     uint8(1),
		"small":    uint16(2),
		"wide":     uint32(3),
		"identity": identity,
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"tiny", "small", "wide"} {
		held, present := read.Member(name)
		if !present {
			t.Errorf("%s is missing", name)
			continue
		}
		if _, whole := held.(dynamic.Integer); !whole {
			t.Errorf("%s came back as %T", name, held)
		}
	}
	held, present := read.Member("identity")
	if !present {
		t.Fatal("the identity is missing")
	}
	if held != dynamic.OfText("8f14e45f-ceea-467a-a4fb-1a9c73d0f2b1") {
		t.Fatalf("expected the uuid as its string form, got %#v", held)
	}
}

func TestAPropertyThisSideCannotCarryIsNamedRatherThanDropped(t *testing.T) {
	// A consumer that acted on the properties it could read would be acting on
	// half the message, so the one it cannot is the whole conversion's answer.
	if _, err := amqp10.Named(map[string]any{"odd": complex(1, 2)}); err == nil {
		t.Error("expected a value outside the representation to be refused")
	} else if !strings.Contains(err.Error(), "odd") {
		t.Errorf("expected the property named, got %v", err)
	}

	// With the position inside a list, because a list of twelve needs that.
	_, err := amqp10.Named(map[string]any{"inside": []any{"fine", complex(1, 2)}})
	if err == nil {
		t.Fatal("expected the nested value to be refused")
	}
	if !strings.Contains(err.Error(), "inside") || !strings.Contains(err.Error(), "element 1") {
		t.Errorf("expected the property and the position named, got %v", err)
	}
}
