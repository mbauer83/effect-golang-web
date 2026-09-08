package acceptance

// The same service over the gRPC wire protocol proper, which is the claim worth
// checking.
//
// Connect was chosen because it speaks gRPC *over* net/http, so one server and
// one middleware stack serve both. That is only true if it really is gRPC on
// the wire, so this runs over HTTP/2 -- h2c, since there is no TLS here -- and
// reads the content type off the request the server received. "application/grpc"
// is what a client generated from the projected .proto file sends, and nothing
// else would do.

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/mbauer83/effect-golang-web/examples/quoting"
	"github.com/mbauer83/effect-golang-web/grpc"
	"github.com/mbauer83/effect-golang/effect"
)

// seen records what the server was actually sent.
type seen struct {
	mutex       sync.Mutex
	contentType string
	protocol    string
}

func (held *seen) note(request *http.Request) {
	held.mutex.Lock()
	defer held.mutex.Unlock()
	held.contentType = request.Header.Get("Content-Type")
	held.protocol = request.Proto
}

func (held *seen) read() (string, string) {
	held.mutex.Lock()
	defer held.mutex.Unlock()
	return held.contentType, held.protocol
}

// overGRPC starts the service behind h2c and returns a gRPC client for it,
// along with what the server saw.
func overGRPC(t *testing.T) (*grpc.Dialled, *seen) {
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
	answering, err := boundary.Handler()
	if err != nil {
		t.Fatal(err)
	}

	// The middleware is the point of the choice: a gRPC procedure goes through
	// the same net/http stack as everything else, so watching it is ordinary.
	noted := &seen{}
	watching := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		noted.note(request)
		answering.ServeHTTP(writer, request)
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{
		Handler:           h2c.NewHandler(watching, &http2.Server{}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() {
		stopping, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		_ = server.Shutdown(stopping)
	})

	// HTTP/2 without TLS, which is what h2c is and what gRPC over plain TCP
	// needs. A deployment with TLS needs none of this.
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLSContext: func(
				ctx context.Context,
				network string,
				address string,
				_ *tls.Config,
			) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, address)
			},
		},
	}
	return grpc.Dial(client, "http://"+listener.Addr().String()), noted
}

func TestTheProcedureIsAnsweredOverTheGRPCProtocolItself(t *testing.T) {
	client, noted := overGRPC(t)

	exit := ranCall(t, grpc.Ask[effect.Unit](client, quoting.Quote,
		quoting.Enquiry{Origin: "Kiel", Destination: "Rotterdam", Kilos: 200}))
	rate, ok := exit.Value()
	if !ok {
		t.Fatalf("unexpected exit: %+v", exit)
	}
	if rate.Carrier != "coastal" || rate.Cents != 21000 {
		t.Fatalf("unexpected rate: %#v", rate)
	}

	contentType, protocol := noted.read()
	// gRPC, not Connect's own protocol: this is what a generated client sends.
	if contentType != "application/grpc" && contentType != "application/grpc+proto" {
		t.Errorf("expected gRPC on the wire, got %q", contentType)
	}
	// Over HTTP/2, because gRPC requires it -- and because that is what makes
	// "one net/http server" a claim rather than a hope.
	if protocol != "HTTP/2.0" {
		t.Errorf("expected HTTP/2, got %q", protocol)
	}
}

func TestARefusalCarriesItsCodeOverTheGRPCProtocolToo(t *testing.T) {
	// The status is a trailer in gRPC and a header in Connect's protocol, so
	// this is a different path through the transport and not the same test
	// twice.
	client, _ := overGRPC(t)
	exit := ranCall(t, grpc.Ask[effect.Unit](client, quoting.Quote,
		quoting.Enquiry{Origin: "Kiel", Destination: "Lima", Kilos: 10}))

	cause, failed := exit.Cause()
	if !failed {
		t.Fatalf("expected a refusal, got %+v", exit)
	}
	failure, typed := cause.Failure()
	if !typed || failure.Code != grpc.NotFound {
		t.Fatalf("expected NotFound over gRPC, got %+v", cause)
	}
}
