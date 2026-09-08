package acceptance

// The dispatch program against a broker that runs inside the test.
//
// What is checked is the part of at-least-once delivery the caller decides:
// which deliveries were accepted, which were discarded, and which came back.
// The program itself never sees the broker -- it depends on the port, which is
// the whole point of there being one.

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/mbauer83/effect-golang-web/amqp091"
	"github.com/mbauer83/effect-golang-web/amqp091/inprocess"
	"github.com/mbauer83/effect-golang-web/examples/dispatch"
	"github.com/mbauer83/effect-golang/effect"
)

type dispatching[A any] = effect.Effect[effect.Unit, amqp091.Fault, A]

// dispatched makes a broker, states the topology, and runs the work.
//
// One broker per test, because a shared one shares its queues and then two
// tests fail in an order-dependent way.
func dispatched[A any](
	t *testing.T,
	work func(*inprocess.Broker) dispatching[A],
) (*inprocess.Broker, effect.Exit[amqp091.Fault, A]) {
	t.Helper()
	runtime, err := effect.NewRuntime(effect.WithDebugTracking())
	if err != nil {
		t.Fatal(err)
	}
	broker := inprocess.NewBroker()

	program := dispatch.Prepare(broker).
		FlatMap(func(effect.Unit) dispatching[A] { return work(broker) })

	// With a deadline: a consumer waits for a delivery that may never come, and
	// a test that hung would take the suite with it.
	within, giveUp := context.WithTimeout(context.Background(), 10*time.Second)
	defer giveUp()

	exit := runtime.Run(within, effect.Unit{}, program)
	if live := runtime.LiveWork(); !live.IsEmpty() {
		t.Fatalf("the program left work behind: %#v", live)
	}
	return broker, exit
}

var placed = []dispatch.Order{
	{Reference: "8f14e45f-ceea-467a-a4fb-1a9c73d0f2b1", Item: "lamp", Quantity: 2},
	{Reference: "c9f0f895-fb98-4b1e-9f1e-6a1c9d0e2b34", Item: "desk", Quantity: 1},
}

// packing is a warehouse that packs everything.
func packing(dispatch.Order) dispatching[effect.Unit] {
	return effect.For[effect.Unit, amqp091.Fault]().Succeed(effect.Unit{})
}

func TestAnOrderPlacedIsShippedAndAccepted(t *testing.T) {
	broker, exit := dispatched(t, func(broker *inprocess.Broker) dispatching[[]dispatch.Order] {
		return effect.ForEach(placed, func(order dispatch.Order) dispatching[effect.Unit] {
			return dispatch.Place(broker, order)
		}).
			FlatMap(func([]effect.Unit) dispatching[[]dispatch.Order] {
				// Two of the queue: the consumer decides how many it reads, and
				// the subscription ends when it is finished.
				return effect.RunCollect(dispatch.Ship(broker, packing).TakeStream(2))
			})
	})

	shipped, ok := exit.Value()
	if !ok {
		t.Fatalf("unexpected exit: %+v", exit)
	}
	if !reflect.DeepEqual(shipped, placed) {
		t.Fatalf("unexpected shipments: %#v", shipped)
	}
	// Accepted, so the broker may forget them. That is the assertion a real
	// broker could not answer.
	if accepted := broker.Accepted(); len(accepted) != 2 {
		t.Fatalf("expected both accepted, got %v", accepted)
	}
	if requeued := broker.Requeued(); len(requeued) != 0 {
		t.Fatalf("expected nothing sent back, got %v", requeued)
	}
}

func TestAMessageTheSchemaRefusesIsDiscardedAndTheNextOneIsRead(t *testing.T) {
	// A queue is not a conversation: one unreadable message says nothing about
	// the next, and there is nobody to send it back to. So it is discarded and
	// the consumer carries on, which a websocket deliberately does not do.
	broker, exit := dispatched(t, func(broker *inprocess.Broker) dispatching[[]dispatch.Order] {
		return amqp091.Publish[effect.Unit](broker,
			amqp091.Target{Exchange: dispatch.Orders, Key: dispatch.Placed},
			amqp091.Message{Body: []byte(`{"reference":"not a uuid","item":"","quantity":0}`)}).
			FlatMap(func(effect.Unit) dispatching[effect.Unit] {
				return dispatch.Place(broker, placed[0])
			}).
			FlatMap(func(effect.Unit) dispatching[[]dispatch.Order] {
				return effect.RunCollect(dispatch.Ship(broker, packing).TakeStream(1))
			})
	})

	shipped, ok := exit.Value()
	if !ok {
		t.Fatalf("unexpected exit: %+v", exit)
	}
	if len(shipped) != 1 || shipped[0] != placed[0] {
		t.Fatalf("expected the readable order shipped, got %#v", shipped)
	}
	if discarded := broker.Discarded(); len(discarded) != 1 {
		t.Fatalf("expected the unreadable message discarded, got %v", discarded)
	}
	if accepted := broker.Accepted(); len(accepted) != 1 {
		t.Fatalf("expected only the readable one accepted, got %v", accepted)
	}
}

func TestAnOrderThePackingRefusesComesBackAndIsShippedWhenItCan(t *testing.T) {
	// The other half of the decision: the packing refusing is usually the
	// warehouse being busy rather than the order being wrong, so the order is
	// still there for whoever is not busy -- and this is the witness that it
	// really came back rather than being counted and dropped, because the same
	// order is packed on the third attempt.
	attempts := 0
	busyTwice := func(dispatch.Order) dispatching[effect.Unit] {
		return effect.For[effect.Unit, amqp091.Fault]().
			Suspend(func() dispatching[effect.Unit] {
				attempts++
				if attempts > 2 {
					return effect.For[effect.Unit, amqp091.Fault]().Succeed(effect.Unit{})
				}
				return effect.For[effect.Unit, amqp091.Fault]().
					Fail[effect.Unit](amqp091.Fault{Doing: "packing", Err: errBusy})
			})
	}

	broker, exit := dispatched(t, func(broker *inprocess.Broker) dispatching[[]dispatch.Order] {
		return dispatch.Place(broker, placed[0]).
			FlatMap(func(effect.Unit) dispatching[[]dispatch.Order] {
				return effect.RunCollect(dispatch.Ship(broker, busyTwice).TakeStream(1))
			})
	})

	shipped, ok := exit.Value()
	if !ok {
		t.Fatalf("unexpected exit: %+v", exit)
	}
	if len(shipped) != 1 || shipped[0] != placed[0] {
		t.Fatalf("expected the order shipped on the third attempt, got %#v", shipped)
	}
	if requeued := broker.Requeued(); len(requeued) != 2 {
		t.Fatalf("expected two refusals sent back, got %v", requeued)
	}
	if accepted := broker.Accepted(); len(accepted) != 1 {
		t.Fatalf("expected one acceptance, got %v", accepted)
	}
	// Nothing left waiting: the order was taken, sent back twice, and settled.
	if waiting := broker.Waiting(dispatch.Shipping); waiting != 0 {
		t.Fatalf("expected the queue empty, got %d waiting", waiting)
	}
}

var errBusy = errors.New("the warehouse is busy")
