package websocket

// Dialling a conversation. The client half exists because a conversation is
// symmetrical once it has begun, and because a library that only accepted would
// leave every test of it reaching for something else.

import (
	"context"

	ws "github.com/coder/websocket"

	"github.com/mbauer83/effect-golang/effect"
)

// Dial opens a conversation with a server, owned by the scope.
//
// Closing the scope closes the socket, so a client cannot leak a connection by
// forgetting to hang up -- which is the same rule the server side follows, for
// the same reason.
func Dial[R any](scope effect.Scope, address string, subprotocols ...string) effect.Effect[R, Fault, Socket] {
	acquire := effect.Try(
		func(ctx context.Context, _ R) (Socket, error) {
			connection, response, err := ws.Dial(ctx, address, &ws.DialOptions{
				Subprotocols: subprotocols,
			})
			if response != nil && response.Body != nil {
				// The handshake response's body is of no use to a caller and
				// would otherwise hold the connection it came on.
				_ = response.Body.Close()
			}
			if err != nil {
				return Socket{}, err
			}
			return Socket{connection: connection}, nil
		},
		func(err error) Fault { return faulted("dialling "+address, err) },
	).Named("dial")

	return scope.AcquireRelease(acquire, closing[R])
}
