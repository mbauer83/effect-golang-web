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
func (from *subscription) Next(ctx context.Context) (amqp091.Delivery, bool, error) {
	select {
	case delivery := <-from.queue.waiting:
		from.broker.mutex.Lock()
		defer from.broker.mutex.Unlock()
		from.queue.held[delivery.Tag] = delivery
		return delivery, true, nil
	case <-ctx.Done():
		return amqp091.Delivery{}, false, ctx.Err()
	}
}

// Ack accepts a delivery, so the broker may forget it.
func (from *subscription) Ack(tag uint64) error {
	return from.settle(tag, func(settled *settlements, _ amqp091.Delivery) {
		settled.accepted = append(settled.accepted, tag)
	})
}

// Discard rejects a delivery without return. There is no dead-letter exchange
// here, so the message is gone -- which is what a queue with no dead-letter
// configuration does.
func (from *subscription) Discard(tag uint64) error {
	return from.settle(tag, func(settled *settlements, _ amqp091.Delivery) {
		settled.discarded = append(settled.discarded, tag)
	})
}

// Requeue rejects a delivery and puts it back, at the end.
//
// A real broker puts it back where it can, which for a single consumer means
// straight back at the front. At the end is the honest simplification: a test
// that depended on the position would be depending on something the protocol
// does not promise.
func (from *subscription) Requeue(tag uint64) error {
	var again amqp091.Delivery
	err := from.settle(tag, func(settled *settlements, delivery amqp091.Delivery) {
		settled.requeued = append(settled.requeued, tag)
		delivery.Redelivered = true
		again = delivery
	})
	if err != nil {
		return err
	}
	// Outside the lock, for the same reason a publish is.
	return from.queue.offer(again)
}

// Close ends the subscription. What it does not do is give back the deliveries
// this consumer took and never settled -- a real broker does, when the channel
// goes, and a test that needs to see that should say so rather than have it
// happen on the way out.
func (from *subscription) Close() error {
	return nil
}

func (from *subscription) settle(
	tag uint64,
	decide func(*settlements, amqp091.Delivery),
) error {
	from.broker.mutex.Lock()
	defer from.broker.mutex.Unlock()

	delivery, unsettled := from.queue.held[tag]
	if !unsettled {
		return errUnknownTag
	}
	delete(from.queue.held, tag)
	decide(&from.broker.settled, delivery)
	return nil
}
