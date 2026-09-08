# AMQP 0-9-1 reference

`amqp091` carries messages over AMQP 0-9-1, the protocol RabbitMQ speaks
natively. A connection and a channel are scoped resources, a consumer is a
[`Stream`](https://github.com/mbauer83/effect-golang/blob/main/docs/reference/stream.md),
publishing is an effect, and acknowledgement is explicit.

The version is in the package name. AMQP 1.0 shares the name and almost nothing
else — no exchanges, no routing keys, no bindings, and a delivery is settled by
disposition rather than by acknowledgement — so a port covering both would cover
neither. A plain `amqp` would have read as the general one.

## Lifetime

```go
amqp091.Connect[R](scope, "amqp://user:password@host:5672/vhost")
amqp091.Open[R](scope, connection)
amqp091.Prefetch[R](channel, 32)
```

A connection is a TCP connection; a channel is a multiplexed session on it. A
channel is not safe to share between goroutines that publish and consume at
once, which is the broker's rule rather than this library's — so a program opens
one connection and a channel per concurrent user of it. Both are scoped, so
neither outlives the effect that asked for it, and a channel the broker closed
first is not a fault on the way out.

`Prefetch` is how a consumer keeps the broker from handing it the whole queue.
A stream is pull-based above the port, but the pushing happens below it: with no
limit the broker sends everything it has, and a consumer reading one delivery at
a time would still be holding all of them.

## Publishing

```go
amqp091.Publish[R](channel, amqp091.Target{Exchange: "orders", Key: "placed"}, message)
amqp091.PublishValue[R](channel, target, OrderSchema, order)
amqp091.Encoded(OrderSchema, order) // (Message, error)
```

`Target` is an exchange and the key it routes by. An empty exchange is the
broker's default one, which routes by queue name — so `Target{Key: "orders"}`
publishes straight to the queue called `orders`, which is what a program with no
topology of its own wants.

The value is encoded before anything is sent, so a value the schema refuses is
reported rather than published as a body the consumer would have to make sense
of. `Encoded` is public because `Message` has more to say than a value does — a
durability, headers — and a caller that wants to set those should not have to
choose between them and the schema.

What succeeds is that the message reached the broker's socket. Publisher
confirms are a separate protocol feature and are not this; a program that needs
the broker to say it *has* the message needs them, and that is a thing to add
when a caller asks.

## Consuming, and acknowledgement

```go
amqp091.Consume[R](channel, "shipping")                // Stream[R, Fault, Delivery]
amqp091.Values[R](channel, "shipping", OrderSchema)    // Stream[R, Fault, Received[A]]

func (received Received[A]) Read() (A, error)
amqp091.Ack[R](received)      // the broker may forget it
amqp091.Discard[R](received)  // dropped, or dead-lettered
amqp091.Requeue[R](received)  // back to the queue
```

Three named operations rather than `Reject(requeue bool)`, because "true" does
not say which way round the question was asked.

**A delivery the schema refuses does not fail the stream.** That is the opposite
of a [websocket](websocket.md) and deliberately so: a conversation is stateful,
so a peer that said something unreadable has said something about the whole
exchange — but a queue is a sequence of separate messages, and one that cannot be
read is one message. It arrives as a `Received` whose `Read` refuses, so the
consumer decides. `Read` is a method and not a field because a zero value that
looked valid would be a trap: a consumer that forgot to ask would act on a
message that was never there.

A requeued message is offered again, and with one consumer it comes straight
back — so a consumer that requeues unconditionally has written a loop. Telling
*busy now* from *never going to work* is the application's job and cannot be
anything else, which is the whole reason acknowledgement is explicit.

**The subscription's release cancels the consumer.** A stream that stopped
early — read three of a million, failed, was interrupted — leaves a broker that
has no idea and goes on sending, into a Go channel nobody reads; with a prefetch
set those deliveries are unacknowledged and the queue stalls behind them. It does
not close the channel: the program may still be publishing on it.

## Topology

```go
var Topology = amqp091.Topology{
    Exchanges: []amqp091.Exchange{{Name: "orders", Routing: amqp091.Direct, Durability: amqp091.Lasting}},
    Queues:    []amqp091.Queue{{Name: "shipping", Durability: amqp091.Lasting}},
    Bindings:  []amqp091.Binding{{Exchange: "orders", Queue: "shipping", Key: "placed"}},
}
amqp091.Declare[R](channel, Topology)
```

A program owns its topology, so it says so in one place. `Declare` states the
exchanges and queues before the bindings, because a binding names both and the
broker will not invent either — that ordering is the reason it exists rather
than leaving a caller to write three loops in the right order. Declaring is
idempotent at the broker: a declaration matching what is there succeeds, and one
that contradicts it is refused, which is the broker telling you two programs
disagree about a name and worth hearing at start-up.

## The port, and the two untyped functions

`Publishing`, `Consuming`, `Declaring` and `Deliveries` are the whole port.
Three interfaces rather than one because most programs use one of them: a
producer publishes, a consumer consumes, and whoever owns the topology declares
it, usually once at start-up.

Unlike the port in [sql](sql.md), this one is not there because two
implementations are worth having — amqp091-go is the only serious one. It is
there because a program that publishes and consumes should be testable without a
broker, and because those three operations are what an application actually
depends on.

`Table` and `Headers` are the boundary. A message's headers are an AMQP field
table, which the protocol defines as a set of named values and the library
represents as `map[string]any`; this is the second place in the module holding a
value it cannot name, and the architecture test names the file. It maps better
than the database one: a field table permits every kind the representation has,
nesting included, so nothing is narrowed on the way through. The two are public
so the boundary can be tested from outside, and their signatures say
`map[string]any` rather than the library's named type, so nothing above the
package acquires the dependency by calling them.

## Testing without a broker

```go
broker := inprocess.NewBroker()
// ... a producer and a consumer written against the port
broker.Accepted()   // []uint64
broker.Discarded()
broker.Requeued()
broker.Waiting("shipping")
```

`amqp091/inprocess` satisfies the port, so a producer and consumer run against
it exactly as they run against RabbitMQ. What it gives a test that a real broker
cannot is a question: which deliveries were accepted, which were discarded, and
which came back — the part of at-least-once delivery the caller decides, and so
the part worth asserting on.

It is not a broker. It routes directly and by fanout, holds everything in
memory, and forgets it. Topic routing, dead-lettering, publisher confirms and
durability are absent, and it **says so** rather than pretending: a fake that
answered every question the way the real one does would have to be the real one.
It does enforce the rules a real broker enforces — declare before you bind,
declare before you consume — because a fake that invented names would hide a
missing declaration until deployment.

## What a real broker verifies, and is not verified here

The integration suite is gated on `EFFECT_GOLANG_AMQP_URL`. Everything the
example program does is covered against the in-process broker either way; what
only a real broker can answer is whether the adapter's own decisions are right —
the flags it passes, the field table it writes, and whether cancelling a
consumer really stops the broker sending.

Those tests **skip** where no broker is reachable, and a skipped test is not
evidence — so CI runs them against a RabbitMQ service container, in a job of
their own. They were written on a machine where no broker was reachable, so
until a pushed run is green they establish nothing.

[`examples/dispatch`](../../examples/dispatch/dispatch.go) is the program.
