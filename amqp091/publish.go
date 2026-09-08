package amqp091

// Publishing a message, and publishing a value as one.

import (
	"context"

	"github.com/mbauer83/effect-golang-schema/schema"
	"github.com/mbauer83/effect-golang/effect"
)

// Publish sends one message.
//
// The broker's acknowledgement of a publish is a separate protocol feature
// (publisher confirms) and is not this: what succeeds here is that the message
// reached the broker's socket. A program that needs the broker to say it has
// the message needs confirms, which is a thing to add when a caller asks.
func Publish[R any](
	channel Publishing,
	target Target,
	message Message,
) effect.Effect[R, Fault, effect.Unit] {
	return effect.Try(
		func(ctx context.Context, _ R) (effect.Unit, error) {
			return effect.Unit{}, channel.Publish(ctx, target, message)
		},
		func(err error) Fault { return faulted("publishing", target.Key, err) },
	).Named("publish")
}

// PublishValue encodes a value through its schema and sends it.
//
// The value is encoded before anything is sent, so a value the schema refuses
// is reported rather than published as a body the consumer would have to make
// sense of.
func PublishValue[R, A any](
	channel Publishing,
	target Target,
	shape schema.Schema[A],
	value A,
) effect.Effect[R, Fault, effect.Unit] {
	return effect.Try(
		func(context.Context, R) (Message, error) { return Encoded(shape, value) },
		func(err error) Fault { return faulted("encoding a message", target.Key, err) },
	).
		FlatMap(func(message Message) effect.Effect[R, Fault, effect.Unit] {
			return Publish[R](channel, target, message)
		}).
		Named("publish-value")
}

// Encoded is the message a value makes.
//
// It is public because Message has more to say than a value does -- headers, a
// durability -- and a caller that wants to set those should not have to choose
// between the schema and them.
func Encoded[A any](shape schema.Schema[A], value A) (Message, error) {
	document, err := schema.EncodeJSON(shape, value)
	if err != nil {
		return Message{}, err
	}
	return Message{Body: document, ContentType: "application/json"}, nil
}
