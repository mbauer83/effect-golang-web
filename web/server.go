package web

import (
	"net"
	"net/http"
	"time"

	"github.com/mbauer83/effect-golang/effect"
)

// Settings are a server's own parameters, as distinct from what it serves.
//
// The zero value is not usable: a server needs either an address or a listener.
// Everything else has a default, and the read-header timeout has a non-zero one
// deliberately -- net/http's own default is none, which leaves a server open to
// a client that opens a connection and never finishes its headers.
type Settings struct {
	// Address is where to listen, as net.Listen takes it.
	Address string
	// Listener is a socket the caller already has, for a server started from
	// systemd or bound to port zero by a test. It takes precedence over
	// Address.
	Listener net.Listener

	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration

	// Grace bounds how long a shutdown waits for the requests already in
	// flight. Zero waits as long as they take.
	Grace time.Duration
}

// Server is a running server's handle.
type Server struct {
	address net.Addr
	serving effect.Fiber[Fault, effect.Unit]
}

// Address is where the server is actually listening, which is what a caller
// that asked for port zero needs.
func (server Server) Address() net.Addr {
	return server.address
}

// Serve starts a server on address, owned by the scope.
func Serve[R any](scope effect.Scope, address string, handler http.Handler) effect.Effect[R, Fault, Server] {
	return ServeWith[R](scope, Settings{Address: address}, handler)
}

// ServeWith starts a server with explicit settings, owned by the scope.
//
// Closing the scope stops the listener and waits for the requests already in
// flight. Graceful shutdown is therefore not a separate mechanism to remember:
// it is what closing a scope already means, and a caller that forgets it cannot
// leak a listener because the scope holds it.
//
// The serve loop is a fiber the scope owns, so it is visible to the runtime's
// observer like any other work rather than being a goroutine nobody accounts
// for.
func ServeWith[R any](scope effect.Scope, settings Settings, handler http.Handler) effect.Effect[R, Fault, Server] {
	operations := effect.For[R, Fault]()
	return listening[R](scope, settings).
		FlatMap(func(listener net.Listener) effect.Effect[R, Fault, Server] {
			return reporting[R](scope).
				FlatMap(func(abandoned chan error) effect.Effect[R, Fault, Server] {
					loop := serving[R](httpServer(settings, handler), listener, settings.Grace, abandoned)
					return operations.ForkIn(scope, loop).
						Map(func(fiber effect.Fiber[Fault, effect.Unit]) Server {
							return Server{address: listener.Addr(), serving: fiber}
						})
				})
		}).
		Named("serve")
}

// Await completes when the server stops, and fails if it stopped for a reason
// other than being shut down. It is what a program that exists to serve waits
// on.
func Await[R any](server Server) effect.Effect[R, Fault, effect.Unit] {
	return server.serving.Join[R]()
}
