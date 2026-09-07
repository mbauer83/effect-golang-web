package unit

// Responses. What matters is that a status cannot be taken back once it has
// gone out, that a length is declared when it is known, and that a wrapper can
// add a header without owning the response.

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mbauer83/effect-golang-web/schema"
	"github.com/mbauer83/effect-golang-web/web"
)

// sent writes a response the way the boundary does and reports what a client
// would have received.
func sent(t *testing.T, response web.Response) *http.Response {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	if err := response.WriteTo(recorder, request); err != nil {
		t.Fatal(err)
	}
	return recorder.Result()
}

func bodyOf(t *testing.T, response *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestATextResponseDeclaresItsTypeAndLength(t *testing.T) {
	received := sent(t, web.Text(http.StatusOK, "hello"))

	if received.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", received.StatusCode)
	}
	if got := received.Header.Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("unexpected content type %q", got)
	}
	if got := received.Header.Get("Content-Length"); got != "5" {
		t.Fatalf("expected the length declared, got %q", got)
	}
	if body := bodyOf(t, received); body != "hello" {
		t.Fatalf("unexpected body %q", body)
	}
}

func TestAJSONResponseEncodesThroughItsSchema(t *testing.T) {
	response, err := web.JSON(http.StatusCreated, bookSchema,
		Book{Title: "T", Authors: []string{}, Pages: 1})
	if err != nil {
		t.Fatal(err)
	}
	received := sent(t, response)

	if received.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", received.StatusCode)
	}
	if got := received.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("unexpected content type %q", got)
	}
	want := `{"title":"T","authors":[],"pages":1,"hasIndex":false}`
	if body := strings.TrimSpace(bodyOf(t, received)); body != want {
		t.Fatalf("expected\n  %s\ngot\n  %s", want, body)
	}
}

func TestAnUnencodableValueIsRefusedBeforeAnythingIsWritten(t *testing.T) {
	// A status cannot be taken back, so the encoding has to happen first. The
	// alternative is a 200 with a truncated body, which is worse than an error.
	_, err := web.JSON(http.StatusOK, floatSchema, 1/zero())
	if err == nil {
		t.Fatal("expected a value that cannot be encoded to be refused")
	}
	var fault web.Fault
	if !errors.As(err, &fault) || fault.Doing != "encoding the response body" {
		t.Fatalf("expected the stage named, got %v", err)
	}
}

// floatSchema and zero exist so a value that cannot be encoded can be built
// without the compiler folding it into a constant.
var floatSchema = schema.Float64()

func zero() float64 { return 0 }

func TestAddingAHeaderLeavesTheOriginalResponseAlone(t *testing.T) {
	// Middleware wraps a response it does not own, so WithHeader has to copy.
	original := web.Text(http.StatusOK, "hello")
	wrapped := original.WithHeader("Cache-Control", "no-store")

	if original.Header().Get("Cache-Control") != "" {
		t.Fatal("the original response was changed")
	}
	if got := sent(t, wrapped).Header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("expected the header on the wrapper, got %q", got)
	}
}

func TestAStreamingResponseWritesAsItGoes(t *testing.T) {
	response := web.Streaming(http.StatusOK, "text/plain", func(writer io.Writer) error {
		for _, chunk := range []string{"one ", "two ", "three"} {
			if _, err := writer.Write([]byte(chunk)); err != nil {
				return err
			}
		}
		return nil
	})
	received := sent(t, response)

	if received.Header.Get("Content-Length") != "" {
		t.Fatal("a length was declared for a body whose length is not known")
	}
	if body := bodyOf(t, received); body != "one two three" {
		t.Fatalf("unexpected body %q", body)
	}
}

func TestADelegatedResponseWritesItsOwnStatusAndHeaders(t *testing.T) {
	// Nothing is buffered on the handler's behalf and nothing overrides it,
	// which is what keeps an existing http.Handler exactly what it was.
	delegate := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("X-Origin", "delegate")
		writer.WriteHeader(http.StatusTeapot)
		_, _ = writer.Write([]byte("brewed"))
	})
	received := sent(t, web.Delegate(delegate).WithHeader("X-Wrapper", "outer"))

	if received.StatusCode != http.StatusTeapot {
		t.Fatalf("expected the delegate's status, got %d", received.StatusCode)
	}
	if received.Header.Get("X-Origin") != "delegate" ||
		received.Header.Get("X-Wrapper") != "outer" {
		t.Fatalf("expected both sets of headers, got %v", received.Header)
	}
	if body := bodyOf(t, received); body != "brewed" {
		t.Fatalf("unexpected body %q", body)
	}
}

func TestChangingTheStatusLeavesTheRestOfTheResponseAlone(t *testing.T) {
	// A wrapper that rewrites the status keeps the headers it was given, and does
	// not alter the response it was handed.
	original := web.Text(http.StatusOK, "hello").WithHeader("ETag", `"abc"`)
	rewritten := original.WithStatus(http.StatusAccepted)

	if original.Status() != http.StatusOK {
		t.Fatalf("the original response was changed, now %d", original.Status())
	}
	received := sent(t, rewritten)
	if received.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", received.StatusCode)
	}
	if received.Header.Get("ETag") != `"abc"` {
		t.Fatalf("expected the header kept, got %v", received.Header)
	}
}
