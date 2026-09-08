# AMQP 1.0 reference

`amqp10` carries messages over AMQP 1.0 — the protocol Azure Service Bus,
ActiveMQ and Qpid speak. A connection, a session and a link are scoped
resources, a receiver is a
[`Stream`](https://github.com/mbauer83/effect-golang/blob/main/docs/reference/stream.md),
sending is an effect, and settlement is explicit.

**It is not a variant of [`amqp091`](amqp091.md) and does not sit behind the
same port.** 1.0 has no exchanges, no routing keys and no bindings: a link is
attached to a node by address, and the broker's own configuration decides what a
node is. There is nothing in the protocol to declare one with, which is why
there is no `Topology` here. A delivery is settled by *disposition* — accepted,
rejected, released or modified — which is four outcomes where 0-9-1 has three,
and the fourth says something 0-9-1 cannot say.

What the two do share is the shape of the answer, and that shape is this
runtime's rather than either protocol's.

## Lifetime

```go
amqp10.Connect[R](scope, "amqps://host:5671", nil)
amqp10.Open[R](scope, connection)                        // a session
amqp10.Sender[R](scope, session, "consignments")         // Sending
amqp10.Receiver[R](scope, session, "consignments", 32)   // Receiving
```

A connection multiplexes sessions; a session multiplexes links; a link is
attached to one node in one direction. A program that sends and receives
attaches two links, not two connections. All four are scoped, so none outlives
the effect that asked for it, and anything the broker closed first is not a
fault on the way out.

`Receiver`'s credit is **not** optional the way a 0-9-1 prefetch is. The
protocol is credit-based: a link with no credit receives nothing at all. A
stream is pull-based above the link, but the credit is what stops the broker
filling memory below it.

## Sending

```go
amqp10.Send[R](link, message)
amqp10.SendValue[R](link, ShipmentSchema, shipment)
amqp10.Encoded(ShipmentSchema, shipment) // (Message, error)
```

Unlike a 0-9-1 publish, `Send` waits for the broker to settle the transfer, so
a success means the broker **has** the message rather than that it reached the
socket. That is the protocol's own flow control rather than an extra feature:
0-9-1 needs publisher confirms to say as much.

The body is written as a data section, not an AMQP value section. A value
section carries a typed AMQP value, which would mean encoding the schema's
output twice — once as JSON and once as an AMQP string — and a receiver written
in another language would read a quoted document.

## Receiving, and the four dispositions

```go
amqp10.Receive[R](link)                     // Stream[R, Fault, Delivery]
amqp10.Values[R](link, ShipmentSchema)      // Stream[R, Fault, Received[A]]

func (received Received[A]) Read() (A, error)
amqp10.Accept[R](received)                  // done; the broker may forget it
amqp10.Reject[R](received, reason)          // never; dead-lettered, with the reason
amqp10.Release[R](received)                 // back, unchanged, saying nothing
amqp10.Modify[R](received, change)          // back, with something said about it
```

`Release` and `Modify` both put the message back, and the difference is the
point of this package:

| | what the broker learns |
|---|---|
| `Release` | the message is back |
| `Modify{Tried: true}` | a receiver attempted it and failed — the delivery count moves |
| `Modify{Elsewhere: true}` | this receiver cannot take it; stop offering it here |
| `Modify{Annotations: …}` | whatever the receiver found out, recorded on the message |

A 0-9-1 requeue is a release. So a broker there deciding whether a message has
failed too often has only its own redelivery count to go on, and a consumer
cannot say *why* it gave a message back. `Change` carries two independent facts
rather than one choice of four, because they are independent: a delivery can
have been attempted and failed while still being deliverable here.

`Reject`'s reason travels with the message. Whoever reads the dead-letter node
afterwards has the only explanation there is going to be.

**A message the schema refuses does not fail the stream**, for the reason it
does not in 0-9-1: a link carries separate messages rather than one stateful
conversation. `Read` is a method and not a field because a zero value that
looked valid would be a trap.

**A link detached cleanly ends the stream rather than failing it.** A broker or
an operator closing a link has said there will be no more, and a consumer told
the stream *failed* would retry against a link that is gone.

A message given back is offered again — with one receiver it comes straight
back, so a consumer that gives everything back has written a loop. Telling
*not now* from *not me* from *never* is the application's job, which is what
[`examples/consign`](../../examples/consign/consign.go) is: three finalities,
each mapping to exactly one disposition.

## The port, and the third untyped file

`Sending` and `Receiving` are the whole port — two interfaces because a link
goes one way, and a program that only sends should not depend on four
dispositions it never makes. The dispositions are on the link rather than on a
`Delivery` because it is the link that owes the broker an answer, and a delivery
outliving its link can no longer be settled at all. A delivery is named by its
tag, which is what the protocol names it by; a message with no tag is refused on
arrival rather than arriving as something the consumer will fail to settle.

`Properties`, `Annotations` and `Named` are the boundary. Application properties
and annotations are AMQP maps, which the protocol defines as sets of named
values and the library represents as `map[string]any`; this is the third place
in the module holding a value it cannot name, and the architecture test names
all three. A UUID becomes text, because the representation has no case for one
and its string form is what a schema's `uuid` format reads.

It is deliberately **not** shared with the 0-9-1 field-table boundary, which
does the same-looking thing. The two protocols permit different value sets —
1.0 has unsigned integers, UUIDs, symbols and described types that 0-9-1 has no
encoding for — and the nested map is a different named type in each library.
Merging them would invent a third concept, *a broker's untyped map*, that
neither specification has, and the first value one protocol accepted and the
other did not would split it again.

## Testing without a broker

```go
broker := inprocess.NewBroker()
broker.Declare("consignments")
sender, _ := broker.Sender("consignments")
receiver, _ := broker.Receiver("consignments")

broker.Accepted()  // []string of tags
broker.Rejected()  // []Rejection{Tag, Reason}
broker.Released()  // []string
broker.Modified()  // []Modification{Tag, Change}
```

`amqp10/inprocess` satisfies the port, so a sender and receiver run against it
exactly as they run against Service Bus. What it gives a test that a real broker
cannot is the four dispositions as questions — including *why* something was
rejected and *what was said* about something given back. `Declare` stands in for
the broker's configuration, and a link to an address with no node is refused at
attach, because that is what a real broker refuses.

It is not a broker: messages are held in memory in the order they were sent, a
rejected message is dropped rather than dead-lettered, and filters, selectors,
transactions and credit exhaustion are absent. It says so rather than
pretending.

## What a real broker verifies, and is not verified here

The integration suite is gated on `EFFECT_GOLANG_AMQP10_URL` and
`EFFECT_GOLANG_AMQP10_NODE` — two variables, because a 1.0 broker's addresses
are its own configuration and there is nothing in the protocol to declare one
with. Those tests **skip** where no broker is reachable, and a skipped test is
not evidence: the message sections, the property map, the dispositions as the
library sends them, and the detached-link ending are unverified here.
