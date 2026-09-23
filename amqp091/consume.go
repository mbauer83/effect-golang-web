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

// Consume subscribes to a queue and streams what arrives, undecoded.
//
// The element is an Envelope[[]byte] and not a Delivery, because a delivery
// parted from its subscription cannot be acknowledged -- and a consumer that
// never acknowledges is one the broker stops sending to. With a prefetch of
// one it receives one message and then waits forever, which is a stall no
// amount of reading fixes. So the three acknowledgements Values gives are the
// three this gives, and Read is the body as it arrived: the identity decoding,
// which cannot refuse.
func Consume[R any](channel Consumer, queue string) effect.Stream[R, Fault, Envelope[[]byte]] {
	return effect.StreamFromResource(
		func(scope effect.Scope) effect.Effect[R, Fault, Deliveries] {
			return subscribe[R](scope, channel, queue)
		},
		func(deliveries Deliveries) effect.Stream[R, Fault, Envelope[[]byte]] {
			return effect.MapStream(streamDeliveries[R](deliveries, queue),
				func(delivery Delivery) Envelope[[]byte] {
					return Envelope[[]byte]{Delivery: delivery, deliveries: deliveries, value: delivery.Body}
				})
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
// be read is one message. It arrives as an Envelope whose Read refuses, so the
// consumer decides what to do with it -- which is the same decision it makes
// about a message it understood and could not act on.
func Values[R, A any](
	channel Consumer,
	queue string,
	shape schema.Schema[A],
) effect.Stream[R, Fault, Envelope[A]] {
	return effect.StreamFromResource(
		func(scope effect.Scope) effect.Effect[R, Fault, Deliveries] {
			return subscribe[R](scope, channel, queue)
		},
		func(deliveries Deliveries) effect.Stream[R, Fault, Envelope[A]] {
			return effect.MapStream(streamDeliveries[R](deliveries, queue),
				func(delivery Delivery) Envelope[A] { return decodeDelivery(deliveries, delivery, shape) })
		},
	)
}

// Envelope is one delivery, decoded, with the acknowledgement still to make.
type Envelope[A any] struct {
	// Delivery is what arrived: the headers, the routing key it came in on, and
	// whether the broker has offered it before.
	Delivery Delivery

	value      A
	refusal    error
	deliveries Deliveries
}

// Read is the value the delivery carried, or why it could not be read.
//
// It is a method rather than a field because a zero value that looked valid
// would be a trap: a consumer that forgot to ask would act on a message that
// was never there.
func (envelope Envelope[A]) Read() (A, error) {
	return envelope.value, envelope.refusal
}

// Ack accepts a delivery, so the broker may forget it.
func Ack[R, A any](envelope Envelope[A]) effect.Effect[R, Fault, effect.Unit] {
	return settle[R](envelope, "accept a delivery", envelope.deliveries.Ack)
}

// Discard rejects a delivery without return. The broker drops it, or routes it
// wherever the queue's dead-letter configuration says -- which is where a
// message nobody can read belongs.
func Discard[R, A any](envelope Envelope[A]) effect.Effect[R, Fault, effect.Unit] {
	return settle[R](envelope, "discard a delivery", envelope.deliveries.Discard)
}

// Requeue rejects a delivery and asks for it back, for a consumer that cannot
// handle it now and expects to later.
//
// A message requeued by the only consumer of a queue comes straight back, so a
// consumer that requeues unconditionally has written a loop. That is the
// caller's decision to make, which is the whole reason acknowledgement is
// explicit.
func Requeue[R, A any](envelope Envelope[A]) effect.Effect[R, Fault, effect.Unit] {
	return settle[R](envelope, "requeue a delivery", envelope.deliveries.Requeue)
}

func settle[R, A any](
	envelope Envelope[A],
	op string,
	answer func(uint64) error,
) effect.Effect[R, Fault, effect.Unit] {
	return effect.Try(
		func(context.Context, R) (effect.Unit, error) {
			return effect.Unit{}, answer(envelope.Delivery.Tag)
		},
		func(err error) Fault { return faultOf(op, envelope.Delivery.Key, err) },
	).WithName("acknowledge")
}

// decodeDelivery decodes one delivery, keeping the refusal rather than raising it.
func decodeDelivery[A any](deliveries Deliveries, delivery Delivery, shape schema.Schema[A]) Envelope[A] {
	value, err := schema.DecodeJSON(shape, delivery.Body)
	if err != nil {
		return Envelope[A]{
			Delivery:   delivery,
			deliveries: deliveries,
			refusal:    faultOf("decode a delivery", delivery.Key, err),
		}
	}
	return Envelope[A]{Delivery: delivery, deliveries: deliveries, value: value}
}

// subscribe starts the subscription and gives the scope the ending of it.
func subscribe[R any](
	scope effect.Scope,
	channel Consumer,
	queue string,
) effect.Effect[R, Fault, Deliveries] {
	acquire := effect.Try(
		func(ctx context.Context, _ R) (Deliveries, error) {
			return channel.Consume(ctx, queue)
		},
		func(err error) Fault { return faultOf("consume", queue, err) },
	).WithName("consume")

	return scope.AcquireRelease(acquire, unsubscribe[R])
}

func unsubscribe[R any](deliveries Deliveries) effect.Effect[R, effect.Never, effect.Unit] {
	return effect.AddFinalizer[R](func(context.Context) error { return deliveries.Close() })
}

// streamDeliveries pulls the subscription one delivery at a time.
func streamDeliveries[R any](deliveries Deliveries, queue string) effect.Stream[R, Fault, Delivery] {
	return effect.StreamFromSteps(func() effect.Effect[R, Fault, effect.Step[Delivery]] {
		return effect.From(func(ctx context.Context, _ R) effect.Exit[Fault, effect.Step[Delivery]] {
			delivery, more, err := deliveries.Next(ctx)
			if err != nil {
				return effect.ExitFailure[Fault, effect.Step[Delivery]](
					faultOf("wait for a delivery", queue, err))
			}
			if !more {
				return effect.ExitSuccess[Fault](effect.EndOfStream[Delivery]())
			}
			return effect.ExitSuccess[Fault](effect.Emit(effect.ChunkOf(delivery)))
		})
	})
}
