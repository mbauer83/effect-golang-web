package amqp10

// The two links behind the port.
//
// A receiver keeps the messages it has handed out until they are settled,
// because go-amqp's dispositions take the message and the port takes a tag: the
// port cannot carry the library's own type without everything above it
// acquiring the dependency, and a tag is what the protocol itself uses to name
// a delivery.

import (
	"context"
	"errors"
	"sync"

	broker "github.com/Azure/go-amqp"
)

// sending is a sender link.
type sending struct {
	sender  *broker.Sender
	address string
}

func (link *sending) Address() string { return link.address }

func (link *sending) Send(ctx context.Context, message Message) error {
	sent, err := transferred(message)
	if err != nil {
		return err
	}
	return link.sender.Send(ctx, sent, nil)
}

func (link *sending) Close(ctx context.Context) error {
	return link.sender.Close(ctx)
}

// receiving is a receiver link and the deliveries it has not settled.
type receiving struct {
	receiver *broker.Receiver
	address  string

	mutex     sync.Mutex
	unsettled map[string]*broker.Message
}

func newReceiving(receiver *broker.Receiver, address string) Receiving {
	return &receiving{
		receiver:  receiver,
		address:   address,
		unsettled: map[string]*broker.Message{},
	}
}

func (link *receiving) Address() string { return link.address }

// Receive waits for the next message, and remembers it until it is settled.
//
// A detached link is the end of the messages rather than a failure: a broker or
// an operator closing a link has said there will be no more, and a consumer
// told the stream failed would retry against a link that is gone.
func (link *receiving) Receive(ctx context.Context) (Delivery, bool, error) {
	received, err := link.receiver.Receive(ctx, nil)
	if err != nil {
		var detached *broker.LinkError
		if errors.As(err, &detached) {
			return Delivery{}, false, nil
		}
		return Delivery{}, false, err
	}
	delivery, err := delivered(received)
	if err != nil {
		return Delivery{}, false, err
	}

	link.mutex.Lock()
	defer link.mutex.Unlock()
	link.unsettled[delivery.Tag] = received
	return delivery, true, nil
}

func (link *receiving) Accept(ctx context.Context, tag string) error {
	return link.settle(ctx, tag, func(ctx context.Context, message *broker.Message) error {
		return link.receiver.AcceptMessage(ctx, message)
	})
}

func (link *receiving) Reject(ctx context.Context, tag string, reason string) error {
	return link.settle(ctx, tag, func(ctx context.Context, message *broker.Message) error {
		return link.receiver.RejectMessage(ctx, message,
			&broker.Error{Condition: "amqp:precondition-failed", Description: reason})
	})
}

func (link *receiving) Release(ctx context.Context, tag string) error {
	return link.settle(ctx, tag, func(ctx context.Context, message *broker.Message) error {
		return link.receiver.ReleaseMessage(ctx, message)
	})
}

func (link *receiving) Modify(ctx context.Context, tag string, change Change) error {
	annotations, err := Annotations(change.Annotations)
	if err != nil {
		return err
	}
	return link.settle(ctx, tag, func(ctx context.Context, message *broker.Message) error {
		return link.receiver.ModifyMessage(ctx, message, &broker.ModifyMessageOptions{
			DeliveryFailed:    change.Tried,
			UndeliverableHere: change.Elsewhere,
			Annotations:       annotations,
		})
	})
}

func (link *receiving) Close(ctx context.Context) error {
	return link.receiver.Close(ctx)
}

// settle finds the delivery, settles it, and forgets it.
//
// Forgotten first, so a disposition the broker refused does not leave the
// message here for a second attempt: the protocol settles a delivery once, and
// a retry would be naming a delivery the broker has already finished with.
func (link *receiving) settle(
	ctx context.Context,
	tag string,
	disposition func(context.Context, *broker.Message) error,
) error {
	link.mutex.Lock()
	message, held := link.unsettled[tag]
	delete(link.unsettled, tag)
	link.mutex.Unlock()

	if !held {
		return errUnknownTag
	}
	return disposition(ctx, message)
}

// closedAlready treats an already-closed connection, session or link as the
// outcome the release wanted.
//
// The broker closes any of them of its own accord -- an idle timeout, a
// shutdown -- and by the time the scope ends there is nothing left to close. A
// release that reported that would report a fault for every program the broker
// let go of first.
func closedAlready(err error) error {
	if err == nil {
		return nil
	}
	var connection *broker.ConnError
	var session *broker.SessionError
	var link *broker.LinkError
	if errors.As(err, &connection) || errors.As(err, &session) || errors.As(err, &link) {
		return nil
	}
	return err
}
