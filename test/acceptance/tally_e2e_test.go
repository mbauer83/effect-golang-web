package acceptance

// A websocket conversation over a real socket, by a real client.
//
// What matters is that the exchange is stateful -- each answer depends on
// everything said before it -- that the messages are described by the same kind
// of Schema an endpoint uses, and that closing the scope closes the socket.

import (
	"context"
	"net"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/mbauer83/effect-golang-web/examples/tally"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang-web/websocket"
	"github.com/mbauer83/effect-golang/effect"
)

type talking[A any] = effect.Effect[effect.Unit, websocket.Fault, A]

// reported collects what the boundary could not answer with, so a test can say
// that a conversation ended cleanly rather than only that its answers were
// right.
type reported struct {
	mutex  sync.Mutex
	faults []error
}

func (recorded *reported) record(_ context.Context, err error) {
	recorded.mutex.Lock()
	defer recorded.mutex.Unlock()
	recorded.faults = append(recorded.faults, err)
}

func (recorded *reported) all() []error {
	recorded.mutex.Lock()
	defer recorded.mutex.Unlock()
	return slices.Clone(recorded.faults)
}

// tallying starts the conversation on a port the operating system chooses and
// returns the address to dial, with whatever the boundary reported.
func tallying(t *testing.T) (string, *reported) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := effect.NewRuntime()
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := web.NewAdapter(runtime, effect.Unit{},
		func(fault websocket.Fault) web.Response {
			return web.Text(http.StatusBadGateway, fault.Error())
		})
	if err != nil {
		t.Fatal(err)
	}
	recorded := &reported{}
	boundary = boundary.WithReport(recorded.record)

	// The tally is a Ref, so making one is an effect and the program that
	// serves it owns it. Serving fails with web's fault; conversing fails with
	// the websocket's, in the interpretation the boundary runs -- two exchanges,
	// two failure channels, and neither has to know the other's.
	serve := effect.For[effect.Unit, web.Fault]()
	program := serve.WidenError(effect.NewRef[effect.Unit](int64(0))).
		FlatMap(func(running effect.Ref[int64]) effect.Effect[effect.Unit, web.Fault, effect.Unit] {
			surface, assembled := web.NewRoutes(tally.Route(boundary, running))
			if assembled != nil {
				return serve.Fail[effect.Unit](web.Fault{
					Doing: "assembling the surface", Err: assembled,
				})
			}
			return effect.Scoped(func(scope effect.Scope) effect.Effect[effect.Unit, web.Fault, effect.Unit] {
				return web.ServeWith[effect.Unit](scope,
					web.Settings{Listener: listener}, boundary.Handler(surface.Handler())).
					FlatMap(func(web.Server) effect.Effect[effect.Unit, web.Fault, effect.Unit] {
						return serve.WidenError(untilStopped())
					})
			})
		})

	serving, stop := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		runtime.Run(serving, effect.Unit{}, program)
	}()
	t.Cleanup(func() {
		stop()
		select {
		case <-stopped:
		case <-time.After(5 * time.Second):
			t.Error("the conversation's server did not stop")
		}
	})
	return "ws://" + listener.Addr().String() + "/tally", recorded
}

// settled waits for the other side of an exchange to notice something.
func settled(t *testing.T, within time.Duration) {
	t.Helper()
	time.Sleep(within)
}

// untilStopped keeps the server up until its context is cancelled.
func untilStopped() effect.Effect[effect.Unit, effect.Never, effect.Unit] {
	return effect.From(func(ctx context.Context, _ effect.Unit) effect.Exit[effect.Never, effect.Unit] {
		<-ctx.Done()
		return effect.ExitSuccess[effect.Never](effect.Unit{})
	})
}

// asking dials, says its piece, and lets the scope hang up.
func asking(t *testing.T, address string, changes ...int32) []tally.Total {
	t.Helper()
	runtime, err := effect.NewRuntime(effect.WithDebugTracking())
	if err != nil {
		t.Fatal(err)
	}
	program := effect.Scoped(func(scope effect.Scope) talking[[]tally.Total] {
		return websocket.Dial[effect.Unit](scope, address).
			FlatMap(func(socket websocket.Socket) talking[[]tally.Total] {
				return effect.ForEach(changes, func(add int32) talking[tally.Total] {
					return tally.Ask[effect.Unit](socket, add)
				})
			})
	})

	answers, succeeded := runtime.Run(context.Background(), effect.Unit{}, program).Value()
	if !succeeded {
		t.Fatal("expected the conversation to succeed")
	}
	if live := runtime.LiveWork(); !live.IsEmpty() {
		t.Fatalf("the conversation left work behind: %#v", live)
	}
	return answers
}

func TestAConversationsAnswersDependOnWhatCameBefore(t *testing.T) {
	address, recorded := tallying(t)

	answers := asking(t, address, 2, 3, 10)
	if len(answers) != 3 {
		t.Fatalf("expected three answers, got %d", len(answers))
	}
	for index, expected := range []int64{2, 5, 15} {
		if answers[index].Total != expected {
			t.Errorf("answer %d: expected %d, got %d", index, expected, answers[index].Total)
		}
	}

	// The client said goodbye when its scope closed, and a peer saying it is
	// finished is not a fault. Reporting it as one would fill an operator's log
	// with every conversation that ended normally.
	//
	// The wait is for the server to observe the goodbye: it arrives as soon as
	// the frame does, so anything reported would be reported within it.
	settled(t, 500*time.Millisecond)
	if faults := recorded.all(); len(faults) != 0 {
		t.Fatalf("a conversation that ended politely was reported: %v", faults)
	}
}

func TestASecondConversationSeesWhatTheFirstDid(t *testing.T) {
	// The tally is shared, which is what the Ref is for: two conversations at
	// once are two fibers reading and writing one cell.
	address, _ := tallying(t)

	first := asking(t, address, 4)
	second := asking(t, address, 6)
	if first[0].Total != 4 || second[0].Total != 10 {
		t.Fatalf("expected 4 then 10, got %d then %d", first[0].Total, second[0].Total)
	}
}
