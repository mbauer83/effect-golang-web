package unit

// The field-table boundary, in both directions.
//
// A message's headers are an AMQP field table, which the protocol defines as a
// set of named values and the library represents as map[string]any. That is the
// second place this module holds a value it cannot name, and unlike the
// database one it maps case for case: a field table permits every kind the
// representation has, nesting included, so nothing is narrowed on the way
// through and the round trip is the claim worth testing.

import (
	"reflect"
	"strings"
	"testing"
	"time"

	broker "github.com/rabbitmq/amqp091-go"

	"github.com/mbauer83/effect-golang-schema/schema/dynamic"
	"github.com/mbauer83/effect-golang-web/amqp091"
)

func TestEveryKindAHeaderMayHoldSurvivesTheRoundTrip(t *testing.T) {
	moment := time.Date(2026, time.September, 8, 12, 0, 0, 0, time.UTC)
	// Nesting included, because a field table holds tables and lists, and a
	// boundary that flattened them would lose what a producer said.
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

	table, err := amqp091.Table(named)
	if err != nil {
		t.Fatal(err)
	}
	// The library's own validator, because the round trip below is this
	// module checking itself: Headers reads a nested table whichever Go type
	// carries it, so a table the protocol would refuse round-trips perfectly
	// and only a broker says otherwise. This is what a broker would say.
	if err := broker.Table(table).Validate(); err != nil {
		t.Fatalf("the table is not one the protocol accepts: %v", err)
	}
	read, err := amqp091.Headers(table)
	if err != nil {
		t.Fatal(err)
	}

	// Declared in name order above, which is the order they come back in: a
	// table is a map and has none, so by name is the only one available and a
	// deterministic one is what lets a consumer forward what it received.
	if !reflect.DeepEqual(read, named) {
		t.Fatalf("expected the headers unchanged, got %#v", read)
	}
}

func TestNoHeadersIsNoTableRatherThanAnEmptyOne(t *testing.T) {
	// A message with no headers should not carry a table saying so. The
	// protocol has no distinction to make there, and an empty one is a field a
	// broker would log.
	table, err := amqp091.Table(dynamic.Object{})
	if err != nil {
		t.Fatal(err)
	}
	if table != nil {
		t.Fatalf("expected no table, got %#v", table)
	}
}

func TestTheNarrowerNumericKindsABrokerSendsAreWidened(t *testing.T) {
	// A broker sends the narrowest type that fits, so a header a producer wrote
	// as an integer comes back as an int8 or an int32. The representation has
	// one whole-number case, and widening is the conversion that loses nothing.
	read, err := amqp091.Headers(map[string]any{
		"small":  int8(1),
		"medium": int16(2),
		"wide":   int32(3),
		"plain":  4,
		"single": float32(1.5),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"small", "medium", "wide", "plain"} {
		held, present := read.Member(name)
		if !present {
			t.Errorf("%s is missing", name)
			continue
		}
		if _, whole := held.(dynamic.Integer); !whole {
			t.Errorf("%s came back as %T", name, held)
		}
	}
	if held, _ := read.Member("single"); held != dynamic.OfNumber(1.5) {
		t.Errorf("expected the float widened exactly, got %#v", held)
	}
}

func TestAHeaderThisSideCannotCarryIsNamedRatherThanDropped(t *testing.T) {
	// A consumer that acted on the headers it could read would be acting on
	// half the message, so the one it cannot is the whole conversion's answer.
	if _, err := amqp091.Headers(map[string]any{"odd": complex(1, 2)}); err == nil {
		t.Error("expected a value outside the protocol to be refused")
	} else if !strings.Contains(err.Error(), "odd") {
		t.Errorf("expected the header named, got %v", err)
	}

	// With the position inside a list, because a list of twelve needs that.
	_, err := amqp091.Headers(map[string]any{
		"inside": []any{"fine", complex(1, 2)},
	})
	if err == nil {
		t.Fatal("expected the nested value to be refused")
	}
	if !strings.Contains(err.Error(), "inside") || !strings.Contains(err.Error(), "element 1") {
		t.Errorf("expected the header and the position named, got %v", err)
	}

	// The other direction has no such case to test: dynamic.Value is sealed by
	// an unexported method, so no value outside that package can reach the
	// conversion at all. Its default branch guards against a case being added
	// to the sum, not against a caller.
}
