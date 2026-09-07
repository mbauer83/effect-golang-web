package unit

// Handlers. A handler is a description, the request is a value, and the
// net/http boundary works in both directions.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

func TestBuildingAHandlerHandlesNothing(t *testing.T) {
	// Nothing runs until the runtime interprets it. A handler that did work
	// when it was composed could not be retried, raced or timed out.
	handled := 0
	handler := func(web.Request) webEffect[web.Response] {
		return effect.From(func(context.Context, effect.Unit) effect.Exit[Refusal, web.Response] {
			handled++
			return effect.ExitSuccess[Refusal](web.Text(http.StatusOK, "done"))
		})
	}

	description := web.Transform(handler, func(response web.Response) web.Response {
		return response.WithHeader("X-Wrapped", "yes")
	})(web.RequestFrom(httptest.NewRequest(http.MethodGet, "/", nil)))

	if handled != 0 {
		t.Fatalf("composing a handler ran it %d times", handled)
	}

	runtime, err := effect.NewRuntime()
	if err != nil {
		t.Fatal(err)
	}
	response, succeeded := runtime.Run(context.Background(), effect.Unit{}, description).Value()
	if !succeeded || handled != 1 {
		t.Fatalf("expected one interpretation to handle once, handled %d", handled)
	}
	if response.Header().Get("X-Wrapped") != "yes" {
		t.Fatalf("expected the transform applied, got %v", response.Header())
	}
}

func TestARequestExposesItsPartsAsValues(t *testing.T) {
	underlying := httptest.NewRequest(http.MethodPost, "/books?shelf=one", strings.NewReader("body"))
	underlying.Header.Set("X-Trace", "abc")
	request := web.RequestFrom(underlying).WithCaptures(map[string]string{"id": "42"})

	if request.Method() != http.MethodPost || request.Path() != "/books" {
		t.Fatalf("unexpected method or path: %s %s", request.Method(), request.Path())
	}
	if request.Query().Get("shelf") != "one" || request.Header().Get("X-Trace") != "abc" {
		t.Fatalf("unexpected query or header: %v %v", request.Query(), request.Header())
	}
	captured, matched := request.Capture("id")
	if !matched || captured != "42" {
		t.Fatalf("expected the captured segment, got %q", captured)
	}
	if _, matched := request.Capture("missing"); matched {
		t.Fatal("expected an unmatched capture to say so")
	}
	if request.Underlying() != underlying {
		t.Fatal("expected the original request to stay reachable")
	}
}

func TestAddingCapturesLeavesTheOriginalRequestAlone(t *testing.T) {
	// A route derives a request; it does not alter the one it was given.
	original := web.RequestFrom(httptest.NewRequest(http.MethodGet, "/", nil))
	_ = original.WithCaptures(map[string]string{"id": "42"})

	if _, matched := original.Capture("id"); matched {
		t.Fatal("the original request was changed")
	}
}

func TestReadingTheBodyIsAnEffectThatCanFail(t *testing.T) {
	runtime, err := effect.NewRuntime()
	if err != nil {
		t.Fatal(err)
	}
	request := web.RequestFrom(httptest.NewRequest(http.MethodPost, "/", strings.NewReader("entity")))

	body, succeeded := runtime.Run(context.Background(), effect.Unit{},
		web.Body[effect.Unit](request)).Value()
	if !succeeded || string(body) != "entity" {
		t.Fatalf("expected the entity, got %q", body)
	}

	refusing := web.RequestFrom(httptest.NewRequest(http.MethodPost, "/", refusingReader{}))
	exit := runtime.Run(context.Background(), effect.Unit{}, web.Body[effect.Unit](refusing))
	cause, failed := exit.Cause()
	if !failed {
		t.Fatalf("expected a body that cannot be read to fail, got %+v", exit)
	}
	failures := cause.Failures()
	if len(failures) != 1 || failures[0].Doing != "reading the request body" {
		t.Fatalf("expected the stage named, got %+v", cause)
	}
}

type refusingReader struct{}

func (refusingReader) Read([]byte) (int, error) { return 0, errRefused }

func TestAnExistingHTTPHandlerBecomesAHandler(t *testing.T) {
	// The ecosystem boundary stays usable in both directions, which is the same
	// rule the runtime follows with native channels.
	existing := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-From", "net/http")
		_, _ = writer.Write([]byte("served " + request.URL.Path))
	})
	received := served(t, web.FromHTTP[effect.Unit, Refusal](existing))

	if received.StatusCode != http.StatusOK || received.Header.Get("X-From") != "net/http" {
		t.Fatalf("unexpected response: %d %v", received.StatusCode, received.Header)
	}
	if body := bodyOf(t, received); body != "served /" {
		t.Fatalf("unexpected body %q", body)
	}
}
