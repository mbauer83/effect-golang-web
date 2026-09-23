package main

// The producer and the consumer, against a broker that runs in this process.
//
// No broker to install and none to reach, because the program depends on the
// port: the same code runs against RabbitMQ by opening a channel instead. What
// is shown is the part of at-least-once delivery the caller decides -- one
// order shipped and accepted, one message discarded because nothing could read
// it, and one order sent back because the warehouse was busy.

import (
	"context"
	"errors"
	"fmt"

	"github.com/mbauer83/effect-golang-web/amqp091"
	"github.com/mbauer83/effect-golang-web/amqp091/inprocess"
	"github.com/mbauer83/effect-golang-web/examples/dispatch"
	"github.com/mbauer83/effect-golang/effect"
)

var errWarehouseBusy = errors.New("the warehouse is busy")

func runDispatch(runtime *effect.Runtime) {
	broker := inprocess.NewBroker()
	attempts := 0

	// Refuses once, then packs: an order sent back is offered again, which is
	// what requeueing means.
	pack := func(dispatch.Order) dispatchEffect[effect.Unit] {
		return effect.For[effect.Unit, amqp091.Fault]().
			Suspend(func() dispatchEffect[effect.Unit] {
				attempts++
				if attempts == 1 {
					return effect.For[effect.Unit, amqp091.Fault]().
						Fail[effect.Unit](amqp091.Fault{Op: "packing", Err: errWarehouseBusy})
				}
				return effect.For[effect.Unit, amqp091.Fault]().Succeed(effect.Unit{})
			})
	}

	// Direct style. Four things happen in order, and as FlatMaps that read
	// inside-out: the last step was nested deepest and each closure existed
	// only to say "then". The body holds no defer, which is the condition --
	// in direct style a defer runs on an ordinary domain failure and not only
	// on a panic.
	program := effect.Gen(func(do *dispatchDo) []dispatch.Order {
		do.Await(dispatch.Prepare(broker))
		// A body nothing can read, so the discard is shown rather than
		// described.
		do.Await(amqp091.Publish[effect.Unit](broker,
			amqp091.Target{Exchange: dispatch.Orders, Key: dispatch.Placed},
			amqp091.Message{Body: []byte(`{"reference":"not a uuid"}`)}))
		do.Await(dispatch.Place(broker, dispatch.Order{
			Reference: "8f14e45f-ceea-467a-a4fb-1a9c73d0f2b1",
			Item:      "lamp",
			Quantity:  2,
		}))
		return do.Await(effect.RunCollect(dispatch.Ship(broker, pack).TakeStream(1)))
	})

	exit := runtime.Run(context.Background(), effect.Unit{}, program)
	shipped, succeeded := exit.Value()
	if !succeeded {
		fail(fmt.Errorf("dispatch: %v", exit))
	}

	fmt.Printf("dispatch: shipped %d, accepted %d, discarded %d, sent back %d\n",
		len(shipped), len(broker.Accepted()), len(broker.Discarded()), len(broker.Requeued()))
	for _, order := range shipped {
		fmt.Printf("  %s x%d (%s)\n", order.Item, order.Quantity, order.Reference)
	}
}

// The channel this scenario works in, and the binder it binds with, named so a
// signature says what it is rather than repeating itself.
type dispatchEffect[A any] = effect.Effect[effect.Unit, amqp091.Fault, A]
type dispatchDo = effect.Do[effect.Unit, amqp091.Fault]
