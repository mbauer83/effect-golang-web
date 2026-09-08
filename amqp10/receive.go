package amqp10

// Receiving from a node. The link is a scoped resource and the messages are a
// Stream, so a consumer reads at the rate it can work and the link is detached
// when the consumer is finished -- including when it stopped early, failed, or
// was cancelled.

import (
	"context"

	"github.com/mbauer83/effect-golang-schema/schema"
	"github.com/mbauer83/effect-golang/effect"
)

// Receive streams what arrives on a link.
//
// A link detached cleanly ends the stream rather than failing it. That is not
// leniency: a broker or an operator closing a link is the end of the messages,
// and a consumer told the stream failed would retry against a link that is
// gone.
func Receive[R any](link Receiving) effect.Stream[R, Fault, Delivery] {
	return effect.StreamFromSteps(func() effect.Effect[R, Fault, effect.Step[Delivery]] {
		return effect.From(func(ctx context.Context, _ R) effect.Exit[Fault, effect.Step[Delivery]] {
			delivery, more, err := link.Receive(ctx)
			if err != nil {
				return effect.ExitFailure[Fault, effect.Step[Delivery]](
					faulted("waiting for a message", link.Address(), err))
			}
			if !more {
				return effect.ExitSuccess[Fault](effect.EndOfStream[Delivery]())
			}
			return effect.ExitSuccess[Fault](effect.Emit(effect.ChunkOf(delivery)))
		})
	})
}

// Values streams what arrives, decoded through the schema.
//
// A message the schema refuses does not fail the stream, for the reason it does
// not in AMQP 0-9-1: a link carries separate messages rather than one stateful
// conversation, so one that cannot be read says something about one message. It
// arrives as a Received whose Read refuses, and the consumer decides -- and here
// there are four things it can decide rather than three.
func Values[R, A any](
	link Receiving,
	shape schema.Schema[A],
) effect.Stream[R, Fault, Received[A]] {
	return effect.MapStream(Receive[R](link),
		func(delivery Delivery) Received[A] { return read(link, delivery, shape) })
}

// Received is one delivery, decoded, with the disposition still to make.
type Received[A any] struct {
	// Delivery is what arrived: the properties, the subject, and how many times
	// the broker has offered it before.
	Delivery Delivery

	value   A
	refusal error
	from    Receiving
}

// Read is the value the delivery carried, or why it could not be read.
//
// A method rather than a field because a zero value that looked valid would be
// a trap: a consumer that forgot to ask would act on a message that was never
// there.
func (received Received[A]) Read() (A, error) {
	return received.value, received.refusal
}

// Accept says the message is done and the broker may forget it.
func Accept[R, A any](received Received[A]) effect.Effect[R, Fault, effect.Unit] {
	return settling[R](received, "accepting a message",
		func(ctx context.Context, tag string) error { return received.from.Accept(ctx, tag) })
}

// Reject says the message will never be processed, and why.
//
// The reason travels with it: the broker records it, and whoever reads the
// dead-letter node afterwards has the only explanation there is going to be.
func Reject[R, A any](received Received[A], reason string) effect.Effect[R, Fault, effect.Unit] {
	return settling[R](received, "rejecting a message",
		func(ctx context.Context, tag string) error {
			return received.from.Reject(ctx, tag, reason)
		})
}

// Release gives the message back unchanged, as though this receiver had never
// had it.
//
// Nothing is recorded, so the broker's count of failed deliveries does not
// move -- which is right when this receiver is shutting down or was never the
// right one, and wrong when it tried and failed. Modify is that case.
func Release[R, A any](received Received[A]) effect.Effect[R, Fault, effect.Unit] {
	return settling[R](received, "releasing a message",
		func(ctx context.Context, tag string) error { return received.from.Release(ctx, tag) })
}

// Modify gives the message back with something said about it.
//
// This is the disposition AMQP 0-9-1 cannot express. A requeue there is a
// release: it says the message is back and nothing else, so a broker deciding
// whether a message has failed too often has only its own redelivery count to
// go on. Here the receiver can say that it tried, that the message should go
// elsewhere, and what it found out -- which is what makes a dead-letter policy
// something the consumer participates in rather than something done to it.
func Modify[R, A any](received Received[A], change Change) effect.Effect[R, Fault, effect.Unit] {
	return settling[R](received, "modifying a message",
		func(ctx context.Context, tag string) error {
			return received.from.Modify(ctx, tag, change)
		})
}

func settling[R, A any](
	received Received[A],
	doing string,
	settle func(context.Context, string) error,
) effect.Effect[R, Fault, effect.Unit] {
	return effect.Try(
		func(ctx context.Context, _ R) (effect.Unit, error) {
			return effect.Unit{}, settle(ctx, received.Delivery.Tag)
		},
		func(err error) Fault { return faulted(doing, received.Delivery.Subject, err) },
	).Named("settle")
}

// read decodes one delivery, keeping the refusal rather than raising it.
func read[A any](link Receiving, delivery Delivery, shape schema.Schema[A]) Received[A] {
	value, err := schema.DecodeJSON(shape, delivery.Body)
	if err != nil {
		return Received[A]{
			Delivery: delivery,
			from:     link,
			refusal:  faulted("decoding a message", delivery.Subject, err),
		}
	}
	return Received[A]{Delivery: delivery, from: link, value: value}
}
