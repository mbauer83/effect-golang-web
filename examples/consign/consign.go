// Package consign hands shipments to a carrier over AMQP 1.0.
//
// It depends on the port and never on a library, which is the point of there
// being one: the program is written once, the test supplies an in-process
// broker and a deployment supplies Service Bus or ActiveMQ.
//
// What it exists to show is the fourth disposition. AMQP 0-9-1 can accept a
// message, drop it, or put it back; 1.0 can also put it back and say something
// about it -- that the delivery was attempted and failed, or that this receiver
// cannot handle it and another should. Those are different facts, and a broker
// deciding whether a message has failed too often needs them told apart.
package consign

import (
	"strconv"

	"github.com/mbauer83/effect-golang-schema/schema"
	"github.com/mbauer83/effect-golang-schema/schema/dynamic"
	"github.com/mbauer83/effect-golang-web/amqp10"
	"github.com/mbauer83/effect-golang/effect"
)

// Shipment is one consignment.
type Shipment struct {
	Reference string
	Carrier   string
	Weight    float64
}

// ShipmentSchema describes a shipment: the wire, and nothing else, because a
// message has no other job.
var ShipmentSchema = schema.Struct[Shipment]("Shipment",
	schema.FieldOf("reference", schema.UUID(),
		func(shipment Shipment) string { return shipment.Reference },
		func(shipment *Shipment, reference string) { shipment.Reference = reference }),
	schema.FieldOf("carrier", schema.MinLength(schema.Text(), 1),
		func(shipment Shipment) string { return shipment.Carrier },
		func(shipment *Shipment, carrier string) { shipment.Carrier = carrier }),
	schema.FieldOf("weight", schema.Above(schema.Float64(), 0),
		func(shipment Shipment) float64 { return shipment.Weight },
		func(shipment *Shipment, weight float64) { shipment.Weight = weight }),
).Documented("one shipment to be consigned")

type consigning[A any] = effect.Effect[effect.Unit, amqp10.Fault, A]

// Consignments is the node shipments are sent to. A program's addresses come
// from the broker's configuration, so it names the one it was told.
const Consignments = "consignments"

// Hand sends one shipment.
//
// Lasting, because a shipment the broker forgot in a restart is a shipment the
// customer paid for and no carrier will collect. The reference is the subject,
// which is the field a broker's own rules are usually written against.
func Hand(link amqp10.Sending, shipment Shipment) consigning[effect.Unit] {
	return effect.For[effect.Unit, amqp10.Fault]().
		Suspend(func() consigning[effect.Unit] {
			message, err := amqp10.Encoded(ShipmentSchema, shipment)
			if err != nil {
				return effect.For[effect.Unit, amqp10.Fault]().
					Fail[effect.Unit](amqp10.Fault{Doing: "handing over a shipment", Err: err})
			}
			message.Durability = amqp10.Lasting
			message.Subject = shipment.Reference
			return amqp10.Send[effect.Unit](link, message)
		})
}

// Outcome is what the carrier did with a shipment.
//
// A sealed sum rather than a struct with an empty reason, because "took it" and
// "would not take it" are different answers and a zero value standing for one
// of them is the kind of thing a caller forgets to check.
type Outcome interface {
	outcome()
}

// Collected means the carrier took it.
type Collected struct{}

// Refused means it did not, and says how final that is.
type Refused struct {
	Finality Finality
	Reason   string
}

func (Collected) outcome() {}
func (Refused) outcome()   {}

// Finality is how final a refusal is. Three states rather than two booleans,
// because they are three and each maps to exactly one disposition.
type Finality uint8

const (
	// NotNow is this carrier failing an attempt it may succeed at later.
	NotNow Finality = iota
	// NotMe is this carrier being the wrong one, where another may do.
	NotMe
	// Never is nobody being able to take it, so retrying is pointless.
	Never
)

// Collect reads the node and offers each shipment to the carrier, streaming
// the ones collected.
//
// Every outcome is a different disposition, which is the whole shape:
//
//   - collected: accepted, and the broker may forget it.
//   - unreadable: rejected with the reason, because nothing will ever read it
//     and the dead-letter node is where a message nobody can read belongs.
//   - refused, Never: rejected, for the same reason -- the carrier has said
//     retrying is pointless.
//   - refused, NotMe: modified as undeliverable here, so the broker stops
//     offering it to this receiver.
//   - refused, NotNow: modified as tried, which moves the broker's count of
//     failed deliveries. A release would put it back saying nothing, and a
//     message that had failed nine times would look like one arriving fresh.
func Collect(
	link amqp10.Receiving,
	carrier func(Shipment) consigning[Outcome],
) effect.Stream[effect.Unit, amqp10.Fault, Shipment] {
	return effect.CollectStreamEffect(
		amqp10.Values[effect.Unit](link, ShipmentSchema),
		func(received amqp10.Received[Shipment]) consigning[effect.Chunk[Shipment]] {
			return collecting(received, carrier)
		})
}

// collecting is what happens to one delivery.
func collecting(
	received amqp10.Received[Shipment],
	carrier func(Shipment) consigning[Outcome],
) consigning[effect.Chunk[Shipment]] {
	shipment, err := received.Read()
	if err != nil {
		return amqp10.Reject[effect.Unit](received, "the shipment cannot be read: "+err.Error()).
			As(effect.ChunkOf[Shipment]())
	}
	return carrier(shipment).
		FlatMap(func(outcome Outcome) consigning[effect.Chunk[Shipment]] {
			return settled(received, shipment, outcome)
		})
}

// settled turns the carrier's answer into the disposition that says it.
func settled(
	received amqp10.Received[Shipment],
	shipment Shipment,
	outcome Outcome,
) consigning[effect.Chunk[Shipment]] {
	refused, declined := outcome.(Refused)
	if !declined {
		return amqp10.Accept[effect.Unit](received).As(effect.ChunkOf(shipment))
	}
	if refused.Finality == Never {
		return amqp10.Reject[effect.Unit](received, refused.Reason).
			As(effect.ChunkOf[Shipment]())
	}
	return amqp10.Modify[effect.Unit](received, amqp10.Change{
		Tried:     refused.Finality == NotNow,
		Elsewhere: refused.Finality == NotMe,
		Annotations: recorded("refused-because", refused.Reason,
			"attempts", strconv.FormatUint(uint64(received.Delivery.Attempts+1), 10)),
	}).As(effect.ChunkOf[Shipment]())
}

// recorded is what to write on a message being given back, so whoever gets it
// next knows what happened to it here.
func recorded(pairs ...string) dynamic.Object {
	written := dynamic.Object{}
	for index := 0; index+1 < len(pairs); index += 2 {
		written.Fields = append(written.Fields,
			dynamic.Field{Name: pairs[index], Value: dynamic.OfText(pairs[index+1])})
	}
	return written
}
