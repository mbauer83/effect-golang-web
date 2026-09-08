package acceptance

// What a gRPC boundary does with a defect.
//
// Its own subject, because two things have to hold at once and they pull in
// opposite directions: the caller must be told nothing, and the operator must
// be told everything. A boundary that satisfied only the first would lose the
// defect entirely.

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

// breaking is a handler that panics, which is what a defect is.
func breaking(quoting.Enquiry) effect.Effect[effect.Unit, quoting.Refusal, quoting.Rate] {
	return effect.From(
		func(context.Context, effect.Unit) effect.Exit[quoting.Refusal, quoting.Rate] {
			panic("the rate table is not there")
		})
}

// broken serves the quoting procedure with a handler that panics, and hands
// back a client and whatever the boundary reported.
//
// choose is the boundary's answer for a defect, or nil to keep the default.
func broken(
	t *testing.T,
	choose func(effect.Cause[quoting.Refusal]) grpc.Failure,
) (*grpc.Dialled, chan error) {
	t.Helper()
	runtime, err := effect.NewRuntime(effect.WithDebugTracking())
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := grpc.NewBoundary(runtime, effect.Unit{},
		grpc.NewConnected(), quoting.Coded)
	if err != nil {
		t.Fatal(err)
	}

	reported := make(chan error, 1)
	boundary = boundary.WithReport(func(_ context.Context, err error) {
		select {
		case reported <- err:
		default:
		}
	})
	if choose != nil {
		boundary = boundary.WithDefectFailure(choose)
	}
	if err := grpc.Answer(boundary, quoting.Quote, breaking); err != nil {
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

	return grpc.DialConnect(&http.Client{Timeout: 10 * time.Second},
		"http://"+listener.Addr().String()), reported
}

// askedOfBroken calls the procedure and returns the failure it answered with.
func askedOfBroken(t *testing.T, client *grpc.Dialled) grpc.Failure {
	t.Helper()
	exit := ranCall(t, grpc.Ask[effect.Unit](client, quoting.Quote,
		quoting.Enquiry{Origin: "Kiel", Destination: "Hamburg", Kilos: 1}))

	cause, failed := exit.Cause()
	if !failed {
		t.Fatalf("expected the defect to become a failure, got %+v", exit)
	}
	failure, typed := cause.Failure()
	if !typed {
		t.Fatalf("expected a typed failure, got %+v", cause)
	}
	return failure
}

func TestADefectAnswersWithNoDetailAndIsReportedInstead(t *testing.T) {
	// A defect is by definition something the application did not account for,
	// so its text is not fit to send to a caller: the answer is Unknown with
	// nothing in it. It has to reach the operator some other way, and MEASURED,
	// the runtime emits its fiber events for forked fibers only -- so a defect
	// in the effect a boundary interprets directly reaches no observer on its
	// own, and the boundary's own report is the only place it appears.
	client, reported := broken(t, nil)
	failure := askedOfBroken(t, client)

	if failure.Code != grpc.Unknown {
		t.Errorf("expected Unknown, got %v", failure.Code)
	}
	// Nothing of the panic in what the caller was told.
	if strings.Contains(failure.Message, "rate table") {
		t.Errorf("the defect's text reached the caller: %q", failure.Message)
	}

	// And it did reach the operator, with the text.
	select {
	case noted := <-reported:
		if !strings.Contains(noted.Error(), "rate table") {
			t.Errorf("expected the defect reported with its text, got %v", noted)
		}
	case <-time.After(2 * time.Second):
		t.Error("the defect reached no observer at all")
	}
}

func TestABoundaryCanChooseWhatADefectAnswersWith(t *testing.T) {
	// The default is right for a service whose callers are strangers. A service
	// whose callers are its own team may want Internal and a correlation
	// identifier, and that is the boundary's decision rather than this
	// package's -- so it is replaceable, and the replacement has to reach the
	// answer the caller actually receives.
	chosen := grpc.Failure{Code: grpc.Internal, Message: "see trace 4711"}
	client, reported := broken(t,
		func(effect.Cause[quoting.Refusal]) grpc.Failure { return chosen })

	failure := askedOfBroken(t, client)
	if failure.Code != chosen.Code || failure.Message != chosen.Message {
		t.Fatalf("expected the chosen answer, got %+v", failure)
	}
	// Still reported, because choosing what the caller is told does not change
	// what the operator needs.
	select {
	case noted := <-reported:
		if !strings.Contains(noted.Error(), "rate table") {
			t.Errorf("expected the defect still reported, got %v", noted)
		}
	case <-time.After(2 * time.Second):
		t.Error("the defect reached no observer at all")
	}
}
