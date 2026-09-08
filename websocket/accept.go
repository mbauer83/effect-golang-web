package websocket

// Accepting a conversation: the upgrade, and the scope that owns what it made.

import (
	"net/http"

	ws "github.com/coder/websocket"

	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

// Settings are what an upgrade may insist on.
//
// The zero value accepts a conversation from the same origin only, which is
// what a browser enforces and what a server should not have to remember to
// ask for.
type Settings struct {
	// Origins are the hosts a browser conversation may come from. Empty means
	// the host serving the request, which is the safe answer; "*" means any,
	// which is a decision worth writing down rather than defaulting into.
	Origins []string
	// Subprotocols are the protocols this server speaks, most preferred first.
	Subprotocols []string
}

// Accept upgrades the request and hands the socket to converse.
//
// The conversation runs inside a scope that owns the socket, so it cannot
// outlive the request: when the scope closes -- because the conversation
// finished, failed, or the request was cancelled -- the socket closes with it.
//
// It takes the boundary because a conversation answers with no response. The
// exchange continues after the upgrade, so there is nothing for a Handler to
// return, and the runtime, the environment and the reporting a handler needs
// are the boundary's.
func Accept[R, E any](
	boundary web.Adapter[R, E],
	converse func(Socket) effect.Effect[R, E, effect.Unit],
	settings Settings,
) web.Handler[R, E] {
	return web.FromHTTP[R, E](http.HandlerFunc(
		func(writer http.ResponseWriter, request *http.Request) {
			connection, err := ws.Accept(writer, request, acceptOptions(settings))
			if err != nil {
				// Accept has already answered the request: it is an HTTP
				// exchange until the upgrade succeeds, and saying so twice
				// would be worse than saying it once.
				return
			}
			boundary.Interpret(request.Context(), conversing(Socket{connection: connection}, converse))
		}))
}

// conversing owns the socket for exactly as long as the conversation.
func conversing[R, E any](
	socket Socket,
	converse func(Socket) effect.Effect[R, E, effect.Unit],
) effect.Effect[R, E, effect.Unit] {
	return effect.Scoped(func(scope effect.Scope) effect.Effect[R, E, effect.Unit] {
		return scope.AcquireRelease(
			effect.For[R, E]().Succeed(socket),
			closing[R],
		).FlatMap(converse).Named("conversation")
	})
}

func acceptOptions(settings Settings) *ws.AcceptOptions {
	return &ws.AcceptOptions{
		OriginPatterns: settings.Origins,
		Subprotocols:   settings.Subprotocols,
	}
}
