package acceptance

// A server's own settings, as distinct from what it serves: the grace a
// shutdown allows, and the deadline that stops a connection from being held
// open by a client that never finishes its headers.

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

// servingWith starts a handler with explicit settings, so the settings
// themselves can be checked rather than assumed to be plumbed through.
func servingWith(
	t *testing.T,
	settings web.Settings,
	handler web.Handler[effect.Unit, web.Fault],
) (string, context.CancelFunc, <-chan effect.Exit[web.Fault, effect.Unit]) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	settings.Listener = listener
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
		return web.ServeWith[effect.Unit](scope, settings, boundary.Handler(handler)).
			FlatMap(web.Await[effect.Unit])
	})
	run, stop := context.WithCancel(context.Background())
	exits := make(chan effect.Exit[web.Fault, effect.Unit], 1)
	go func() { exits <- runtime.Run(run, effect.Unit{}, program) }()
	return listener.Addr().String(), stop, exits
}

func TestAGracePeriodThatRunsOutIsReportedRatherThanWaitedOutForever(t *testing.T) {
	// A request that never finishes must not keep a process alive indefinitely,
	// and abandoning one is not something to do quietly.
	held := &gate{entered: make(chan struct{}), released: make(chan struct{})}
	address, stop, exits := servingWith(t,
		web.Settings{
			Grace:             100 * time.Millisecond,
			ReadHeaderTimeout: 2 * time.Second,
			ReadTimeout:       5 * time.Second,
			WriteTimeout:      5 * time.Second,
			IdleTimeout:       5 * time.Second,
		},
		waiting(held))
	defer close(held.released)

	go func() {
		if response, err := http.Get("http://" + address + "/"); err == nil {
			_ = response.Body.Close()
		}
	}()
	<-held.entered
	stop()

	select {
	case exit := <-exits:
		// The program itself was interrupted, so the abandonment cannot arrive
		// as its failure: a forked fiber's outcome is observed by joining it,
		// and an interrupted program joins nothing. It arrives in the closing
		// cause, which is the one channel an interruption cannot take away.
		cause, failed := exit.Cause()
		if !failed || !cause.ContainsDefect() {
			t.Fatalf("expected the abandoned request reported in the cause, got %+v", exit)
		}
		if !strings.Contains(cause.String(), "waiting for in-flight requests") {
			t.Fatalf("expected the stage named, got %s", cause)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the grace period was not honoured")
	}
}

func TestAConnectionThatSendsNoHeadersIsNotHeldOpen(t *testing.T) {
	// net/http has no read-header deadline of its own, so a client that opens a
	// connection and stops can hold one forever. This checks the deadline is
	// plumbed through; the default is the same plumbing with a longer value.
	held := &gate{entered: make(chan struct{}), released: make(chan struct{})}
	close(held.released)
	address, stop, exits := servingWith(t,
		web.Settings{ReadHeaderTimeout: 150 * time.Millisecond}, waiting(held))
	defer func() { stop(); awaitStopped(t, exits) }()

	connection, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = connection.Close() }()

	if err := connection.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Read(make([]byte, 1)); err == nil {
		t.Fatal("expected the server to close a connection that sent no headers")
	} else if isTimeout(err) {
		t.Fatalf("the connection was still open after three seconds: %v", err)
	}
}

func isTimeout(err error) bool {
	var timeout interface{ Timeout() bool }
	return errors.As(err, &timeout) && timeout.Timeout()
}
