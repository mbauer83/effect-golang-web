package acceptance

// The consign program against a real AMQP 1.0 broker.
//
// Gated on an address and a node, because a 1.0 broker's addresses are its own
// configuration and there is nothing in the protocol to declare one with: set
// EFFECT_GOLANG_AMQP10_URL and EFFECT_GOLANG_AMQP10_NODE to run these.
// Everything the program does is checked against the in-process broker either
// way. What only a real broker can answer is whether the adapter's own
// decisions are right -- the message sections it writes, the property map, the
// four dispositions, and whether a detached link ends the stream instead of
// failing it.

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/mbauer83/effect-golang-schema/schema/dynamic"
	"github.com/mbauer83/effect-golang-web/amqp10"
	"github.com/mbauer83/effect-golang-web/examples/consign"
	"github.com/mbauer83/effect-golang/effect"
)

// linked connects, opens a session, attaches a link each way, and runs the
// work.
func linked[A any](
	t *testing.T,
	work func(amqp10.Sending, amqp10.Receiving) consigning[A],
) effect.Exit[amqp10.Fault, A] {
	t.Helper()
	address := os.Getenv("EFFECT_GOLANG_AMQP10_URL")
	node := os.Getenv("EFFECT_GOLANG_AMQP10_NODE")
	if address == "" || node == "" {
		t.Skip("set EFFECT_GOLANG_AMQP10_URL and EFFECT_GOLANG_AMQP10_NODE to run this")
	}

	runtime, err := effect.NewRuntime(effect.WithDebugTracking())
	if err != nil {
		t.Fatal(err)
	}

	program := effect.Scoped(func(scope effect.Scope) consigning[A] {
		return amqp10.Connect[effect.Unit](scope, address, nil).
			FlatMap(func(connection *amqp10.Connection) consigning[A] {
				return amqp10.Open[effect.Unit](scope, connection).
					FlatMap(func(session *amqp10.Session) consigning[A] {
						return attachedBoth(scope, session, node, work)
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

// attachedBoth attaches a sender and a receiver to the same node.
//
// One credit, so the broker holds one unsettled message at a time: with none
// the link receives nothing at all, which is the part of 1.0 that has no
// equivalent in a 0-9-1 prefetch.
func attachedBoth[A any](
	scope effect.Scope,
	session *amqp10.Session,
	node string,
	work func(amqp10.Sending, amqp10.Receiving) consigning[A],
) consigning[A] {
	return amqp10.Sender[effect.Unit](scope, session, node).
		FlatMap(func(sender amqp10.Sending) consigning[A] {
			return amqp10.Receiver[effect.Unit](scope, session, node, 1).
				FlatMap(func(receiver amqp10.Receiving) consigning[A] {
					return work(sender, receiver)
				})
		})
}

func TestAMessageCrossesARealNodeWithItsPropertiesAndComesBackAsItself(t *testing.T) {
	// What only a real broker can answer: the sections and the property map the
	// adapter wrote are ones the protocol accepts, and they read back case for
	// case.
	sent := amqp10.Message{
		Body:        []byte(`{"reference":"8f14e45f-ceea-467a-a4fb-1a9c73d0f2b1","carrier":"overland","weight":12.5}`),
		ContentType: "application/json",
		Subject:     "8f14e45f-ceea-467a-a4fb-1a9c73d0f2b1",
		Properties:  everyPropertyKind(),
		Durability:  amqp10.Lasting,
	}

	exit := linked(t, func(
		sender amqp10.Sending,
		receiver amqp10.Receiving,
	) consigning[[]amqp10.Delivery] {
		return amqp10.Send[effect.Unit](sender, sent).
			FlatMap(func(effect.Unit) consigning[[]amqp10.Delivery] {
				return effect.RunCollect(amqp10.Receive[effect.Unit](receiver).TakeStream(1))
			}).
			FlatMap(func(arrived []amqp10.Delivery) consigning[[]amqp10.Delivery] {
				// Settled before the assertions, so a failing run does not
				// leave the node holding what it read for the next one.
				return accepting(receiver, arrived).As(arrived)
			})
	})

	arrived, ok := exit.Value()
	if !ok {
		t.Fatalf("unexpected exit: %+v", exit)
	}
	if len(arrived) != 1 {
		t.Fatalf("expected one delivery, got %d", len(arrived))
	}
	if string(arrived[0].Body) != string(sent.Body) {
		t.Errorf("unexpected body: %s", arrived[0].Body)
	}
	// The subject, because a broker's own rules are usually written against it.
	if arrived[0].Subject != sent.Subject {
		t.Errorf("unexpected subject: %q", arrived[0].Subject)
	}
	for _, property := range everyPropertyKind().Fields {
		held, present := arrived[0].Properties.Member(property.Name)
		if !present {
			t.Errorf("%s did not survive the broker", property.Name)
			continue
		}
		if !sameValue(held, property.Value) {
			t.Errorf("%s came back as %#v, sent %#v", property.Name, held, property.Value)
		}
	}
}

func TestTheProgramRunsAgainstARealNode(t *testing.T) {
	// The example itself, not the adapter: a shipment handed over, collected,
	// and accepted, with the four dispositions going through the library rather
	// than through the in-process broker.
	exit := linked(t, func(
		sender amqp10.Sending,
		receiver amqp10.Receiving,
	) consigning[[]consign.Shipment] {
		return consign.Hand(sender, consignment).
			FlatMap(func(effect.Unit) consigning[[]consign.Shipment] {
				return effect.RunCollect(consign.Collect(receiver, taking).TakeStream(1))
			})
	})

	collected, ok := exit.Value()
	if !ok {
		t.Fatalf("unexpected exit: %+v", exit)
	}
	if len(collected) != 1 || collected[0] != consignment {
		t.Fatalf("unexpected shipments: %#v", collected)
	}
}

// everyPropertyKind is one property of each kind that survives a round trip
// through a broker unchanged.
//
// Not the whole representation: a broker sends the narrowest type that fits, so
// a float32 would come back as one and the comparison would be about the broker
// rather than about the boundary.
func everyPropertyKind() dynamic.Object {
	return dynamic.Object{Fields: []dynamic.Field{
		{Name: "boolean", Value: dynamic.OfBoolean(true)},
		{Name: "integer", Value: dynamic.OfInteger(7)},
		{Name: "number", Value: dynamic.OfNumber(1.5)},
		{Name: "text", Value: dynamic.OfText("held")},
	}}
}

// accepting settles every delivery a test read, so a run leaves the node as it
// found it.
func accepting(link amqp10.Receiving, arrived []amqp10.Delivery) consigning[effect.Unit] {
	return effect.ForEach(arrived, func(delivery amqp10.Delivery) consigning[effect.Unit] {
		return effect.Try(
			func(ctx context.Context, _ effect.Unit) (effect.Unit, error) {
				return effect.Unit{}, link.Accept(ctx, delivery.Tag)
			},
			func(err error) amqp10.Fault {
				return amqp10.Fault{Doing: "accepting a message", Err: err}
			},
		)
	}).As(effect.Unit{})
}
