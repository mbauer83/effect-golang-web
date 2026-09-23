package amqp091

// One subscription behind the port.
//
// amqp091-go pushes deliveries onto a Go channel; the port is pulled. Next is
// where the two meet, and it is a select rather than a receive because a
// consumer whose context was cancelled must stop waiting: the broker has no
// reason to send anything, and a receive with nothing to receive would hold the
// fiber until the connection went.

import (
	"context"

	broker "github.com/rabbitmq/amqp091-go"
)

// subscription is one consumer on a channel.
type subscription struct {
	channel    *broker.Channel
	deliveries <-chan broker.Delivery
	tag        string
}

// Next waits for the next delivery.
//
// A closed channel is the subscription ending: the broker cancelled the
// consumer, or the channel went. Either way there is nothing more to read, and
// a stream that ended is the honest report -- the reason it ended, if the
// channel went, is on the connection rather than here.
func (subscription *subscription) Next(ctx context.Context) (Delivery, bool, error) {
	select {
	case raw, more := <-subscription.deliveries:
		if !more {
			return Delivery{}, false, nil
		}
		delivery, err := deliveryOf(raw)
		if err != nil {
			return Delivery{}, false, err
		}
		return delivery, true, nil
	case <-ctx.Done():
		return Delivery{}, false, ctx.Err()
	}
}

// Ack accepts one delivery. Never several: multiple acknowledgement accepts
// every delivery up to a tag, which is a different operation with a different
// failure mode, and expressing it as a flag on this one would hide that.
func (subscription *subscription) Ack(tag uint64) error {
	return subscription.channel.Ack(tag, false)
}

// Discard rejects one delivery without return.
func (subscription *subscription) Discard(tag uint64) error {
	return subscription.channel.Reject(tag, false)
}

// Requeue rejects one delivery and asks for it back.
func (subscription *subscription) Requeue(tag uint64) error {
	return subscription.channel.Reject(tag, true)
}

// Close cancels the consumer, and does not close the channel.
//
// Cancelling is the whole job. A stream that stopped early -- read three of a
// million, failed, was interrupted -- leaves a broker that has no idea and goes
// on sending, into a Go channel nobody reads; with a prefetch set those
// deliveries are unacknowledged and the queue stalls behind them. Closing the
// channel instead would be worse the other way: the program may still be
// publishing on it, and the channel is a scoped resource with a lifetime of its
// own.
//
// A channel the broker has already closed has no consumer left to cancel, which
// is the outcome this wanted rather than a fault to report.
func (subscription *subscription) Close() error {
	return ignoreClosed(subscription.channel.Cancel(subscription.tag, false))
}

// deliveryOf is what arrived, in the universal representation.
func deliveryOf(raw broker.Delivery) (Delivery, error) {
	headers, err := Headers(raw.Headers)
	if err != nil {
		return Delivery{}, err
	}
	return Delivery{
		Body:        raw.Body,
		ContentType: raw.ContentType,
		Headers:     headers,
		Exchange:    raw.Exchange,
		Key:         raw.RoutingKey,
		Tag:         raw.DeliveryTag,
		Redelivered: raw.Redelivered,
	}, nil
}
