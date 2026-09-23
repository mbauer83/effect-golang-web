package amqp10

// The go-amqp adapter's lifetime.
//
// A connection, a session and a link, each a scoped resource, because that is
// what the protocol has: a connection multiplexes sessions, a session
// multiplexes links, and a link is attached to one node in one direction. A
// program that sends and receives attaches two links; it does not need two
// connections.

import (
	"context"

	broker "github.com/Azure/go-amqp"

	"github.com/mbauer83/effect-golang/effect"
)

// Connection is a connection to a broker.
type Connection struct {
	connection *broker.Conn
}

// Session is a session on a connection, and is what links are attached from.
type Session struct {
	session *broker.Session
}

// Connect opens a connection and gives the scope the closing of it.
//
// The address is an AMQP URI: amqp://host:5672 or amqps://host:5671. Unlike
// 0-9-1's Dial, this one takes the interpretation's context, because the
// protocol's opening handshake is a round trip and a caller that cancelled
// should not wait for it.
func Connect[R any](scope effect.Scope, address string, options *broker.ConnOptions) effect.Effect[R, Fault, *Connection] {
	acquire := effect.Try(
		func(ctx context.Context, _ R) (*Connection, error) {
			connection, err := broker.Dial(ctx, address, options)
			if err != nil {
				return nil, err
			}
			return &Connection{connection: connection}, nil
		},
		func(err error) Fault { return faultOf("connecting", address, err) },
	).WithName("connect")

	return scope.AcquireRelease(acquire, disconnect[R])
}

// Open starts a session on the connection.
func Open[R any](scope effect.Scope, connection *Connection) effect.Effect[R, Fault, *Session] {
	acquire := effect.Try(
		func(ctx context.Context, _ R) (*Session, error) {
			session, err := connection.connection.NewSession(ctx, nil)
			if err != nil {
				return nil, err
			}
			return &Session{session: session}, nil
		},
		func(err error) Fault { return faultOf("opening a session", "", err) },
	).WithName("open-session")

	return scope.AcquireRelease(acquire, endSession[R])
}

// Sender attaches a link to a node that accepts messages.
func Sender[R any](
	scope effect.Scope,
	session *Session,
	address string,
) effect.Effect[R, Fault, SenderLink] {
	acquire := effect.Try(
		func(ctx context.Context, _ R) (SenderLink, error) {
			sender, err := session.session.NewSender(ctx, address, nil)
			if err != nil {
				return nil, err
			}
			return &senderLink{sender: sender, address: address}, nil
		},
		func(err error) Fault { return faultOf("attaching a sender", address, err) },
	).WithName("attach-sender")

	return scope.AcquireRelease(acquire, detachSender[R])
}

// Receiver attaches a link from a node that produces them.
//
// credit is how many unsettled messages the broker may have in flight to this
// link. It is not optional in 1.0 the way a prefetch is in 0-9-1: the protocol
// is credit-based, so a link with no credit receives nothing at all. A stream
// is pull-based above this, but the credit is what stops the broker filling
// memory below it.
func Receiver[R any](
	scope effect.Scope,
	session *Session,
	address string,
	credit int32,
) effect.Effect[R, Fault, ReceiverLink] {
	acquire := effect.Try(
		func(ctx context.Context, _ R) (ReceiverLink, error) {
			receiver, err := session.session.NewReceiver(ctx, address,
				&broker.ReceiverOptions{Credit: credit})
			if err != nil {
				return nil, err
			}
			return newReceiverLink(receiver, address), nil
		},
		func(err error) Fault { return faultOf("attaching a receiver", address, err) },
	).WithName("attach-receiver")

	return scope.AcquireRelease(acquire, detachReceiver[R])
}

func disconnect[R any](connection *Connection) effect.Effect[R, effect.Never, effect.Unit] {
	return effect.AddFinalizer[R](func(context.Context) error {
		return ignoreClosed(connection.connection.Close())
	})
}

func endSession[R any](session *Session) effect.Effect[R, effect.Never, effect.Unit] {
	return effect.AddFinalizer[R](func(ctx context.Context) error {
		return ignoreClosed(session.session.Close(ctx))
	})
}

func detachSender[R any](link SenderLink) effect.Effect[R, effect.Never, effect.Unit] {
	return effect.AddFinalizer[R](func(ctx context.Context) error {
		return ignoreClosed(link.Close(ctx))
	})
}

func detachReceiver[R any](link ReceiverLink) effect.Effect[R, effect.Never, effect.Unit] {
	return effect.AddFinalizer[R](func(ctx context.Context) error {
		return ignoreClosed(link.Close(ctx))
	})
}
