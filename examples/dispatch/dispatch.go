// Package dispatch places orders on a queue and ships what arrives.
//
// It depends on the port and never on a broker, which is the point of there
// being one: the program is written once, the test supplies an in-process
// broker and a deployment supplies RabbitMQ. Nothing here imports amqp091-go.
//
// One schema does two jobs: it encodes what is published and decodes what
// arrives, so a producer and a consumer cannot disagree about the shape without
// one of them failing to build.
package dispatch

import (
	"github.com/mbauer83/effect-golang-web/amqp091"
	"github.com/mbauer83/effect-golang-web/schema"
	"github.com/mbauer83/effect-golang/effect"
)

// Order is one order.
type Order struct {
	Reference string
	Item      string
	Quantity  int32
}

// OrderSchema describes an order: the wire, and nothing else, because a message
// has no other job.
var OrderSchema = schema.Struct[Order]("Order",
	schema.FieldOf("reference", schema.UUID(),
		func(order Order) string { return order.Reference },
		func(order *Order, reference string) { order.Reference = reference }),
	schema.FieldOf("item", schema.MinLength(schema.Text(), 1),
		func(order Order) string { return order.Item },
		func(order *Order, item string) { order.Item = item }),
	schema.FieldOf("quantity", schema.AtLeast(schema.Int32(), 1),
		func(order Order) int32 { return order.Quantity },
		func(order *Order, quantity int32) { order.Quantity = quantity }),
).Documented("one order to be shipped")

type dispatching[A any] = effect.Effect[effect.Unit, amqp091.Fault, A]

// Where the messages go. A program that owns its topology says so in one place.
const (
	// Orders is the exchange orders are published to.
	Orders = "orders"
	// Shipping is the queue the shipping side reads.
	Shipping = "shipping"
	// Placed is the routing key of an order that has been placed.
	Placed = "placed"
)

// Topology is what this program needs the broker to hold.
var Topology = amqp091.Topology{
	Exchanges: []amqp091.Exchange{
		{Name: Orders, Routing: amqp091.Direct, Durability: amqp091.Lasting},
	},
	Queues: []amqp091.Queue{
		{Name: Shipping, Durability: amqp091.Lasting},
	},
	Bindings: []amqp091.Binding{
		{Exchange: Orders, Queue: Shipping, Key: Placed},
	},
}

// Prepare states the topology. A program declares it at start-up, so a
// disagreement about a name is heard then rather than at the first message.
func Prepare(channel amqp091.Declaring) dispatching[effect.Unit] {
	return amqp091.Declare[effect.Unit](channel, Topology)
}

// Place publishes one order.
//
// Lasting, because an order the broker forgot in a restart is an order the
// customer placed and nobody will ship.
func Place(channel amqp091.Publishing, order Order) dispatching[effect.Unit] {
	return effect.For[effect.Unit, amqp091.Fault]().
		Suspend(func() dispatching[effect.Unit] {
			message, err := amqp091.Encoded(OrderSchema, order)
			if err != nil {
				return effect.For[effect.Unit, amqp091.Fault]().
					Fail[effect.Unit](amqp091.Fault{Doing: "placing an order", Err: err})
			}
			message.Durability = amqp091.Lasting
			return amqp091.Publish[effect.Unit](channel,
				amqp091.Target{Exchange: Orders, Key: Placed}, message)
		})
}

// Ship reads the queue and packs each order, streaming the ones it shipped.
//
// The acknowledgement is the whole point of the shape. An order the schema
// refuses is discarded: a queue is not a conversation, so one unreadable
// message says nothing about the next, and there is nobody to send it back to.
// An order the packing refused comes back to the queue, because the packing
// refusing is usually the warehouse being busy rather than the order being
// wrong. Only an order that was packed is accepted, and only then does it
// appear in the stream.
//
// A requeued order is offered again, which is what requeueing means: with one
// consumer it comes straight back, so a packing that fails permanently would
// loop. Telling "busy now" from "never going to work" is the application's job
// and cannot be anything else -- which is the reason acknowledgement is
// explicit rather than a policy this package chose.
func Ship(
	channel amqp091.Consuming,
	pack func(Order) dispatching[effect.Unit],
) effect.Stream[effect.Unit, amqp091.Fault, Order] {
	return effect.CollectStreamEffect(
		amqp091.Values[effect.Unit](channel, Shipping, OrderSchema),
		func(received amqp091.Received[Order]) dispatching[effect.Chunk[Order]] {
			return shipping(received, pack)
		})
}

// shipping is what happens to one delivery.
func shipping(
	received amqp091.Received[Order],
	pack func(Order) dispatching[effect.Unit],
) dispatching[effect.Chunk[Order]] {
	order, err := received.Read()
	if err != nil {
		return amqp091.Discard[effect.Unit](received).As(effect.ChunkOf[Order]())
	}
	return pack(order).
		FlatMap(func(effect.Unit) dispatching[effect.Chunk[Order]] {
			return amqp091.Ack[effect.Unit](received).As(effect.ChunkOf(order))
		}).
		CatchAll(func(amqp091.Fault) dispatching[effect.Chunk[Order]] {
			return amqp091.Requeue[effect.Unit](received).As(effect.ChunkOf[Order]())
		})
}
