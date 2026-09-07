package bookstore

import (
	"net"

	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

// Serve runs the bookstore until the caller's context is cancelled.
//
// The server's lifetime is the scope's, so cancellation stops the listener and
// lets the requests already in flight finish. Nothing here says "shutdown":
// that is what closing a scope already means, which is the point of the server
// being a scoped resource rather than something with a Close to remember.
func Serve(
	listener net.Listener,
	boundary web.Adapter[effect.Unit, Fault],
	store *Store,
) effect.Effect[effect.Unit, web.Fault, effect.Unit] {
	return effect.Scoped(func(scope effect.Scope) effect.Effect[effect.Unit, web.Fault, effect.Unit] {
		settings := web.Settings{Listener: listener}
		return web.ServeWith[effect.Unit](scope, settings, boundary.Handler(Handler(store))).
			FlatMap(web.Await[effect.Unit]).
			Named("bookstore")
	})
}

// Boundary builds the boundary that interprets the bookstore's handlers.
//
// It lives here rather than in the handler because the runtime, the environment
// and the mapping from a failure to a status are a deployment's decisions, not a
// handler's.
func Boundary(runtime *effect.Runtime) (web.Adapter[effect.Unit, Fault], error) {
	return web.NewAdapter(runtime, effect.Unit{}, StatusFor)
}
