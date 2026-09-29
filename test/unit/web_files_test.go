package unit

// Files: what net/http does for a file is kept, and what it would do besides
// -- listing a directory, reaching outside the file system -- is not.

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

var assetFiles = fstest.MapFS{
	"logo.svg":         {Data: []byte("<svg/>"), ModTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
	"fonts/body.woff2": {Data: []byte("font")},
}

func servedFile(t *testing.T, path string, header http.Header) *http.Response {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	for name, values := range header {
		request.Header[name] = values
	}
	return dispatched(t, request, web.Files[effect.Unit, Refusal]("/static/", assetFiles))
}

func TestAFileIsServedWithTheTypeItsNameSays(t *testing.T) {
	received := servedFile(t, "/static/logo.svg", nil)
	if received.StatusCode != http.StatusOK || received.Header.Get("Content-Type") != "image/svg+xml" {
		t.Fatalf("expected the svg served, got %d %q", received.StatusCode, received.Header.Get("Content-Type"))
	}
	if nested := servedFile(t, "/static/fonts/body.woff2", nil); nested.StatusCode != http.StatusOK {
		t.Fatalf("expected a nested file served, got %d", nested.StatusCode)
	}
}

func TestAnythingThatIsNotAFileIsNotFound(t *testing.T) {
	for _, path := range []string{"/static/fonts", "/static/missing.svg", "/static/../web_files_test.go", "/static/fonts/../logo.svg"} {
		if received := servedFile(t, path, nil); received.StatusCode != http.StatusNotFound {
			t.Fatalf("expected %s not found, got %d", path, received.StatusCode)
		}
	}
}

func TestAConditionalRequestForAnUnchangedFileIsNotModified(t *testing.T) {
	received := servedFile(t, "/static/logo.svg", http.Header{"If-Modified-Since": {"Thu, 01 Jan 2026 00:00:00 GMT"}})
	if received.StatusCode != http.StatusNotModified {
		t.Fatalf("expected 304, got %d", received.StatusCode)
	}
}
