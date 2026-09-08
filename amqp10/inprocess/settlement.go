package inprocess

// The four dispositions, recorded so a test can ask which was made.
//
// Released and Modified put the message back, because that is what they mean:
// another receiver may take it. Accepted and Rejected do not -- the difference
// being that a real broker dead-letters a rejection according to the node's
// configuration, and there is no configuration here.

import (
	"context"

	"github.com/mbauer83/effect-golang-web/amqp10"
)

// Accept says the message is done.
func (link *receiver) Accept(_ context.Context, tag string) error {
	return link.settle(tag, func(settled *settlements, _ amqp10.Delivery) {
		settled.accepted = append(settled.accepted, tag)
	})
}

// Reject says the message will never be processed, and keeps the reason.
func (link *receiver) Reject(_ context.Context, tag string, reason string) error {
	return link.settle(tag, func(settled *settlements, _ amqp10.Delivery) {
		settled.rejected = append(settled.rejected, Rejection{Tag: tag, Reason: reason})
	})
}

// Release gives the message back unchanged.
func (link *receiver) Release(_ context.Context, tag string) error {
	return link.settle(tag, func(settled *settlements, delivery amqp10.Delivery) {
		settled.released = append(settled.released, tag)
		link.returned(delivery)
	})
}

// Modify gives the message back with something said about it, and records what.
func (link *receiver) Modify(_ context.Context, tag string, change amqp10.Change) error {
	return link.settle(tag, func(settled *settlements, delivery amqp10.Delivery) {
		settled.modified = append(settled.modified, Modification{Tag: tag, Change: change})
		if change.Elsewhere {
			// Undeliverable here: another receiver may have it, and there is
			// only one node, so it goes back and this link will see it again.
			// A real broker would route it away from this receiver; a fake
			// that pretended to would be pretending to be a broker.
			link.returned(delivery)
			return
		}
		// Attempted and failed: the broker's count of deliveries moves, which
		// is the whole difference from a release.
		delivery.Attempts++
		link.returned(delivery)
	})
}

// returned puts a delivery back at the node, at the end.
//
// A real broker puts it back where it can, which for a single receiver means
// straight back at the front. At the end is the honest simplification: a test
// depending on the position would be depending on something the protocol does
// not promise.
//
// The broker's lock is already held by settle.
func (link *receiver) returned(delivery amqp10.Delivery) {
	node, known := link.broker.nodes[link.address]
	if !known {
		return
	}
	node.waiting = append(node.waiting, delivery)
}

func (link *receiver) settle(
	tag string,
	decide func(*settlements, amqp10.Delivery),
) error {
	link.mutex.Lock()
	delivery, unsettled := link.unsettled[tag]
	delete(link.unsettled, tag)
	link.mutex.Unlock()

	if !unsettled {
		return errUnknownTag
	}
	link.broker.mutex.Lock()
	defer link.broker.mutex.Unlock()
	decide(&link.broker.settled, delivery)
	return nil
}
