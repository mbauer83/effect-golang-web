package unit

// The boundary's other door. A transport that takes over the connection -- a
// websocket, an event stream -- answers with no response, so it cannot go
// through Handler and still wants what the boundary owns.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

func TestInterpretRunsAnExchangeTheWayAHandlerIsRun(t *testing.T) {
	runtime, err := effect.NewRuntime()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := web.NewAdapter(runtime, effect.Unit{}, refusalStatus)
	if err != nil {
		t.Fatal(err)
	}

	ran := false
	adapter.WithReport(func(context.Context, error) {}).
		Interpret(context.Background(), effect.From(
			func(context.Context, effect.Unit) effect.Exit[Refusal, effect.Unit] {
				ran = true
				return effect.ExitSuccess[Refusal](effect.Unit{})
			}))
	if !ran {
		t.Fatal("expected the exchange to be interpreted")
	}
}

func TestAnExchangeThatEndedBadlyIsReportedBecauseNoClientCanBeTold(t *testing.T) {
	// There is no response to put a status in: the exchange is over and the
	// peer is gone. Reporting it is the only thing left.
	reported := []error{}
	boundary := interpreting(t, func(_ context.Context, err error) {
		reported = append(reported, err)
	})

	boundary.Interpret(context.Background(), effect.From(
		func(context.Context, effect.Unit) effect.Exit[Refusal, effect.Unit] {
			return effect.ExitFailure[Refusal, effect.Unit](Refusal{Because: "it went wrong"})
		}))

	if len(reported) != 1 || !strings.Contains(reported[0].Error(), "it went wrong") {
		t.Fatalf("expected the failure reported once, got %v", reported)
	}
}

func TestAnInterruptedExchangeIsNotReported(t *testing.T) {
	// A conversation that ended because its scope closed is not a fault, and
	// reporting one would fill a log with every exchange that ended normally.
	reported := []error{}
	boundary := interpreting(t, func(_ context.Context, err error) {
		reported = append(reported, err)
	})

	stopped, stop := context.WithCancel(context.Background())
	stop()
	boundary.Interpret(stopped, effect.From(
		func(context.Context, effect.Unit) effect.Exit[Refusal, effect.Unit] {
			return effect.ExitSuccess[Refusal](effect.Unit{})
		}))

	if len(reported) != 0 {
		t.Fatalf("expected an interruption to pass without comment, got %v", reported)
	}
}

func TestADefectInAnExchangeIsReported(t *testing.T) {
	reported := []error{}
	boundary := interpreting(t, func(_ context.Context, err error) {
		reported = append(reported, err)
	})

	boundary.Interpret(context.Background(), effect.From(
		func(context.Context, effect.Unit) effect.Exit[Refusal, effect.Unit] {
			return effect.ExitCause[Refusal, effect.Unit](
				effect.DieCause[Refusal](effect.Defect{Value: errors.New("a bug")}))
		}))

	if len(reported) != 1 || !strings.Contains(reported[0].Error(), "a bug") {
		t.Fatalf("expected the defect reported, got %v", reported)
	}
}

func interpreting(t *testing.T, report func(context.Context, error)) web.Adapter[effect.Unit, Refusal] {
	t.Helper()
	runtime, err := effect.NewRuntime()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := web.NewAdapter(runtime, effect.Unit{}, refusalStatus)
	if err != nil {
		t.Fatal(err)
	}
	return adapter.WithReport(report)
}
