package websocket

// The inbound side of a conversation, as a stream.
//
// A stream is what an inbound side is: pulled one message at a time, ended by
// the peer rather than by the reader, and finished when the scope that owns the
// socket closes. Nothing here buffers, and nothing needs a goroutine.

import (
	"context"
	"errors"

	ws "github.com/coder/websocket"

	"github.com/mbauer83/effect-golang/effect"
)

// Inbound is every message the peer sends, until it closes or the scope does.
//
// A polite close ends the stream rather than failing it, because a peer saying
// it is finished is not a failure. Anything else is.
func Inbound[R any](socket Socket) effect.Stream[R, Fault, Message] {
	return effect.StreamFromSteps(func() effect.Effect[R, Fault, effect.Step[Message]] {
		return effect.From(func(ctx context.Context, _ R) effect.Exit[Fault, effect.Step[Message]] {
			kind, data, err := socket.connection.Read(ctx)
			if err != nil {
				return received(err)
			}
			return effect.ExitSuccess[Fault](effect.Emit(effect.ChunkOf(
				Message{Kind: messageKind(kind), Data: data},
			)))
		})
	})
}

// received decides what the end of a read means.
func received(err error) effect.Exit[Fault, effect.Step[Message]] {
	if ended(err) {
		return effect.ExitSuccess[Fault](effect.EndOfStream[Message]())
	}
	return effect.ExitFailure[Fault, effect.Step[Message]](faulted("reading a message", err))
}

// ended reports the ways a conversation finishes rather than breaks.
//
// A cancelled context is one of them: the scope that owns the socket is
// closing, so the stream is over and reporting a failure would be reporting
// the shutdown as a fault.
func ended(err error) bool {
	switch ws.CloseStatus(err) {
	case ws.StatusNormalClosure, ws.StatusGoingAway:
		return true
	}
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
