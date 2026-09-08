package websocket

import (
	"context"

	ws "github.com/coder/websocket"

	"github.com/mbauer83/effect-golang/effect"
)

// Kind is what a message carries.
type Kind uint8

const (
	// Text is a UTF-8 message, which is what a JSON conversation uses.
	Text Kind = iota
	// Binary is an opaque message.
	Binary
)

// Message is one message, whole.
//
// The protocol frames a message and may split it; this is the message, because
// a frame is not a unit anything above the transport cares about.
type Message struct {
	Kind Kind
	Data []byte
}

// Socket is one open conversation.
//
// It is the same type on both sides. A conversation is symmetrical once it has
// begun, and the only asymmetry -- who accepted and who dialled -- is over by
// the time there is a socket at all.
type Socket struct {
	connection *ws.Conn
}

// Send writes one message.
//
// The context the runtime is interpreting under is the message's deadline, so a
// cancelled conversation stops sending rather than blocking on a peer that has
// stopped reading.
func Send[R any](socket Socket, message Message) effect.Effect[R, Fault, effect.Unit] {
	return effect.Try(
		func(ctx context.Context, _ R) (effect.Unit, error) {
			return effect.Unit{}, socket.connection.Write(ctx, messageType(message.Kind), message.Data)
		},
		func(err error) Fault { return faulted("sending a message", err) },
	).Named("send")
}

// Receive reads the next message, whole.
//
// It is the counterpart of Send, for an exchange that is a request and a reply
// rather than a stream: a client that asks one thing and waits for one answer
// does not want a stream, and Inbound is the wrong shape for it.
func Receive[R any](socket Socket) effect.Effect[R, Fault, Message] {
	return effect.Try(
		func(ctx context.Context, _ R) (Message, error) {
			kind, data, err := socket.connection.Read(ctx)
			if err != nil {
				return Message{}, err
			}
			return Message{Kind: messageKind(kind), Data: data}, nil
		},
		func(err error) Fault { return faulted("reading a message", err) },
	).Named("receive")
}

// SendText writes one text message, which is the common case.
func SendText[R any](socket Socket, text string) effect.Effect[R, Fault, effect.Unit] {
	return Send[R](socket, Message{Kind: Text, Data: []byte(text)})
}

// Close ends the conversation politely, saying why.
//
// A conversation inside a scope does not need this: closing the scope closes
// the socket. It is here for the case where one side is finished and the other
// is not, which the protocol has a code for.
func Close[R any](socket Socket, reason string) effect.Effect[R, Fault, effect.Unit] {
	return effect.Try(
		func(context.Context, R) (effect.Unit, error) {
			return effect.Unit{}, socket.connection.Close(ws.StatusNormalClosure, reason)
		},
		func(err error) Fault { return faulted("closing the connection", err) },
	).Named("close")
}

// closing releases the socket when its scope ends.
//
// It says goodbye first. A peer that is told the conversation is over ends its
// own inbound stream and reports nothing, whereas a peer whose connection
// simply vanishes has to treat that as the failure it usually is -- so a scope
// that closed abruptly would make every normal departure look like a dropped
// connection, and fill an operator's log with them.
//
// If the polite close cannot be sent, the connection goes anyway: a release
// that failed to let go would be worse than one that was rude.
func closing[R any](socket Socket) effect.Effect[R, effect.Never, effect.Unit] {
	return effect.Release[R](func(context.Context) error {
		if err := socket.connection.Close(ws.StatusNormalClosure, ""); err == nil {
			return nil
		}
		// The polite close did not go: the peer said goodbye first, or the
		// write would have blocked. Either way the connection has to go, and
		// once it has gone the release owes nothing -- so CloseNow's own
		// complaint that it was already closed is the outcome that was wanted
		// rather than a fault to report.
		_ = socket.connection.CloseNow()
		return nil
	})
}

func messageType(kind Kind) ws.MessageType {
	if kind == Binary {
		return ws.MessageBinary
	}
	return ws.MessageText
}

func messageKind(kind ws.MessageType) Kind {
	if kind == ws.MessageBinary {
		return Binary
	}
	return Text
}
