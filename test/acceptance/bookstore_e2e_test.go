package acceptance

// The bookstore served over a real socket, by a real client. What matters is
// that one schema serves the request and the response, that the application's
// vocabulary becomes statuses at the boundary and nowhere else, and that
// closing the scope is what shuts the server down.

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mbauer83/effect-golang-web/examples/bookstore"
	"github.com/mbauer83/effect-golang-web/schema"
	"github.com/mbauer83/effect-golang/effect"
)

// running starts the bookstore on a port the operating system chooses and
// returns its base URL. It stops when the test ends.
func running(t *testing.T, store *bookstore.Store) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := effect.NewRuntime(effect.WithDebugTracking())
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := bookstore.Boundary(runtime)
	if err != nil {
		t.Fatal(err)
	}

	serving, stop := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		runtime.Run(serving, effect.Unit{}, bookstore.Serve(listener, boundary, store))
	}()
	t.Cleanup(func() {
		stop()
		select {
		case <-stopped:
		case <-time.After(5 * time.Second):
			t.Error("the server did not stop within five seconds")
		}
		if live := runtime.LiveWork(); !live.IsEmpty() {
			t.Errorf("the server left work behind: %#v", live)
		}
	})
	return "http://" + listener.Addr().String()
}

func get(t *testing.T, url string) *http.Response {
	t.Helper()
	response, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	return response
}

func read(t *testing.T, response *http.Response) []byte {
	t.Helper()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestTheBookstoreAnswersWithTheDocumentItsSchemaDescribes(t *testing.T) {
	base := running(t, bookstore.NewStore(
		bookstore.Book{Title: "Zionomicon", Authors: []string{"John A. De Goes"}, Pages: 632},
	))
	response := get(t, base+"/books")

	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.StatusCode)
	}
	// The client decodes with the same schema the server encoded with, which is
	// the whole claim: one description, both directions.
	books, err := schema.DecodeJSON(bookstore.CatalogueSchema, read(t, response))
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 || books[0].Title != "Zionomicon" {
		t.Fatalf("unexpected collection: %#v", books)
	}
}

func TestAPostedEntityIsDecodedThroughTheSameSchema(t *testing.T) {
	store := bookstore.NewStore()
	base := running(t, store)

	response, err := http.Post(base+"/books", "application/json",
		strings.NewReader(`{"title":"New","authors":["A"],"pages":10}`))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", response.StatusCode, read(t, response))
	}
	if held := store.All(); len(held) != 1 || held[0].Pages != 10 {
		t.Fatalf("unexpected store contents: %#v", held)
	}
}

func TestARefusedEntityBecomesABadRequestThatNamesTheField(t *testing.T) {
	// The rule lives in the schema, the vocabulary lives in the application,
	// and the status is decided once at the boundary. The handler chose none of
	// them.
	base := running(t, bookstore.NewStore())

	response, err := http.Post(base+"/books", "application/json",
		strings.NewReader(`{"title":"New","authors":["A"],"pages":0}`))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", response.StatusCode)
	}
	if body := string(read(t, response)); !strings.Contains(body, "pages") {
		t.Fatalf("expected the field named, got %q", body)
	}
}

func TestSomethingTheStoreDoesNotHoldIsNotFound(t *testing.T) {
	base := running(t, bookstore.NewStore())

	if got := get(t, base+"/books/Missing").StatusCode; got != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", got)
	}
}
