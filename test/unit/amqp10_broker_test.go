package unit

// The AMQP 1.0 port, driven directly.
//
// What the acceptance suite checks is the program; what this checks is the
// port's own rules -- the ones a real broker enforces and the ones the fourth
// disposition depends on. A modified message's attempt count is here rather
// than there because it is broker behaviour, and the program only reads it.

import (
	"context"
	"testing"

	"github.com/mbauer83/effect-golang-web/amqp10"
	"github.com/mbauer83/effect-golang-web/amqp10/inprocess"
	"github.com/mbauer83/effect-golang-web/schema/dynamic"
	"github.com/mbauer83/effect-golang/effect"
)

type consigning[A any] = effect.Effect[effect.Unit, amqp10.Fault, A]

// attached declares a node and hands back a link each way.
func attached(t *testing.T, address string) (*inprocess.Broker, amqp10.Sending, amqp10.Receiving) {
	t.Helper()
	broker := inprocess.NewBroker()
	broker.Declare(address)
	sender, err := broker.Sender(address)
	if err != nil {
		t.Fatal(err)
	}
	receiver, err := broker.Receiver(address)
	if err != nil {
		t.Fatal(err)
	}
	return broker, sender, receiver
}

func TestALinkIsRefusedAtAnAddressNoNodeIsAt(t *testing.T) {
	// A real broker refuses at attach, before a single message, because the
	// address is its own configuration. A fake that invented the node would
	// hide a wrong address until deployment.
	broker := inprocess.NewBroker()
	if _, err := broker.Sender("nowhere"); err == nil {
		t.Error("expected a sender to be refused")
	}
	if _, err := broker.Receiver("nowhere"); err == nil {
		t.Error("expected a receiver to be refused")
	}
}

func TestGivingAMessageBackAsTriedMovesTheCountAndReleasingDoesNot(t *testing.T) {
	// This is the difference between the two dispositions, and the reason 1.0
	// is a separate package: a broker deciding whether a message has failed too
	// often counts the attempts, and a release says the message is back and
	// nothing else. In 0-9-1 there is only the release.
	broker, sender, receiver := attached(t, "consignments")

	program := amqp10.Send[effect.Unit](sender, amqp10.Message{Body: []byte("{}")}).
		FlatMap(func(effect.Unit) consigning[[]amqp10.Delivery] {
			return effect.RunCollect(amqp10.Receive[effect.Unit](receiver).TakeStream(1))
		}).
		FlatMap(func(first []amqp10.Delivery) consigning[[]amqp10.Delivery] {
			if len(first) != 1 || first[0].Attempts != 0 {
				t.Errorf("expected a fresh delivery, got %#v", first)
			}
			return settledTried(receiver, first[0], "busy").
				AndThen(effect.RunCollect(amqp10.Receive[effect.Unit](receiver).TakeStream(1)))
		}).
		FlatMap(func(second []amqp10.Delivery) consigning[[]amqp10.Delivery] {
			if len(second) != 1 || second[0].Attempts != 1 {
				t.Errorf("expected one attempt recorded, got %#v", second)
			}
			// Released this time: back again, and the count has not moved.
			return settledBack(receiver, second[0]).
				AndThen(effect.RunCollect(amqp10.Receive[effect.Unit](receiver).TakeStream(1)))
		})

	exit := effect.Run(context.Background(), effect.Unit{}, program)
	third, ok := exit.Value()
	if !ok {
		t.Fatalf("unexpected exit: %+v", exit)
	}
	if len(third) != 1 || third[0].Attempts != 1 {
		t.Fatalf("expected the count unmoved by the release, got %#v", third)
	}
	if len(broker.Modified()) != 1 || len(broker.Released()) != 1 {
		t.Fatalf("expected one of each, got %v and %v", broker.Modified(), broker.Released())
	}
}

func TestSettlingTheSameDeliveryTwiceIsRefused(t *testing.T) {
	// The protocol settles a delivery once. A second disposition would be
	// naming a delivery the broker has already finished with, so it is refused
	// rather than sent -- and the message is not put back a second time.
	_, sender, receiver := attached(t, "consignments")

	program := amqp10.Send[effect.Unit](sender, amqp10.Message{Body: []byte("{}")}).
		FlatMap(func(effect.Unit) consigning[[]amqp10.Delivery] {
			return effect.RunCollect(amqp10.Receive[effect.Unit](receiver).TakeStream(1))
		}).
		FlatMap(func(arrived []amqp10.Delivery) consigning[effect.Unit] {
			return settledDone(receiver, arrived[0]).
				AndThen(settledDone(receiver, arrived[0]))
		})

	exit := effect.Run(context.Background(), effect.Unit{}, program)
	if exit.IsSuccess() {
		t.Fatalf("expected the second settlement to be refused, got %+v", exit)
	}
}

// The three settlements this file needs, each built the way a program builds
// one: a Received carries the link, so it is made by reading through Values.
func settledTried(link amqp10.Receiving, delivery amqp10.Delivery, why string) consigning[effect.Unit] {
	return effect.Try(
		func(ctx context.Context, _ effect.Unit) (effect.Unit, error) {
			return effect.Unit{}, link.Modify(ctx, delivery.Tag, amqp10.Change{
				Tried: true,
				Annotations: dynamic.Object{Fields: []dynamic.Field{
					{Name: "refused-because", Value: dynamic.OfText(why)},
				}},
			})
		},
		func(err error) amqp10.Fault {
			return amqp10.Fault{Doing: "modifying a message", Err: err}
		},
	)
}

func settledBack(link amqp10.Receiving, delivery amqp10.Delivery) consigning[effect.Unit] {
	return effect.Try(
		func(ctx context.Context, _ effect.Unit) (effect.Unit, error) {
			return effect.Unit{}, link.Release(ctx, delivery.Tag)
		},
		func(err error) amqp10.Fault {
			return amqp10.Fault{Doing: "releasing a message", Err: err}
		},
	)
}

func settledDone(link amqp10.Receiving, delivery amqp10.Delivery) consigning[effect.Unit] {
	return effect.Try(
		func(ctx context.Context, _ effect.Unit) (effect.Unit, error) {
			return effect.Unit{}, link.Accept(ctx, delivery.Tag)
		},
		func(err error) amqp10.Fault {
			return amqp10.Fault{Doing: "accepting a message", Err: err}
		},
	)
}
