package amqp091

// Consuming a queue. The subscription is a scoped resource and the deliveries
// are a Stream, so a consumer reads at the rate it can work and the
// subscription ends when the consumer does -- including when it stopped early,
// failed, or was cancelled.

import (
	"context"

	"github.com/mbauer83/effect-golang-schema/schema"
	"github.com/mbauer83/effect-golang/effect"
)

// Consume subscribes to a queue and streams what arrives.
func Consume[R any](channel Consuming, queue string) effect.Stream[R, Fault, Delivery] {
	return effect.StreamFromResource(
		func(scope effect.Scope) effect.Effect[R, Fault, Deliveries] {
			return subscribing[R](scope, channel, queue)
		},
		func(from Deliveries) effect.Stream[R, Fault, Delivery] {
			return arriving[R](from, queue)
		},
	)
}

// Values subscribes to a queue and streams what arrives, decoded through the
// schema.
//
// A delivery the schema refuses does not fail the stream. That is the opposite
// of a websocket conversation, and deliberately: a conversation is stateful, so
// a peer that said something unreadable has said something about the whole
// exchange, but a queue is a sequence of separate messages and one that cannot
// be read is one message. It arrives as a Received whose Read refuses, so the
// consumer decides what to do with it -- which is the same decision it makes
// about a message it understood and could not act on.
func Values[R, A any](
	channel Consuming,
	queue string,
	shape schema.Schema[A],
) effect.Stream[R, Fault, Received[A]] {
	return effect.StreamFromResource(
		func(scope effect.Scope) effect.Effect[R, Fault, Deliveries] {
			return subscribing[R](scope, channel, queue)
		},
		func(from Deliveries) effect.Stream[R, Fault, Received[A]] {
			return effect.MapStream(arriving[R](from, queue),
				func(delivery Delivery) Received[A] { return read(from, delivery, shape) })
		},
	)
}

// Received is one delivery, decoded, with the acknowledgement still to make.
type Received[A any] struct {
	// Delivery is what arrived: the headers, the routing key it came in on, and
	// whether the broker has offered it before.
	Delivery Delivery

	value   A
	refusal error
	from    Deliveries
}

// Read is the value the delivery carried, or why it could not be read.
//
// It is a method rather than a field because a zero value that looked valid
// would be a trap: a consumer that forgot to ask would act on a message that
// was never there.
func (received Received[A]) Read() (A, error) {
	return received.value, received.refusal
}

// Ack accepts a delivery, so the broker may forget it.
func Ack[R, A any](received Received[A]) effect.Effect[R, Fault, effect.Unit] {
	return answering[R](received, "accepting a delivery", received.from.Ack)
}

// Discard rejects a delivery without return. The broker drops it, or routes it
// wherever the queue's dead-letter configuration says -- which is where a
// message nobody can read belongs.
func Discard[R, A any](received Received[A]) effect.Effect[R, Fault, effect.Unit] {
	return answering[R](received, "discarding a delivery", received.from.Discard)
}

// Requeue rejects a delivery and asks for it back, for a consumer that cannot
// handle it now and expects to later.
//
// A message requeued by the only consumer of a queue comes straight back, so a
// consumer that requeues unconditionally has written a loop. That is the
// caller's decision to make, which is the whole reason acknowledgement is
// explicit.
func Requeue[R, A any](received Received[A]) effect.Effect[R, Fault, effect.Unit] {
	return answering[R](received, "requeueing a delivery", received.from.Requeue)
}

func answering[R, A any](
	received Received[A],
	doing string,
	answer func(uint64) error,
) effect.Effect[R, Fault, effect.Unit] {
	return effect.Try(
		func(context.Context, R) (effect.Unit, error) {
			return effect.Unit{}, answer(received.Delivery.Tag)
		},
		func(err error) Fault { return faulted(doing, received.Delivery.Key, err) },
	).Named("acknowledge")
}

// read decodes one delivery, keeping the refusal rather than raising it.
func read[A any](from Deliveries, delivery Delivery, shape schema.Schema[A]) Received[A] {
	value, err := schema.DecodeJSON(shape, delivery.Body)
	if err != nil {
		return Received[A]{
			Delivery: delivery,
			from:     from,
			refusal:  faulted("decoding a delivery", delivery.Key, err),
		}
	}
	return Received[A]{Delivery: delivery, from: from, value: value}
}

// subscribing starts the subscription and gives the scope the ending of it.
func subscribing[R any](
	scope effect.Scope,
	channel Consuming,
	queue string,
) effect.Effect[R, Fault, Deliveries] {
	acquire := effect.Try(
		func(ctx context.Context, _ R) (Deliveries, error) {
			return channel.Consume(ctx, queue)
		},
		func(err error) Fault { return faulted("consuming", queue, err) },
	).Named("consume")

	return scope.AcquireRelease(acquire, unsubscribing[R])
}

func unsubscribing[R any](from Deliveries) effect.Effect[R, effect.Never, effect.Unit] {
	return effect.Release[R](func(context.Context) error { return from.Close() })
}

// arriving pulls the subscription one delivery at a time.
func arriving[R any](from Deliveries, queue string) effect.Stream[R, Fault, Delivery] {
	return effect.StreamFromSteps(func() effect.Effect[R, Fault, effect.Step[Delivery]] {
		return effect.From(func(ctx context.Context, _ R) effect.Exit[Fault, effect.Step[Delivery]] {
			delivery, more, err := from.Next(ctx)
			if err != nil {
				return effect.ExitFailure[Fault, effect.Step[Delivery]](
					faulted("waiting for a delivery", queue, err))
			}
			if !more {
				return effect.ExitSuccess[Fault](effect.EndOfStream[Delivery]())
			}
			return effect.ExitSuccess[Fault](effect.Emit(effect.ChunkOf(delivery)))
		})
	})
}
