package acceptance

// The store's own behaviour. Its operations are effects because it stands in
// for a database, and these check the two things that follow from that: a
// refusal travels in the failure channel, and concurrent writers do not corrupt
// it.

import (
	"context"
	"testing"

	"github.com/mbauer83/effect-golang-web/examples/bookstore"
	"github.com/mbauer83/effect-golang/effect"
)

func TestTheStoreRefusesInTheFailureChannelAndNotThroughAnError(t *testing.T) {
	// The store's refusal is typed and travels where a typed failure travels.
	// It used to come back as an error that the boundary dug the type out of
	// with errors.As, which is the thing a failure channel exists to avoid.
	store := built(t, bookstore.NewStore(bookstore.Book{Title: "Held", Authors: []string{"A"}, Pages: 10}))
	runtime, err := effect.NewRuntime()
	if err != nil {
		t.Fatal(err)
	}

	exit := runtime.Run(context.Background(), effect.Unit{},
		store.Add(bookstore.Book{Title: "Held", Authors: []string{"A"}, Pages: 10}))
	cause, failed := exit.Cause()
	if !failed {
		t.Fatalf("expected the duplicate to be refused, got %+v", exit)
	}
	failures := cause.Failures()
	if len(failures) != 1 || failures[0].Kind != bookstore.AlreadyHeld {
		t.Fatalf("expected the application's own refusal, got %+v", cause)
	}

	// And nothing was added.
	if held := interpreted(t, store.All()); len(held) != 1 {
		t.Fatalf("expected the store unchanged, got %#v", held)
	}
}

func TestTheStoreIsSafeWhenManyRequestsAddAtOnce(t *testing.T) {
	// A server handles requests concurrently, so the store has to survive it.
	// Exactly one of a hundred attempts at one title may succeed.
	store := built(t, bookstore.NewStore())
	runtime, err := effect.NewRuntime()
	if err != nil {
		t.Fatal(err)
	}

	adding := make([]effect.Effect[effect.Unit, bookstore.Fault, effect.Unit], 0, 100)
	for range 100 {
		adding = append(adding, store.Add(
			bookstore.Book{Title: "Contended", Authors: []string{"A"}, Pages: 1}))
	}
	// In parallel, or this would prove nothing: a sequential traversal never
	// has two attempts inside the store at once. The ones that lose report it
	// rather than failing the whole batch.
	outcomes := effect.ForEachPar(adding, func(
		attempt effect.Effect[effect.Unit, bookstore.Fault, effect.Unit],
	) effect.Effect[effect.Unit, effect.Never, bool] {
		return attempt.CatchCause(func(effect.Cause[bookstore.Fault]) effect.Effect[effect.Unit, effect.Never, effect.Unit] {
			return effect.Succeed[effect.Unit, effect.Never](effect.Unit{})
		}).As(true)
	})
	if _, succeeded := runtime.Run(context.Background(), effect.Unit{}, outcomes).Value(); !succeeded {
		t.Fatal("expected every attempt to be accounted for")
	}

	if held := interpreted(t, store.All()); len(held) != 1 {
		t.Fatalf("expected exactly one book, got %d", len(held))
	}
}

func TestAReaderCannotChangeTheStoreThroughWhatItWasGiven(t *testing.T) {
	// A read hands back a copy. Without one, a caller holding the result would
	// be holding the store's own memory, and writing to it would change the
	// store from outside every rule the store enforces.
	store := built(t, bookstore.NewStore(bookstore.Book{Title: "Held", Authors: []string{"A"}, Pages: 10}))

	held := interpreted(t, store.All())
	held[0].Title = "changed"

	if again := interpreted(t, store.All()); again[0].Title != "Held" {
		t.Fatalf("the store was changed through a reader's copy: %#v", again)
	}
}
