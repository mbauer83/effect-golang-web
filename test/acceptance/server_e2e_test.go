package acceptance

// A server's lifetime is a scope. These check what that means in practice:
// closing the scope stops accepting and waits for the requests already in
// flight, and a server that cannot start says so instead of half-existing.

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

type gate struct {
	entered  chan struct{}
	released chan struct{}
}

// waiting is a handler that reports when it has been entered and then waits to
// be let go, which is how a request can be held in flight while the server is
// asked to stop.
func waiting(held *gate) web.Handler[effect.Unit, web.Fault] {
	return func(web.Request) effect.Effect[effect.Unit, web.Fault, web.Response] {
		return effect.From(func(context.Context, effect.Unit) effect.Exit[web.Fault, web.Response] {
			close(held.entered)
			<-held.released
			return effect.ExitSuccess[web.Fault](web.Text(http.StatusOK, "finished"))
		})
	}
}

// serving starts one handler on a port the operating system chooses, and
// returns its address together with the cancel that closes the scope.
func serving(
	t *testing.T,
	handler web.Handler[effect.Unit, web.Fault],
) (string, context.CancelFunc, <-chan effect.Exit[web.Fault, effect.Unit]) {
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
		func(fault web.Fault) web.Response { return web.Text(http.StatusBadGateway, fault.Error()) })
	if err != nil {
		t.Fatal(err)
	}

	program := effect.Scoped(func(scope effect.Scope) effect.Effect[effect.Unit, web.Fault, effect.Unit] {
		return web.ServeWith[effect.Unit](scope,
			web.Settings{Listener: listener}, boundary.Handler(handler)).
			FlatMap(web.Await[effect.Unit])
	})

	run, stop := context.WithCancel(context.Background())
	exits := make(chan effect.Exit[web.Fault, effect.Unit], 1)
	go func() { exits <- runtime.Run(run, effect.Unit{}, program) }()
	return "http://" + listener.Addr().String(), stop, exits
}

func TestClosingTheScopeLetsARequestInFlightFinish(t *testing.T) {
	held := &gate{entered: make(chan struct{}), released: make(chan struct{})}
	address, stop, exits := serving(t, waiting(held))

	answered := make(chan int, 1)
	go func() {
		response, err := http.Get(address + "/")
		if err != nil {
			answered <- 0
			return
		}
		defer func() { _ = response.Body.Close() }()
		answered <- response.StatusCode
	}()

	<-held.entered
	// The scope closes while the request is still being handled. Shutting down
	// must wait for it rather than cutting the connection, which is the whole
	// difference between stopping a server and shutting one down.
	stop()

	// Nothing may reach the client while the handler is still held. A server
	// that closed its connections instead of draining them would answer here,
	// with a transport error, and that is what this window detects.
	select {
	case status := <-answered:
		t.Fatalf("the request was answered while still in flight, with status %d", status)
	case <-time.After(250 * time.Millisecond):
	}
	close(held.released)

	select {
	case status := <-answered:
		if status != http.StatusOK {
			t.Fatalf("the in-flight request was cut off, got status %d", status)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the in-flight request never completed")
	}
	awaitStopped(t, exits)
}

func TestOnceTheScopeIsClosedNothingElseIsAccepted(t *testing.T) {
	held := &gate{entered: make(chan struct{}), released: make(chan struct{})}
	close(held.released)
	address, stop, exits := serving(t, waiting(held))

	// One request first, so the server is demonstrably up before it is stopped.
	if response, err := http.Get(address + "/"); err == nil {
		_ = response.Body.Close()
	} else {
		t.Fatal(err)
	}
	stop()
	awaitStopped(t, exits)

	if response, err := http.Get(address + "/"); err == nil {
		_ = response.Body.Close()
		t.Fatal("expected the listener to be closed")
	}
}

// awaitStopped waits for the program to finish and checks that the shutdown
// gave up on nothing. A shutdown that reported an abandoned request when there
// was none would mean it never really waited -- which is what happens if its
// own deadline is inherited from the cancellation that started it.
func awaitStopped(t *testing.T, exits <-chan effect.Exit[web.Fault, effect.Unit]) {
	t.Helper()
	select {
	case exit := <-exits:
		cause, failed := exit.Cause()
		if failed && cause.ContainsDefect() {
			t.Fatalf("the shutdown reported giving up when nothing was in flight: %s", cause)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the server did not stop within five seconds")
	}
}

func TestAServerWithNothingToServeOnIsRefused(t *testing.T) {
	runtime, err := effect.NewRuntime()
	if err != nil {
		t.Fatal(err)
	}
	program := effect.Scoped(func(scope effect.Scope) effect.Effect[effect.Unit, web.Fault, web.Server] {
		return web.ServeWith[effect.Unit](scope, web.Settings{}, http.NotFoundHandler())
	})

	cause, failed := runtime.Run(context.Background(), effect.Unit{}, program).Cause()
	if !failed {
		t.Fatal("expected a server with neither an address nor a listener to be refused")
	}
	failures := cause.Failures()
	if len(failures) != 1 || failures[0].Doing != "opening the listener" {
		t.Fatalf("expected the stage named, got %+v", cause)
	}
}

func TestAnAddressAlreadyInUseFailsRatherThanHanging(t *testing.T) {
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = taken.Close() }()

	runtime, err := effect.NewRuntime()
	if err != nil {
		t.Fatal(err)
	}
	program := effect.Scoped(func(scope effect.Scope) effect.Effect[effect.Unit, web.Fault, web.Server] {
		return web.Serve[effect.Unit](scope, taken.Addr().String(), http.NotFoundHandler())
	})

	cause, failed := runtime.Run(context.Background(), effect.Unit{}, program).Cause()
	if !failed {
		t.Fatal("expected a taken address to be refused")
	}
	if failures := cause.Failures(); len(failures) != 1 {
		t.Fatalf("expected one failure, got %+v", cause)
	}
}

func TestTheServerReportsWhereItIsActuallyListening(t *testing.T) {
	// A caller that asked for port zero has to be able to find out what it got.
	runtime, err := effect.NewRuntime()
	if err != nil {
		t.Fatal(err)
	}
	program := effect.Scoped(func(scope effect.Scope) effect.Effect[effect.Unit, web.Fault, string] {
		return web.Serve[effect.Unit](scope, "127.0.0.1:0", http.NotFoundHandler()).
			Map(func(server web.Server) string { return server.Address().String() })
	})

	address, succeeded := runtime.Run(context.Background(), effect.Unit{}, program).Value()
	if !succeeded {
		t.Fatal("expected the server to start")
	}
	if _, port, err := net.SplitHostPort(address); err != nil || port == "0" {
		t.Fatalf("expected a real port, got %q", address)
	}
}
