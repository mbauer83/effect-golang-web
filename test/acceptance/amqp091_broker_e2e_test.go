package acceptance

// The dispatch program against a real broker.
//
// Gated on an address, because a broker is not something a test suite may
// assume: set EFFECT_GOLANG_AMQP_URL to run these. Everything the program does
// is checked against the in-process broker either way -- what only a real one
// can answer is whether the adapter's own decisions are right: the flags it
// passes, the field table it writes, and whether cancelling a consumer really
// stops the broker sending.

import (
	"context"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/mbauer83/effect-golang-schema/schema/dynamic"
	"github.com/mbauer83/effect-golang-web/amqp091"
	"github.com/mbauer83/effect-golang-web/examples/dispatch"
	"github.com/mbauer83/effect-golang/effect"
)

// brokered opens a connection and a channel, states a topology of its own, and
// runs the work.
//
// The topology is named for the test run, so two runs against the same broker
// do not share queues -- and it is Transient, so the broker forgets it.
func brokered[A any](
	t *testing.T,
	work func(*amqp091.Channel, amqp091.Topology) dispatching[A],
) effect.Exit[amqp091.Fault, A] {
	t.Helper()
	address := os.Getenv("EFFECT_GOLANG_AMQP_URL")
	if address == "" {
		t.Skip("set EFFECT_GOLANG_AMQP_URL to run this against a broker")
	}

	runtime, err := effect.NewRuntime(effect.WithDebugTracking())
	if err != nil {
		t.Fatal(err)
	}
	topology := temporary(t.Name())

	program := effect.Scoped(func(scope effect.Scope) dispatching[A] {
		return amqp091.Connect[effect.Unit](scope, address).
			FlatMap(func(connection *amqp091.Connection) dispatching[A] {
				return amqp091.Open[effect.Unit](scope, connection).
					FlatMap(func(channel *amqp091.Channel) dispatching[A] {
						// One at a time, so cancelling a consumer is the
						// difference between the rest arriving and the rest
						// waiting.
						return amqp091.Prefetch[effect.Unit](channel, 1).
							AndThen(amqp091.Declare[effect.Unit](channel, topology)).
							FlatMap(func(effect.Unit) dispatching[A] {
								return work(channel, topology)
							})
					})
			})
	})

	within, giveUp := context.WithTimeout(context.Background(), 30*time.Second)
	defer giveUp()

	exit := runtime.Run(within, effect.Unit{}, program)
	if live := runtime.LiveWork(); !live.IsEmpty() {
		t.Fatalf("the program left work behind: %#v", live)
	}
	return exit
}

func temporary(name string) amqp091.Topology {
	queue := "effect-golang-" + name
	exchange := queue + "-exchange"
	return amqp091.Topology{
		Exchanges: []amqp091.Exchange{
			{Name: exchange, Routing: amqp091.Direct, Durability: amqp091.Transient},
		},
		Queues: []amqp091.Queue{{Name: queue, Durability: amqp091.Transient}},
		Bindings: []amqp091.Binding{
			{Exchange: exchange, Queue: queue, Key: dispatch.Placed},
		},
	}
}

func TestAMessageCrossesARealBrokerWithItsHeadersAndComesBackAsItself(t *testing.T) {
	// What only a real broker can answer: the field table the adapter wrote is
	// one the protocol accepts, and it reads back case for case.
	sent := amqp091.Message{
		Body:        []byte(`{"reference":"8f14e45f-ceea-467a-a4fb-1a9c73d0f2b1","item":"lamp","quantity":2}`),
		ContentType: "application/json",
		Headers:     everyHeaderKind(),
		Durability:  amqp091.Lasting,
	}

	exit := brokered(t, func(
		channel *amqp091.Channel,
		topology amqp091.Topology,
	) dispatching[[]amqp091.Delivery] {
		return amqp091.Publish[effect.Unit](channel, published(topology), sent).
			FlatMap(func(effect.Unit) dispatching[[]amqp091.Delivery] {
				return effect.RunCollect(
					amqp091.Consume[effect.Unit](channel, topology.Queues[0].Name).TakeStream(1))
			})
	})

	arrived, ok := exit.Value()
	if !ok {
		t.Fatalf("unexpected exit: %+v", exit)
	}
	if len(arrived) != 1 {
		t.Fatalf("expected one delivery, got %d", len(arrived))
	}
	// The headers survived the broker, which is the boundary's real test: the
	// unit suite checks the conversion, and this checks that what it produces
	// is what the protocol carries.
	for _, header := range everyHeaderKind().Fields {
		held, present := arrived[0].Headers.Member(header.Name)
		if !present {
			t.Errorf("%s did not survive the broker", header.Name)
			continue
		}
		if !sameValue(held, header.Value) {
			t.Errorf("%s came back as %#v, sent %#v", header.Name, held, header.Value)
		}
	}
}

func TestStoppingEarlyStopsTheBrokerSendingAndLeavesTheRestWaiting(t *testing.T) {
	// The reason a subscription's release cancels the consumer. A stream that
	// read one of three and stopped leaves a broker that has no idea and goes
	// on sending, into a channel nobody reads -- with a prefetch of one those
	// deliveries are unacknowledged and the queue stalls behind them. So the
	// second consumer is the assertion: it sees what the first left.
	exit := brokered(t, func(
		channel *amqp091.Channel,
		topology amqp091.Topology,
	) dispatching[[]amqp091.Delivery] {
		queue := topology.Queues[0].Name
		return effect.ForEach([]int{1, 2, 3}, func(int) dispatching[effect.Unit] {
			return amqp091.Publish[effect.Unit](channel, published(topology),
				amqp091.Message{Body: []byte("{}")})
		}).
			FlatMap(func([]effect.Unit) dispatching[[]amqp091.Delivery] {
				// One, then the scope that held the subscription closes.
				return effect.RunCollect(
					amqp091.Consume[effect.Unit](channel, queue).TakeStream(1))
			}).
			FlatMap(func(first []amqp091.Delivery) dispatching[[]amqp091.Delivery] {
				if len(first) != 1 {
					t.Errorf("expected one delivery from the first consumer, got %d", len(first))
				}
				// A fresh subscription: the two the first consumer never
				// acknowledged are back, so the broker knows it is gone.
				return effect.RunCollect(
					amqp091.Consume[effect.Unit](channel, queue).TakeStream(2))
			})
	})

	rest, ok := exit.Value()
	if !ok {
		t.Fatalf("unexpected exit: %+v", exit)
	}
	if len(rest) != 2 {
		t.Fatalf("expected the other two waiting, got %d", len(rest))
	}
}

// everyHeaderKind is one header of each kind the protocol carries.
//
// Not the whole representation: a broker sends the narrowest type that fits, so
// a nested table and a list are here but a float32 would come back as one and
// the comparison would be about the broker rather than about the boundary.
func everyHeaderKind() dynamic.Object {
	return dynamic.Object{Fields: []dynamic.Field{
		{Name: "boolean", Value: dynamic.OfBoolean(true)},
		{Name: "integer", Value: dynamic.OfInteger(7)},
		{Name: "number", Value: dynamic.OfNumber(1.5)},
		{Name: "text", Value: dynamic.OfText("held")},
		{Name: "nested", Value: dynamic.Object{Fields: []dynamic.Field{
			{Name: "deeper", Value: dynamic.OfText("held")},
		}}},
		{Name: "list", Value: dynamic.List{Elements: []dynamic.Value{
			dynamic.OfInteger(1), dynamic.OfText("two"),
		}}},
	}}
}

// sameValue compares two values of the representation.
//
// reflect.DeepEqual would do for the scalars; a nested object needs this
// because the broker returns its members in its own order, and the boundary
// sorts by name -- so two objects that agree may not be identical slices.
func sameValue(held dynamic.Value, sent dynamic.Value) bool {
	switch expected := sent.(type) {
	case dynamic.Object:
		object, isObject := held.(dynamic.Object)
		if !isObject || len(object.Fields) != len(expected.Fields) {
			return false
		}
		for _, member := range expected.Fields {
			carried, present := object.Member(member.Name)
			if !present || !sameValue(carried, member.Value) {
				return false
			}
		}
		return true
	case dynamic.List:
		list, isList := held.(dynamic.List)
		if !isList || len(list.Elements) != len(expected.Elements) {
			return false
		}
		for index, element := range expected.Elements {
			if !sameValue(list.Elements[index], element) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(held, sent)
	}
}

func published(topology amqp091.Topology) amqp091.Target {
	return amqp091.Target{Exchange: topology.Exchanges[0].Name, Key: dispatch.Placed}
}
