package websocket

// Typed messages: the same Schema that describes a request body describes a
// message, so a conversation and an endpoint agree about a shape without
// anyone writing it twice.

import (
	"context"

	"github.com/mbauer83/effect-golang-schema/schema"
	"github.com/mbauer83/effect-golang/effect"
)

// SendValue encodes a value through its schema and sends it as one text
// message.
//
// The value is encoded before anything is written, so a value the schema
// refuses is reported rather than sent as a half-written message the peer would
// have to make sense of.
func SendValue[R, A any](socket Socket, shape schema.Schema[A], value A) effect.Effect[R, Fault, effect.Unit] {
	return effect.For[R, Fault]().
		Suspend(func() effect.Effect[R, Fault, effect.Unit] {
			document, err := schema.EncodeJSON(shape, value)
			if err != nil {
				return effect.For[R, Fault]().
					Fail[effect.Unit](faulted("encoding a message", err))
			}
			return Send[R](socket, Message{Kind: Text, Data: document})
		}).
		Named("send-value")
}

// ReceiveValue reads the next message, decoded through its schema.
//
// It is the counterpart of SendValue, for the exchange that is one question and
// one answer.
func ReceiveValue[R, A any](socket Socket, shape schema.Schema[A]) effect.Effect[R, Fault, A] {
	return Receive[R](socket).
		FlatMap(func(message Message) effect.Effect[R, Fault, A] {
			return effect.Try(
				func(context.Context, R) (A, error) {
					return schema.DecodeJSON(shape, message.Data)
				},
				func(err error) Fault { return faulted("reading a message", err) },
			)
		}).
		Named("receive-value")
}

// Values is the inbound side decoded through a schema.
//
// A message the schema refuses fails the stream. That is deliberate: a
// conversation is stateful, and a peer that sent one message this side cannot
// read has said something about the whole exchange rather than about one
// message. A conversation that would rather skip it can read Inbound and decode
// each message itself.
func Values[R, A any](socket Socket, shape schema.Schema[A]) effect.Stream[R, Fault, A] {
	return effect.MapStreamEffect(Inbound[R](socket),
		func(message Message) effect.Effect[R, Fault, A] {
			return effect.Try(
				func(context.Context, R) (A, error) {
					return schema.DecodeJSON(shape, message.Data)
				},
				func(err error) Fault { return faulted("reading a message", err) },
			)
		})
}
