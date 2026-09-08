package acceptance

// The consign program against a broker that runs inside the test.
//
// What is checked is the disposition each outcome produced, because that is the
// part of at-least-once delivery the caller decides -- and the reason AMQP 1.0
// is a separate package is that there are four of them where 0-9-1 has three.
// The program itself never sees the broker.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mbauer83/effect-golang-web/amqp10"
	"github.com/mbauer83/effect-golang-web/amqp10/inprocess"
	"github.com/mbauer83/effect-golang-web/examples/consign"
	"github.com/mbauer83/effect-golang/effect"
)

type consigning[A any] = effect.Effect[effect.Unit, amqp10.Fault, A]

// consigned declares the node, attaches both links, and runs the work.
func consigned[A any](
	t *testing.T,
	work func(amqp10.Sending, amqp10.Receiving) consigning[A],
) (*inprocess.Broker, effect.Exit[amqp10.Fault, A]) {
	t.Helper()
	runtime, err := effect.NewRuntime(effect.WithDebugTracking())
	if err != nil {
		t.Fatal(err)
	}
	broker := inprocess.NewBroker()
	broker.Declare(consign.Consignments)

	sender, err := broker.Sender(consign.Consignments)
	if err != nil {
		t.Fatal(err)
	}
	receiver, err := broker.Receiver(consign.Consignments)
	if err != nil {
		t.Fatal(err)
	}

	within, giveUp := context.WithTimeout(context.Background(), 10*time.Second)
	defer giveUp()

	exit := runtime.Run(within, effect.Unit{}, work(sender, receiver))
	if live := runtime.LiveWork(); !live.IsEmpty() {
		t.Fatalf("the program left work behind: %#v", live)
	}
	return broker, exit
}

var consignment = consign.Shipment{
	Reference: "8f14e45f-ceea-467a-a4fb-1a9c73d0f2b1",
	Carrier:   "overland",
	Weight:    12.5,
}

// taking is a carrier that takes everything.
func taking(consign.Shipment) consigning[consign.Outcome] {
	return effect.For[effect.Unit, amqp10.Fault]().Succeed[consign.Outcome](consign.Collected{})
}

func TestAShipmentCollectedIsAccepted(t *testing.T) {
	broker, exit := consigned(t, func(
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
	if accepted := broker.Accepted(); len(accepted) != 1 {
		t.Fatalf("expected one acceptance, got %v", accepted)
	}
	if waiting := broker.Waiting(consign.Consignments); waiting != 0 {
		t.Fatalf("expected the node empty, got %d waiting", waiting)
	}
}

func TestAShipmentNobodyCanReadIsRejectedWithTheReason(t *testing.T) {
	// Rejected rather than given back: nothing will ever read it, so putting it
	// back would be asking the next receiver to fail the same way. The reason
	// travels with it, because whoever reads the dead-letter node afterwards
	// has the only explanation there is going to be.
	broker, exit := consigned(t, func(
		sender amqp10.Sending,
		receiver amqp10.Receiving,
	) consigning[[]consign.Shipment] {
		return amqp10.Send[effect.Unit](sender, amqp10.Message{
			Body: []byte(`{"reference":"not a uuid","carrier":"","weight":0}`),
		}).
			FlatMap(func(effect.Unit) consigning[[]consign.Shipment] {
				return effect.RunCollect(consign.Collect(receiver, taking).TakeStream(1))
			})
	})

	collected, ok := exit.Value()
	if !ok {
		t.Fatalf("unexpected exit: %+v", exit)
	}
	if len(collected) != 0 {
		t.Fatalf("expected nothing collected, got %#v", collected)
	}
	rejected := broker.Rejected()
	if len(rejected) != 1 {
		t.Fatalf("expected one rejection, got %v", rejected)
	}
	if !strings.Contains(rejected[0].Reason, "cannot be read") {
		t.Errorf("expected the reason to say so, got %q", rejected[0].Reason)
	}
	// And it names the field, because the reason carries the schema's own.
	if !strings.Contains(rejected[0].Reason, "reference") {
		t.Errorf("expected the field named, got %q", rejected[0].Reason)
	}
}
