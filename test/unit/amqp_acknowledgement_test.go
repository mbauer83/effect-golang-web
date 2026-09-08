package unit

// Settling a delivery: the part of at-least-once delivery the caller decides.
//
// The in-process broker records which deliveries were accepted, discarded and
// requeued, which is the question a real broker cannot be asked. What it does
// not model is prefetch -- so how a broker behaves towards a consumer that
// never settles is the gated acceptance suite's to answer, not this one's.

import (
	"testing"

	"github.com/mbauer83/effect-golang-web/amqp091"
	"github.com/mbauer83/effect-golang-web/amqp091/inprocess"
	"github.com/mbauer83/effect-golang/effect"
)

func TestAnUndecodedDeliveryCanStillBeAcknowledged(t *testing.T) {
	// Which is the whole reason Consume yields a Received rather than a
	// Delivery. Acknowledgement belongs to the subscription, so an element
	// parted from its own could not be settled at all -- and a consumer that
	// never settles is one a real broker stops sending to.
	broker := inprocess.NewBroker()
	program := amqp091.DeclareQueue[effect.Unit](broker, amqp091.Queue{Name: "raw"}).
		FlatMap(func(effect.Unit) queueing[effect.Unit] {
			return amqp091.Publish[effect.Unit](broker,
				amqp091.Target{Key: "raw"}, amqp091.Message{Body: []byte("one")})
		}).
		FlatMap(func(effect.Unit) queueing[[]amqp091.Received[[]byte]] {
			return effect.RunCollect(effect.MapStreamEffect(
				amqp091.Consume[effect.Unit](broker, "raw").TakeStream(1),
				func(received amqp091.Received[[]byte]) queueing[amqp091.Received[[]byte]] {
					return amqp091.Ack[effect.Unit](received).As(received)
				}))
		})

	arrived, ok := ran(t, program).Value()
	if !ok {
		t.Fatalf("expected the delivery read and accepted, got %+v", ran(t, program))
	}
	// Read is the identity decoding: the body as it arrived, and it cannot
	// refuse.
	body, err := arrived[0].Read()
	if err != nil || string(body) != "one" {
		t.Fatalf("unexpected read: %q %v", body, err)
	}
	if accepted := broker.Accepted(); len(accepted) != 1 ||
		accepted[0] != arrived[0].Delivery.Tag {
		t.Fatalf("expected the delivery accepted, got %v", accepted)
	}
	if waiting := broker.Waiting("raw"); waiting != 0 {
		t.Fatalf("expected nothing left waiting, got %d", waiting)
	}
}
