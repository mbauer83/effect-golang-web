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

	// Refuses once and then takes it, because a shipment given back is attempts
	// again: a carrier that always refused would loop, which is what the
	// disposition means.
	attempts := 0
	carrier := func(consign.Shipment) consignEffect[consign.Outcome] {
		return effect.For[effect.Unit, amqp10.Fault]().
			Suspend(func() consignEffect[consign.Outcome] {
				attempts++
				if attempts > 1 {
					return effect.For[effect.Unit, amqp10.Fault]().
						Succeed[consign.Outcome](consign.Collection{})
				}
				return effect.For[effect.Unit, amqp10.Fault]().
					Succeed[consign.Outcome](consign.Refusal{
					Finality: consign.NotNow,
					Reason:   "no room on today's van",
				})
			})
	}

	// Direct style: three things in order, which as FlatMaps read inside-out
	// with the last nested deepest. No defer in the body, which is the
	// condition for using it.
	program := effect.Gen(func(do *consignDo) []consign.Shipment {
		do.Await(amqp10.Send[effect.Unit](sender, amqp10.Message{
			Body: []byte(`{"reference":"not a uuid","carrier":"","weight":0}`),
		}))
		do.Await(consign.Hand(sender, consign.Shipment{
			Reference: "8f14e45f-ceea-467a-a4fb-1a9c73d0f2b1",
			Carrier:   "overland",
			Weight:    12.5,
		}))
		return do.Await(effect.RunCollect(consign.Collect(receiver, carrier).TakeStream(1)))
	})

	exit := runtime.Run(context.Background(), effect.Unit{}, program)
	shipments, succeeded := exit.Value()
	if !succeeded {
		fail(fmt.Errorf("consign: %v", exit))
	}

	fmt.Printf("consign: collected %d, accepted %d, rejected %d, given back %d\n",
		len(shipments), len(broker.Accepted()), len(broker.Rejected()), len(broker.Modified()))
	for _, rejection := range broker.Rejected() {
		fmt.Printf("  rejected: %s\n", rejection.Reason)
	}
	for _, modification := range broker.Modified() {
		fmt.Printf("  given back: delivery failed=%v undeliverable here=%v\n",
			modification.Change.DeliveryFailed, modification.Change.UndeliverableHere)
	}
}

// The channel this scenario works in, and the Do it awaits through.
type consignEffect[A any] = effect.Effect[effect.Unit, amqp10.Fault, A]
type consignDo = effect.Do[effect.Unit, amqp10.Fault]
