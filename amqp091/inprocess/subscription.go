package inprocess

// One subscription, and the settling of what it took.
//
// A delivery handed out is held until it is settled, so requeueing can put back
// the message that was actually taken rather than a copy of it. That is not
// bookkeeping for its own sake: a consumer that requeues and then reads again
// must see the same body, and a fake that made a new one would let a test pass
// that a broker would fail.

import (
	"context"

	"github.com/mbauer83/effect-golang-web/amqp091"
)

type subscription struct {
	broker *Broker
	queue  *queue
}

// Next waits for the next delivery, or for the consumer to be cancelled.
//
// It never reports the end: an open subscription on a broker that is still
// running has no end, and a consumer decides when it has read enough. A test
// reads a known number with TakeStream, or runs the consumer in a scope it
// closes.
func (subscription *subscription) Next(ctx context.Context) (amqp091.Delivery, bool, error) {
	select {
	case delivery := <-subscription.queue.backlog:
		subscription.broker.mutex.Lock()
		defer subscription.broker.mutex.Unlock()
		subscription.queue.unsettled[delivery.Tag] = delivery
		return delivery, true, nil
	case <-ctx.Done():
		return amqp091.Delivery{}, false, ctx.Err()
	}
}

// Ack accepts a delivery, so the broker may forget it.
func (subscription *subscription) Ack(tag uint64) error {
	return subscription.settle(tag, func(outcomes *settlements, _ amqp091.Delivery) {
		outcomes.acks = append(outcomes.acks, tag)
	})
}

// Discard rejects a delivery without return. There is no dead-letter exchange
// here, so the message is gone -- which is what a queue with no dead-letter
// configuration does.
func (subscription *subscription) Discard(tag uint64) error {
	return subscription.settle(tag, func(outcomes *settlements, _ amqp091.Delivery) {
		outcomes.discards = append(outcomes.discards, tag)
	})
}

// Requeue rejects a delivery and puts it back, at the end.
//
// A real broker puts it back where it can, which for a single consumer means
// straight back at the front. At the end is the honest simplification: a test
// that depended on the position would be depending on something the protocol
// does not promise.
func (subscription *subscription) Requeue(tag uint64) error {
	var again amqp091.Delivery
	err := subscription.settle(tag, func(outcomes *settlements, delivery amqp091.Delivery) {
		outcomes.requeues = append(outcomes.requeues, tag)
		delivery.Redelivered = true
		again = delivery
	})
	if err != nil {
		return err
	}
	// Outside the lock, for the same reason a publish is.
	return subscription.queue.offer(again)
}

// Close ends the subscription. What it does not do is give back the deliveries
// this consumer took and never settled -- a real broker does, when the channel
// goes, and a test that needs to see that should say so rather than have it
// happen on the way out.
func (subscription *subscription) Close() error {
	return nil
}

func (subscription *subscription) settle(
	tag uint64,
	decide func(*settlements, amqp091.Delivery),
) error {
	subscription.broker.mutex.Lock()
	defer subscription.broker.mutex.Unlock()

	delivery, unsettled := subscription.queue.unsettled[tag]
	if !unsettled {
		return errUnknownTag
	}
	delete(subscription.queue.unsettled, tag)
	decide(&subscription.broker.outcomes, delivery)
	return nil
}
