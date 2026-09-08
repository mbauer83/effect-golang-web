package amqp10

// Sending a message, and sending a value as one.

import (
	"context"

	"github.com/mbauer83/effect-golang-web/schema"
	"github.com/mbauer83/effect-golang/effect"
)

// Send transfers one message.
//
// Unlike a 0-9-1 publish, this waits for the broker to settle the transfer:
// 1.0's flow control is credit-based and the sender learns whether the message
// was accepted. So a success here means the broker has the message, not merely
// that it reached the socket.
func Send[R any](link Sending, message Message) effect.Effect[R, Fault, effect.Unit] {
	return effect.Try(
		func(ctx context.Context, _ R) (effect.Unit, error) {
			return effect.Unit{}, link.Send(ctx, message)
		},
		func(err error) Fault { return faulted("sending", link.Address(), err) },
	).Named("send")
}

// SendValue encodes a value through its schema and sends it.
//
// The value is encoded before anything is sent, so a value the schema refuses
// is reported rather than transferred as a body the receiver would have to make
// sense of.
func SendValue[R, A any](
	link Sending,
	shape schema.Schema[A],
	value A,
) effect.Effect[R, Fault, effect.Unit] {
	return effect.Try(
		func(context.Context, R) (Message, error) { return Encoded(shape, value) },
		func(err error) Fault { return faulted("encoding a message", link.Address(), err) },
	).
		FlatMap(func(message Message) effect.Effect[R, Fault, effect.Unit] {
			return Send[R](link, message)
		}).
		Named("send-value")
}

// Encoded is the message a value makes.
//
// It is public because Message has more to say than a value does -- a subject,
// properties, a durability -- and a caller that wants to set those should not
// have to choose between them and the schema.
func Encoded[A any](shape schema.Schema[A], value A) (Message, error) {
	document, err := schema.EncodeJSON(shape, value)
	if err != nil {
		return Message{}, err
	}
	return Message{Body: document, ContentType: "application/json"}, nil
}
