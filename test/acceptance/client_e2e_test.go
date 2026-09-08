package acceptance

// The client against the server, through the endpoints both hold.
//
// The other bookstore suite calls it with raw net/http, which is what says
// there is a real server answering something that has never seen this module.
// What that cannot say is the other half of the claim: that one Endpoint value
// dispatches a request, projects into the contract, and is what a caller
// calls. These say it -- the same three declarations, read from the other side.

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/mbauer83/effect-golang-web/examples/bookstore"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

// called interprets one client effect.
func called[A any](
	t *testing.T,
	fx effect.Effect[effect.Unit, web.Fault, A],
) effect.Exit[web.Fault, A] {
	t.Helper()
	runtime, err := effect.NewRuntime()
	if err != nil {
		t.Fatal(err)
	}
	return runtime.Run(context.Background(), effect.Unit{}, fx)
}

func TestAClientCallsTheEndpointsTheServerServes(t *testing.T) {
	store := built(t, bookstore.NewStore())
	client := web.Dial(http.DefaultClient, running(t, store))
	added := bookstore.Book{Title: "Called", Authors: []string{"A Client"}, Pages: 12}

	sending, err := web.Carrying(web.Requesting{}, bookstore.BookSchema, added)
	if err != nil {
		t.Fatal(err)
	}
	created, ok := called(t, web.Call[effect.Unit](client, bookstore.AddBook, sending)).Value()
	if !ok {
		t.Fatalf("expected the book added, got %+v",
			called(t, web.Call[effect.Unit](client, bookstore.AddBook, sending)))
	}
	if created.Title != added.Title || created.Pages != added.Pages {
		t.Fatalf("unexpected book: %#v", created)
	}

	// The title fills the endpoint's captured segment by name, so the path is
	// built from the declaration and not pasted together by the caller.
	read, ok := called(t, web.Call[effect.Unit](client, bookstore.FindBook,
		web.Requesting{Path: map[string]string{"title": added.Title}})).Value()
	if !ok {
		t.Fatal("expected the book read back")
	}
	if read.Title != added.Title {
		t.Fatalf("unexpected book: %#v", read)
	}

	catalogue, ok := called(t, web.Call[effect.Unit](client, bookstore.ListBooks,
		web.Requesting{})).Value()
	if !ok || len(catalogue) != 1 {
		t.Fatalf("unexpected catalogue: %#v", catalogue)
	}
}

func TestAStatusTheEndpointDidNotDeclareArrivesAsARefusal(t *testing.T) {
	// The endpoint declares 200. The server answers 404, which is a status it
	// documented and still not the one it promised -- so the caller is told
	// what arrived rather than handed a zero value.
	client := web.Dial(http.DefaultClient, running(t, built(t, bookstore.NewStore())))

	exit := called(t, web.Call[effect.Unit](client, bookstore.FindBook,
		web.Requesting{Path: map[string]string{"title": "Missing"}}))
	cause, failed := exit.Cause()
	if !failed {
		t.Fatalf("expected the call to fail, got %+v", exit)
	}
	fault, is := cause.Failure()
	if !is {
		t.Fatalf("expected a typed failure, got %v", cause)
	}
	var refusal web.Refusal
	if !errors.As(fault, &refusal) {
		t.Fatalf("expected a Refusal, got %v", fault)
	}
	if refusal.Status != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", refusal.Status)
	}
}

func TestACaptureWithNothingToFillItIsRefusedBeforeAnythingIsSent(t *testing.T) {
	// Otherwise the literal "{title}" goes out, the server answers 404, and
	// the caller is looking at the wrong end of its own mistake.
	client := web.Dial(http.DefaultClient, "http://127.0.0.1:1")

	exit := called(t, web.Call[effect.Unit](client, bookstore.FindBook, web.Requesting{}))
	cause, failed := exit.Cause()
	if !failed {
		t.Fatalf("expected the call to be refused, got %+v", exit)
	}
	fault, _ := cause.Failure()
	if fault.Doing != "building the path" {
		t.Fatalf("unexpected fault: %+v", fault)
	}
}
