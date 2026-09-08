package main

// The AMQP 1.0 sender and receiver, against a broker in this process.
//
// What is shown is the disposition each outcome produced, which is the reason
// 1.0 is a separate package: one shipment collected and accepted, one rejected
// because nothing could read it, and one given back with the attempt recorded --
// the last being the outcome AMQP 0-9-1 has no way to express.

import (
	"context"
	"fmt"

	"github.com/mbauer83/effect-golang-web/amqp10"
	"github.com/mbauer83/effect-golang-web/amqp10/inprocess"
	"github.com/mbauer83/effect-golang-web/examples/consign"
	"github.com/mbauer83/effect-golang/effect"
)

func runConsign(runtime *effect.Runtime) {
	broker := inprocess.NewBroker()
	broker.Declare(consign.Consignments)

	sender, err := broker.Sender(consign.Consignments)
	if err != nil {
		fail(err)
	}
	receiver, err := broker.Receiver(consign.Consignments)
	if err != nil {
		fail(err)
	}

	// Refuses once and then takes it, because a shipment given back is offered
	// again: a carrier that always refused would loop, which is what the
	// disposition means.
	offered := 0
	carrier := func(consign.Shipment) effect.Effect[effect.Unit, amqp10.Fault, consign.Outcome] {
		return effect.For[effect.Unit, amqp10.Fault]().
			Suspend(func() effect.Effect[effect.Unit, amqp10.Fault, consign.Outcome] {
				offered++
				if offered > 1 {
					return effect.For[effect.Unit, amqp10.Fault]().
						Succeed[consign.Outcome](consign.Collected{})
				}
				return effect.For[effect.Unit, amqp10.Fault]().
					Succeed[consign.Outcome](consign.Refused{
						Finality: consign.NotNow,
						Reason:   "no room on today's van",
					})
			})
	}

	program := amqp10.Send[effect.Unit](sender, amqp10.Message{
		Body: []byte(`{"reference":"not a uuid","carrier":"","weight":0}`),
	}).
		FlatMap(func(effect.Unit) effect.Effect[effect.Unit, amqp10.Fault, effect.Unit] {
			return consign.Hand(sender, consign.Shipment{
				Reference: "8f14e45f-ceea-467a-a4fb-1a9c73d0f2b1",
				Carrier:   "overland",
				Weight:    12.5,
			})
		}).
		FlatMap(func(effect.Unit) effect.Effect[effect.Unit, amqp10.Fault, []consign.Shipment] {
			return effect.RunCollect(consign.Collect(receiver, carrier).TakeStream(1))
		})

	exit := runtime.Run(context.Background(), effect.Unit{}, program)
	collected, succeeded := exit.Value()
	if !succeeded {
		fail(fmt.Errorf("consign: %v", exit))
	}

	fmt.Printf("consign: collected %d, accepted %d, rejected %d, given back %d\n",
		len(collected), len(broker.Accepted()), len(broker.Rejected()), len(broker.Modified()))
	for _, rejection := range broker.Rejected() {
		fmt.Printf("  rejected: %s\n", rejection.Reason)
	}
	for _, modification := range broker.Modified() {
		fmt.Printf("  given back: tried=%v elsewhere=%v\n",
			modification.Change.Tried, modification.Change.Elsewhere)
	}
}
