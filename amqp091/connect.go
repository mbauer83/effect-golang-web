package amqp091

// The amqp091-go adapter's lifetime.
//
// A connection is a TCP connection and a channel is a multiplexed session on
// it, so a program opens one connection and a channel per concurrent user of
// it -- a channel is not safe to share across goroutines that publish and
// consume at once, which is the broker's rule rather than this library's. Both
// are scoped resources, so neither outlives the effect that asked for it.

import (
	"context"

	broker "github.com/rabbitmq/amqp091-go"

	"github.com/mbauer83/effect-golang/effect"
)

// Connection is a connection to a broker.
type Connection struct {
	connection *broker.Connection
}

// Channel is a session on a connection, and is what the port is satisfied by.
type Channel struct {
	channel *broker.Channel
}

// Connect opens a connection and gives the scope the closing of it.
//
// The address is an AMQP URI: amqp://user:password@host:5672/vhost, or amqps
// for TLS.
func Connect[R any](scope effect.Scope, address string) effect.Effect[R, Fault, *Connection] {
	acquire := effect.Try(
		func(_ context.Context, _ R) (*Connection, error) {
			connection, err := broker.Dial(address)
			if err != nil {
				return nil, err
			}
			return &Connection{connection: connection}, nil
		},
		func(err error) Fault { return faulted("connecting", "", err) },
	).Named("connect")

	return scope.AcquireRelease(acquire, disconnecting[R])
}

// Open takes a channel on the connection.
func Open[R any](scope effect.Scope, connection *Connection) effect.Effect[R, Fault, *Channel] {
	acquire := effect.Try(
		func(context.Context, R) (*Channel, error) {
			channel, err := connection.connection.Channel()
			if err != nil {
				return nil, err
			}
			return &Channel{channel: channel}, nil
		},
		func(err error) Fault { return faulted("opening a channel", "", err) },
	).Named("open-channel")

	return scope.AcquireRelease(acquire, closing[R])
}

// Prefetch limits how many unacknowledged deliveries the broker will send this
// channel at once.
//
// It is how a consumer keeps the broker from handing it the whole queue: with
// no limit the broker sends everything it has, and a consumer that reads one
// delivery at a time would still be holding all of them. A stream is pull-based
// above this, but the pushing happens below it.
func Prefetch[R any](channel *Channel, count int) effect.Effect[R, Fault, effect.Unit] {
	return effect.Try(
		func(context.Context, R) (effect.Unit, error) {
			return effect.Unit{}, channel.channel.Qos(count, 0, false)
		},
		func(err error) Fault { return faulted("setting the prefetch", "", err) },
	).Named("prefetch")
}

func disconnecting[R any](connection *Connection) effect.Effect[R, effect.Never, effect.Unit] {
	return effect.Release[R](func(context.Context) error {
		return closedAlready(connection.connection.Close())
	})
}

func closing[R any](channel *Channel) effect.Effect[R, effect.Never, effect.Unit] {
	return effect.Release[R](func(context.Context) error {
		return closedAlready(channel.channel.Close())
	})
}

// closedAlready treats a closed connection as the outcome the release wanted.
//
// The broker closes a channel or a connection of its own accord -- a refused
// declaration, a shutdown -- and by the time the scope ends there is nothing
// left to close. A release that reported that would report a fault for every
// program the broker disconnected first.
func closedAlready(err error) error {
	if err == nil || err == broker.ErrClosed {
		return nil
	}
	return err
}
