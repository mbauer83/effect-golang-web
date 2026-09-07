package unit

// The boundary between a handler and net/http. What matters is that a typed
// failure becomes the status the application chose, that a defect never becomes
// one by accident, and that the runtime still records what happened.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
	"github.com/mbauer83/effect-golang/effecttest"
)

// Refusal is an application's own failure type, with its own idea of what a
// status should be -- which is the point: the handler does not decide.
type Refusal struct{ Because string }

func (refusal Refusal) Error() string { return refusal.Because }

func refusalStatus(refusal Refusal) web.Response {
	return web.Text(http.StatusConflict, refusal.Because)
}

type webEffect[A any] = effect.Effect[effect.Unit, Refusal, A]

// served interprets one handler through the boundary and returns what a client
// would have received.
func served(t *testing.T, handler web.Handler[effect.Unit, Refusal], options ...effect.RuntimeOption) *http.Response {
	t.Helper()
	runtime, err := effect.NewRuntime(options...)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := web.NewAdapter(runtime, effect.Unit{}, refusalStatus)
	if err != nil {
		t.Fatal(err)
	}
	// Reports are discarded here so a deliberate defect does not write to the
	// test's own error output; what the default does with one is not the
	// subject of these cases.
	recorder := httptest.NewRecorder()
	adapter.WithReport(func(context.Context, error) {}).
		Handler(handler).
		ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	return recorder.Result()
}

func TestASuccessfulHandlerAnswersWithItsResponse(t *testing.T) {
	received := served(t, web.Respond[effect.Unit, Refusal](web.Text(http.StatusOK, "hello")))

	if received.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", received.StatusCode)
	}
	if body := bodyOf(t, received); body != "hello" {
		t.Fatalf("unexpected body %q", body)
	}
}

func TestATypedFailureBecomesTheStatusTheApplicationChose(t *testing.T) {
	refusing := func(web.Request) webEffect[web.Response] {
		return effect.For[effect.Unit, Refusal]().Fail[web.Response](Refusal{Because: "already exists"})
	}
	received := served(t, refusing)

	if received.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", received.StatusCode)
	}
	if body := bodyOf(t, received); body != "already exists" {
		t.Fatalf("unexpected body %q", body)
	}
}

func TestADefectIsAFiveHundredWithNothingSaidToTheClient(t *testing.T) {
	// A defect is something the application did not account for, so it must not
	// become a status of its own choosing and its text is not fit to send on.
	received := served(t, defecting)

	if received.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", received.StatusCode)
	}
	if body := bodyOf(t, received); body != "" {
		t.Fatalf("expected no detail sent to the client, got %q", body)
	}
}

func TestADefectReachesTheBoundarysReportBecauseNothingElseSeesIt(t *testing.T) {
	// MEASURED, and the reason this hook is not optional: the runtime emits its
	// fiber events for forked fibers, so a defect in the effect the boundary
	// interprets directly -- which is what every request is -- reaches an
	// observer nowhere. An observer installed on the runtime records nothing
	// for this request.
	observer := &effecttest.RecordingObserver{}
	runtime, err := effect.NewRuntime(effect.WithObserver(observer))
	if err != nil {
		t.Fatal(err)
	}
	reported := make([]error, 0, 1)
	adapter, err := web.NewAdapter(runtime, effect.Unit{}, refusalStatus)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	adapter.WithReport(func(_ context.Context, err error) { reported = append(reported, err) }).
		Handler(defecting).
		ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if len(reported) != 1 || !strings.Contains(reported[0].Error(), "a bug") {
		t.Fatalf("expected the defect reported once, got %v", reported)
	}
	for _, event := range observer.Events() {
		if event.Status == "defect" {
			t.Fatalf("the runtime did record it after all: %#v", event)
		}
	}
}

// defecting is a handler that fails in a way the application did not describe.
func defecting(web.Request) webEffect[web.Response] {
	return effect.From(func(context.Context, effect.Unit) effect.Exit[Refusal, web.Response] {
		return effect.ExitCause[Refusal, web.Response](
			effect.DieCause[Refusal](effect.Defect{Value: errors.New("a bug")}),
		)
	})
}

func TestAnInterruptedHandlerAnswersUnavailableRatherThanFailing(t *testing.T) {
	// A client that goes away, or a server shutting down, is not the
	// application failing. It must not reach the failure mapping.
	runtime, err := effect.NewRuntime()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := web.NewAdapter(runtime, effect.Unit{},
		func(Refusal) web.Response { return web.Text(http.StatusConflict, "wrong") })
	if err != nil {
		t.Fatal(err)
	}

	cancelled, stop := context.WithCancel(context.Background())
	stop()
	request := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(cancelled)
	recorder := httptest.NewRecorder()
	adapter.Handler(web.Respond[effect.Unit, Refusal](web.Text(http.StatusOK, "hello"))).
		ServeHTTP(recorder, request)

	if got := recorder.Result().StatusCode; got != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", got)
	}
}

func TestTheDefectResponseAndTheReportAreReplaceable(t *testing.T) {
	runtime, err := effect.NewRuntime()
	if err != nil {
		t.Fatal(err)
	}
	reported := make([]error, 0, 1)
	adapter, err := web.NewAdapter(runtime, effect.Unit{}, refusalStatus)
	if err != nil {
		t.Fatal(err)
	}
	adapter = adapter.
		WithDefectResponse(func(effect.Cause[Refusal]) web.Response {
			return web.Text(http.StatusBadGateway, "ask again later")
		}).
		WithReport(func(_ context.Context, err error) { reported = append(reported, err) })

	recorder := httptest.NewRecorder()
	adapter.Handler(defecting).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if got := recorder.Result().StatusCode; got != http.StatusBadGateway {
		t.Fatalf("expected the replacement status, got %d", got)
	}
	if len(reported) != 1 || !strings.Contains(reported[0].Error(), "a bug") {
		t.Fatalf("expected the defect reported once, got %v", reported)
	}
}

func TestAnAdapterWithoutWhatItNeedsIsRefusedAtCompositionTime(t *testing.T) {
	// A handler cannot supply a runtime or a status for its failure, so an
	// adapter missing either is a declaration mistake -- and one that would
	// otherwise surface on the first request in production.
	if _, err := web.NewAdapter[effect.Unit, Refusal](nil, effect.Unit{}, refusalStatus); err == nil {
		t.Error("expected a nil runtime to be refused")
	}
	runtime, err := effect.NewRuntime()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := web.NewAdapter[effect.Unit, Refusal](runtime, effect.Unit{}, nil); err == nil {
		t.Error("expected a missing failure mapping to be refused")
	}
}
