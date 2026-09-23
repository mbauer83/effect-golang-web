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
	schema.FieldOf("carrier", schema.Text().Check(schema.MinLength(1)),
		func(shipment Shipment) string { return shipment.Carrier },
		func(shipment *Shipment, carrier string) { shipment.Carrier = carrier }),
	schema.FieldOf("weight", schema.Float64().Check(schema.Above[float64](0)),
		func(shipment Shipment) float64 { return shipment.Weight },
		func(shipment *Shipment, weight float64) { shipment.Weight = weight }),
).WithDescription("one shipment to be consigned")

type consignEffect[A any] = effect.Effect[effect.Unit, amqp10.Fault, A]

// Consignments is the node shipments are sent to. A program's addresses come
// from the broker's configuration, so it names the one it was told.
const Consignments = "consignments"

// Hand sends one shipment.
//
// Lasting, because a shipment the broker forgot in a restart is a shipment the
// customer paid for and no carrier will collect. The reference is the subject,
// which is the field a broker's own rules are usually written against.
func Hand(link amqp10.SenderLink, shipment Shipment) consignEffect[effect.Unit] {
	return effect.For[effect.Unit, amqp10.Fault]().
		Suspend(func() consignEffect[effect.Unit] {
			message, err := amqp10.Encode(ShipmentSchema, shipment)
			if err != nil {
				return effect.For[effect.Unit, amqp10.Fault]().
					Fail[effect.Unit](amqp10.Fault{Op: "handing over a shipment", Err: err})
			}
			message.Durability = amqp10.Durable
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

// Collection means the carrier took it.
type Collection struct{}

// Refusal means it did not, and says how final that is.
type Refusal struct {
	Finality Finality
	Reason   string
}

func (Collection) outcome() {}
func (Refusal) outcome()    {}

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
	link amqp10.ReceiverLink,
	carrier func(Shipment) consignEffect[Outcome],
) effect.Stream[effect.Unit, amqp10.Fault, Shipment] {
	return effect.CollectStreamEffect(
		amqp10.Values[effect.Unit](link, ShipmentSchema),
		func(received amqp10.Envelope[Shipment]) consignEffect[effect.Chunk[Shipment]] {
			return collectConsignment(received, carrier)
		})
}

// collectConsignment is what happens to one delivery.
func collectConsignment(
	received amqp10.Envelope[Shipment],
	carrier func(Shipment) consignEffect[Outcome],
) consignEffect[effect.Chunk[Shipment]] {
	// Direct style: offer it, then settle it according to what came back. As a
	// FlatMap the settling was nested inside the offering, which is the wrong
	// way round for something that happens after it.
	return effect.Gen(func(do *consignDo) effect.Chunk[Shipment] {
		shipment, err := received.Read()
		if err != nil {
			return do.Await(amqp10.Reject[effect.Unit](received,
				"the shipment cannot be read: "+err.Error()).As(effect.ChunkOf[Shipment]()))
		}
		outcome := do.Await(carrier(shipment))
		return do.Await(settleConsignment(received, shipment, outcome))
	})
}

// consignDo is the binder this program binds in. No defer in the body, which is
// the condition for direct style.
type consignDo = effect.Do[effect.Unit, amqp10.Fault]

// settleConsignment turns the carrier's answer into the disposition that says it.
func settleConsignment(
	received amqp10.Envelope[Shipment],
	shipment Shipment,
	outcome Outcome,
) consignEffect[effect.Chunk[Shipment]] {
	refused, declined := outcome.(Refusal)
	if !declined {
		return amqp10.Accept[effect.Unit](received).As(effect.ChunkOf(shipment))
	}
	if refused.Finality == Never {
		return amqp10.Reject[effect.Unit](received, refused.Reason).
			As(effect.ChunkOf[Shipment]())
	}
	return amqp10.Modify[effect.Unit](received, amqp10.Change{
		DeliveryFailed:    refused.Finality == NotNow,
		UndeliverableHere: refused.Finality == NotMe,
		Annotations: annotations("refused-because", refused.Reason,
			"attempts", strconv.FormatUint(uint64(received.Delivery.Attempts+1), 10)),
	}).As(effect.ChunkOf[Shipment]())
}

// annotations is what to write on a message being given back, so whoever gets it
// next knows what happened to it here.
func annotations(pairs ...string) dynamic.Object {
	object := dynamic.Object{}
	for index := 0; index+1 < len(pairs); index += 2 {
		object.Fields = append(object.Fields,
			dynamic.Field{Name: pairs[index], Value: dynamic.OfText(pairs[index+1])})
	}
	return object
}
