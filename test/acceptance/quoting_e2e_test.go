package acceptance

// The quoting service over a real socket, called by a real client.
//
// Nothing short of this establishes anything: the protocol is the point, so a
// test that called the handler directly would test the schema layer and skip
// everything the transport does. What runs here is an http.Server on a port the
// operating system chose, a Connect client over it, and the gRPC wire protocol
// in between.

import (
	"context"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mbauer83/effect-golang-web/examples/quoting"
	"github.com/mbauer83/effect-golang-web/grpc"
	"github.com/mbauer83/effect-golang/effect"
)

var routes = map[string]quoting.Rate{
	"Kiel-Hamburg":   {Carrier: "overland", Currency: "EUR", Cents: 4000},
	"Kiel-Rotterdam": {Carrier: "coastal", Currency: "EUR", Cents: 19000},
}

type calling[A any] = effect.Effect[effect.Unit, grpc.Failure, A]

// quoted starts the service on a socket, and hands back a client for it.
//
// The listener is closed by the test rather than by a scope, because what is
// being tested is the transport and not the lifetime -- and a server whose
// shutdown is part of the assertion is covered where the HTTP core's is.
func quoted(t *testing.T) *grpc.Dialled {
	t.Helper()
	runtime, err := effect.NewRuntime(effect.WithDebugTracking())
	if err != nil {
		t.Fatal(err)
	}
	transport := grpc.NewConnected()
	boundary, err := grpc.NewBoundary(runtime, effect.Unit{}, transport, quoting.Coded)
	if err != nil {
		t.Fatal(err)
	}
	if err := quoting.Answer(boundary, routes); err != nil {
		t.Fatal(err)
	}
	handler, err := boundary.Handler()
	if err != nil {
		t.Fatal(err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() {
		stopping, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		_ = server.Shutdown(stopping)
	})

	// Connect's own protocol, which runs over HTTP/1.1. gRPC proper needs
	// HTTP/2, and over plain TCP that means h2c -- a decision about the
	// deployment rather than about the RPC, and not what this test is about.
	return grpc.DialConnect(&http.Client{Timeout: 10 * time.Second},
		"http://"+listener.Addr().String())
}

// ran interprets a call and returns its exit.
func ranCall[A any](t *testing.T, work calling[A]) effect.Exit[grpc.Failure, A] {
	t.Helper()
	within, giveUp := context.WithTimeout(context.Background(), 10*time.Second)
	defer giveUp()
	return effect.Run(within, effect.Unit{}, work)
}

func TestAProcedureAnswersOverTheWire(t *testing.T) {
	client := quoted(t)
	exit := ranCall(t, grpc.Ask[effect.Unit](client, quoting.Quote,
		quoting.Enquiry{Origin: "Kiel", Destination: "Hamburg", Kilos: 100}))

	rate, ok := exit.Value()
	if !ok {
		t.Fatalf("unexpected exit: %+v", exit)
	}
	if rate.Carrier != "overland" || rate.Currency != "EUR" {
		t.Fatalf("unexpected rate: %#v", rate)
	}
	// Priced by weight, so the handler really ran rather than a default
	// coming back.
	if rate.Cents != 5000 {
		t.Fatalf("expected the weight priced, got %d", rate.Cents)
	}
}

func TestAnApplicationRefusalBecomesItsCodeAndNothingElse(t *testing.T) {
	// The three refusals, each mapping to the code its own function chose. The
	// handler never mentions a code, which is what makes the mapping readable
	// on its own -- and this is where it is checked to be the one that arrives.
	client := quoted(t)
	for named, expected := range map[string]struct {
		enquiry quoting.Enquiry
		code    grpc.Code
	}{
		"no route": {
			enquiry: quoting.Enquiry{Origin: "Kiel", Destination: "Lima", Kilos: 10},
			code:    grpc.NotFound,
		},
		"too heavy": {
			enquiry: quoting.Enquiry{Origin: "Kiel", Destination: "Hamburg", Kilos: 30000},
			code:    grpc.FailedPrecondition,
		},
	} {
		exit := ranCall(t, grpc.Ask[effect.Unit](client, quoting.Quote, expected.enquiry))
		cause, failed := exit.Cause()
		if !failed {
			t.Errorf("%s: expected a refusal, got %+v", named, exit)
			continue
		}
		failure, typed := cause.Failure()
		if !typed {
			t.Errorf("%s: expected a typed failure, got %+v", named, cause)
			continue
		}
		if failure.Code != expected.code {
			t.Errorf("%s: expected code %v, got %v (%s)",
				named, expected.code, failure.Code, failure.Message)
		}
		if failure.Message == "" {
			t.Errorf("%s: expected a message for an operator", named)
		}
	}
}

func TestARequestTheDescriptionRefusesNeverReachesTheHandler(t *testing.T) {
	// The description is the contract, so a request that does not satisfy it is
	// InvalidArgument whatever the state of the service -- and the refusal
	// happens on the caller's side, before anything is sent, because the caller
	// holds the same description.
	client := quoted(t)
	exit := ranCall(t, grpc.Ask[effect.Unit](client, quoting.Quote,
		quoting.Enquiry{Origin: "Kiel", Destination: "Hamburg", Kilos: 0}))

	cause, failed := exit.Cause()
	if !failed {
		t.Fatalf("expected the enquiry to be refused, got %+v", exit)
	}
	failure, typed := cause.Failure()
	if !typed || failure.Code != grpc.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %+v", cause)
	}
	// The field, because a caller with twelve of them needs to know which.
	if !strings.Contains(failure.Message, "kilos") {
		t.Fatalf("expected the field named, got %q", failure.Message)
	}
}

func TestTheServerRefusesARequestItsOwnDescriptionRejects(t *testing.T) {
	// The test above proves the *caller's* check, because Ask holds the same
	// description and refuses before sending. This is the other half, and the
	// half that matters for a client generated from the projected .proto in
	// another language: it does not hold the description, so the server's own
	// check is the only one.
	//
	// The bytes are what such a client sends for kilos = 0. Protobuf writes
	// nothing at all for an implicit-presence zero, so the field is simply not
	// there -- a message protobuf is perfectly happy with and the description
	// is not.
	client := quoted(t)
	sent := []byte{
		0x0a, 0x04, 'K', 'i', 'e', 'l', // field 1: origin
		0x12, 0x07, 'H', 'a', 'm', 'b', 'u', 'r', 'g', // field 2: destination
	}

	answer, failure := client.Call(context.Background(), quoting.Quote.Path(), sent)
	if failure == nil {
		t.Fatalf("expected the server to refuse it, got %d bytes", len(answer))
	}
	if failure.Code != grpc.InvalidArgument {
		t.Fatalf("expected InvalidArgument from the server, got %v (%s)",
			failure.Code, failure.Message)
	}
	if !strings.Contains(failure.Message, "kilos") {
		t.Errorf("expected the field named, got %q", failure.Message)
	}
	// And the procedure named, because a caller talking to twelve of them
	// needs to know which refused.
	if !strings.Contains(failure.Message, quoting.Quote.Path()) {
		t.Errorf("expected the procedure named, got %q", failure.Message)
	}
}
