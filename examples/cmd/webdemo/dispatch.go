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
	packed := 0

	// Refuses once, then packs: an order sent back is offered again, which is
	// what requeueing means.
	packing := func(dispatch.Order) effect.Effect[effect.Unit, amqp091.Fault, effect.Unit] {
		return effect.For[effect.Unit, amqp091.Fault]().
			Suspend(func() effect.Effect[effect.Unit, amqp091.Fault, effect.Unit] {
				packed++
				if packed == 1 {
					return effect.For[effect.Unit, amqp091.Fault]().
						Fail[effect.Unit](amqp091.Fault{Doing: "packing", Err: errWarehouseBusy})
				}
				return effect.For[effect.Unit, amqp091.Fault]().Succeed(effect.Unit{})
			})
	}

	program := dispatch.Prepare(broker).
		FlatMap(func(effect.Unit) effect.Effect[effect.Unit, amqp091.Fault, effect.Unit] {
			// A body nothing can read, so the discard is shown rather than
			// described.
			return amqp091.Publish[effect.Unit](broker,
				amqp091.Target{Exchange: dispatch.Orders, Key: dispatch.Placed},
				amqp091.Message{Body: []byte(`{"reference":"not a uuid"}`)})
		}).
		FlatMap(func(effect.Unit) effect.Effect[effect.Unit, amqp091.Fault, effect.Unit] {
			return dispatch.Place(broker, dispatch.Order{
				Reference: "8f14e45f-ceea-467a-a4fb-1a9c73d0f2b1",
				Item:      "lamp",
				Quantity:  2,
			})
		}).
		FlatMap(func(effect.Unit) effect.Effect[effect.Unit, amqp091.Fault, []dispatch.Order] {
			return effect.RunCollect(dispatch.Ship(broker, packing).TakeStream(1))
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
