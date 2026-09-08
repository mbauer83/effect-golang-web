package acceptance

// A conversation's lifetime. The socket belongs to a scope, so a conversation
// cannot outlive the request that started it and nothing has to remember to
// hang up -- which is checked by using a socket after its scope has closed, and
// by watching a server let go of one when the exchange goes wrong.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mbauer83/effect-golang-web/websocket"
	"github.com/mbauer83/effect-golang/effect"
)

func TestAMessageTheSchemaRefusesEndsTheConversation(t *testing.T) {
	// A conversation is stateful, so a peer that said something this side
	// cannot read has said something about the whole exchange.
	address, _ := tallying(t)
	runtime, err := effect.NewRuntime()
	if err != nil {
		t.Fatal(err)
	}

	program := effect.Scoped(func(scope effect.Scope) talking[websocket.Message] {
		return websocket.Dial[effect.Unit](scope, address).
			FlatMap(func(socket websocket.Socket) talking[websocket.Message] {
				return websocket.SendText[effect.Unit](socket, `{"add":"lots"}`).
					FlatMap(func(effect.Unit) talking[websocket.Message] {
						return websocket.Receive[effect.Unit](socket)
					})
			})
	})

	// With a deadline, so that a socket the server left open fails for a
	// different reason from one it closed -- and the test can tell which.
	waiting, giveUp := context.WithTimeout(context.Background(), 2*time.Second)
	defer giveUp()

	exit := runtime.Run(waiting, effect.Unit{}, program)
	cause, failed := exit.Cause()
	if !failed {
		t.Fatal("expected the server to hang up rather than answer")
	}
	for _, fault := range cause.Failures() {
		if errors.Is(fault, context.DeadlineExceeded) {
			t.Fatal("the server left the socket open: the read waited for the deadline " +
				"rather than being refused")
		}
	}
}

func TestClosingTheScopeClosesTheSocket(t *testing.T) {
	address, _ := tallying(t)
	runtime, err := effect.NewRuntime()
	if err != nil {
		t.Fatal(err)
	}

	// The socket is acquired inside a scope and used after it closes, which is
	// the one thing a scoped resource promises will not work.
	var escaped websocket.Socket
	program := effect.Scoped(func(scope effect.Scope) talking[effect.Unit] {
		return websocket.Dial[effect.Unit](scope, address).
			Map(func(socket websocket.Socket) effect.Unit {
				escaped = socket
				return effect.Unit{}
			})
	})
	if _, succeeded := runtime.Run(context.Background(), effect.Unit{}, program).Value(); !succeeded {
		t.Fatal("expected the conversation to start")
	}

	after := runtime.Run(context.Background(), effect.Unit{},
		websocket.SendText[effect.Unit](escaped, `{"add":1}`))
	if _, succeeded := after.Value(); succeeded {
		t.Fatal("expected the socket to be closed with its scope")
	}
	if !strings.Contains(after.String(), "sending a message") {
		t.Fatalf("expected the stage named, got %s", after)
	}
}

func TestARawConversationCarriesWhateverItLikes(t *testing.T) {
	// Inbound and Send are the untyped pair, for a conversation that decodes
	// its own messages -- and for one that carries something a schema does not
	// describe, which a binary message usually is.
	address, _ := tallying(t)
	runtime, err := effect.NewRuntime()
	if err != nil {
		t.Fatal(err)
	}

	program := effect.Scoped(func(scope effect.Scope) talking[[]websocket.Message] {
		return websocket.Dial[effect.Unit](scope, address).
			FlatMap(func(socket websocket.Socket) talking[[]websocket.Message] {
				said := websocket.Send[effect.Unit](socket, websocket.Message{
					Kind: websocket.Text,
					Data: []byte(`{"add":7}`),
				})
				return said.FlatMap(func(effect.Unit) talking[[]websocket.Message] {
					// Reading the inbound side as messages rather than values,
					// and taking one so the stream ends without waiting for the
					// peer to finish.
					return effect.RunCollect(
						websocket.Inbound[effect.Unit](socket).TakeStream(1))
				})
			})
	})

	heard, succeeded := runtime.Run(context.Background(), effect.Unit{}, program).Value()
	if !succeeded || len(heard) != 1 {
		t.Fatalf("expected one message, got %#v", heard)
	}
	if heard[0].Kind != websocket.Text {
		t.Fatalf("expected a text message, got kind %v", heard[0].Kind)
	}
	if got := string(heard[0].Data); !strings.Contains(got, `"total":7`) {
		t.Fatalf("unexpected message: %s", got)
	}
}
