package web

// How a server actually runs: the socket it holds, the loop that accepts on it,
// and how a shutdown reports what it had to give up on.

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/mbauer83/effect-golang/effect"
)

// reporting registers the finalizer that surfaces a shutdown which gave up on
// requests still in flight.
//
// It has to be a finalizer rather than the serve fiber's own outcome. A forked
// fiber's failure is observed by joining it, and a program interrupted while
// waiting cannot join anything -- so the one outcome that must never be lost
// would be exactly the one that is. A scope composes its finalizers' faults
// into the closing cause whatever happened to the body, which is where this
// belongs.
func reporting[R any](scope effect.Scope) effect.Effect[R, Fault, chan error] {
	abandoned := make(chan error, 1)
	return scope.AcquireRelease(
		effect.For[R, Fault]().Succeed(abandoned),
		func(reported chan error) effect.Effect[R, effect.Never, effect.Unit] {
			return effect.Release[R](func(context.Context) error {
				// The scope has already awaited the serve fiber, so whatever it
				// had to say has been said by now.
				select {
				case err := <-reported:
					return err
				default:
					return nil
				}
			})
		},
	)
}

// listening opens the socket as a scoped resource, so a server that fails
// between binding and serving still gives the port back.
func listening[R any](scope effect.Scope, settings Settings) effect.Effect[R, Fault, net.Listener] {
	acquire := effect.Try(
		func(context.Context, R) (net.Listener, error) {
			if settings.Listener != nil {
				return settings.Listener, nil
			}
			if settings.Address == "" {
				return nil, errNoListener
			}
			return net.Listen("tcp", settings.Address)
		},
		func(err error) Fault { return Fault{Doing: "opening the listener", Err: err} },
	).Named("listen")

	return scope.AcquireRelease(acquire, closingListener[R])
}

// closingListener releases the socket. A shutdown has usually closed it
// already, which is not a failure: it is the ordinary path.
func closingListener[R any](listener net.Listener) effect.Effect[R, effect.Never, effect.Unit] {
	return effect.Release[R](func(context.Context) error {
		if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			return err
		}
		return nil
	})
}

// acceptLoop is the accept loop's own state: the server, the goroutine running
// it, how long a shutdown may wait, and where a shutdown that gave up reports.
// Keeping them together is what leaves the two functions below narrow.
type acceptLoop struct {
	server    *http.Server
	stopped   chan error
	grace     time.Duration
	abandoned chan<- error
}

// serving runs the accept loop until it stops or its context is cancelled.
//
// net/http's Serve blocks and does not take a context, so the goroutine here is
// the ordinary adapter for a blocking library call: its lifetime is bounded by
// this effect, and nothing outlives the fiber that owns it.
func serving[R any](
	server *http.Server,
	listener net.Listener,
	grace time.Duration,
	abandoned chan<- error,
) effect.Effect[R, Fault, effect.Unit] {
	return effect.From(func(ctx context.Context, _ R) effect.Exit[Fault, effect.Unit] {
		loop := acceptLoop{
			server:    server,
			stopped:   make(chan error, 1),
			grace:     grace,
			abandoned: abandoned,
		}
		go func() { loop.stopped <- server.Serve(listener) }()

		select {
		case err := <-loop.stopped:
			return served(err)
		case <-ctx.Done():
			loop.shutDown(ctx)
			// Being shut down is how a server is meant to end, so the loop
			// succeeded. What a shutdown had to give up on is reported by the
			// scope, not by this fiber.
			return effect.ExitSuccess[Fault](effect.Unit{})
		}
	}).Named("serve-loop")
}

// shutDown stops accepting, waits for the requests already in flight, and then
// waits for the accept loop itself to return. A grace period that runs out
// means requests were abandoned, which is not something to pass over quietly.
func (loop acceptLoop) shutDown(ctx context.Context) {
	// Cleanup keeps the context's values and drops its cancellation, because a
	// shutdown that began with a cancelled context must still be allowed to
	// finish.
	closing := context.WithoutCancel(ctx)
	if loop.grace > 0 {
		bounded, done := context.WithTimeout(closing, loop.grace)
		defer done()
		closing = bounded
	}

	if err := loop.server.Shutdown(closing); err != nil {
		loop.abandoned <- Fault{Doing: "waiting for in-flight requests", Err: err}
		return
	}
	<-loop.stopped
}

// served reports why the accept loop stopped. A closed server is the ordinary
// end of one, not a failure.
func served(err error) effect.Exit[Fault, effect.Unit] {
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		return effect.ExitSuccess[Fault](effect.Unit{})
	}
	return effect.ExitFailure[Fault, effect.Unit](Fault{Doing: "serving", Err: err})
}

func httpServer(settings Settings, handler http.Handler) *http.Server {
	return &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: orElse(settings.ReadHeaderTimeout, defaultReadHeaderTimeout),
		ReadTimeout:       settings.ReadTimeout,
		WriteTimeout:      settings.WriteTimeout,
		IdleTimeout:       settings.IdleTimeout,
	}
}

func orElse(chosen time.Duration, fallback time.Duration) time.Duration {
	if chosen > 0 {
		return chosen
	}
	return fallback
}

// defaultReadHeaderTimeout is set because net/http's own default is none, and a
// server with no header deadline can be held open by a client that never
// finishes sending them.
const defaultReadHeaderTimeout = 10 * time.Second
