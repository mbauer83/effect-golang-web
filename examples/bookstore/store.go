// Package bookstore is a complete HTTP program on the web core.
//
// It serves a small collection over JSON, decoding and encoding through one
// schema, and its lifetime is a scope: cancelling the caller's context shuts
// the server down and lets the requests already in flight finish.
//
// Each endpoint is a declaration and its handler is separate, so dispatch and
// the published contract come from one value. A handler takes the decoded input
// and returns the value to answer with: it never sees a status, because which
// status its refusal becomes is the boundary's decision and not the handler's.
package bookstore

import (
	"errors"
	"slices"

	"github.com/mbauer83/effect-golang/effect"
	"github.com/mbauer83/effect-golang/experimental/direct"
)

// Book is one entry.
//
// The schema for it is written by hand in shapes.go, which is what Struct and
// FieldOf are for: the Go type came first and the schema binds it.
type Book struct {
	Title   string
	Authors []string
	Pages   int
}

// Store holds the collection.
//
// The books live in a Ref, so what used to be a mutex around a slice is now the
// cell's business rather than the store's: Add's check and append are one
// Modify, which is the step a lock was there to make indivisible.
//
// Its operations are effects, not calls. That is not ceremony: this stands in
// for a database, and a database blocks, fails and has to be cancellable. A
// store whose methods returned values and errors would teach the wrong shape
// for the thing it stands in for -- and it did, until Add's typed refusal had
// to be dug back out of an error with errors.As at the boundary.
type Store struct {
	books effect.Ref[[]Book]
}

// NewStore builds a store holding the given books.
//
// It is an effect because the Ref it holds is: state that existed before
// interpretation would be shared between runs and between the attempts of a
// retry, which is never what a program meant.
func NewStore(books ...Book) effect.Effect[effect.Unit, effect.Never, *Store] {
	return effect.NewRef[effect.Unit](slices.Clone(books)).
		Map(func(held effect.Ref[[]Book]) *Store { return &Store{books: held} })
}

// All lists the collection.
//
// It hands back a copy, so a reader cannot change the store through what it was
// given.
func (store *Store) All() storeEffect[[]Book] {
	return widened(store.books.Get[effect.Unit]().Map(slices.Clone)).Named("list-books")
}

// Add appends a book, or refuses because the store already holds that title.
//
// The check and the append are one Modify, so two requests adding the same
// title cannot both find it absent. The refusal is the application's own, in
// the application's own vocabulary, and it travels in the failure channel
// rather than as an error a caller has to interrogate; nothing here knows it
// will become a status.
func (store *Store) Add(book Book) storeEffect[effect.Unit] {
	added := effect.Modify[effect.Unit](store.books, func(held []Book) ([]Book, bool) {
		if slices.ContainsFunc(held, sameTitle(book.Title)) {
			return held, false
		}
		return append(held, book), true
	})
	// Direct style: read the answer, then decide. As a FlatMap the deciding
	// was nested inside the reading, which is the wrong way round for
	// something that happens after it -- and a refusal is bound like anything
	// else, because binding one abandons the body, which is what a refusal
	// means.
	return direct.Run(func(bind *storing) effect.Unit {
		if direct.Bind(bind, widened(added)) {
			return effect.Unit{}
		}
		direct.Bind(bind, effect.For[effect.Unit, Fault]().Fail[effect.Unit](Fault{
			Kind: AlreadyHeld,
			Err:  errors.New(book.Title + " is already held"),
		}))
		return effect.Unit{}
	}).Named("add-book")
}

// Find returns the book with the given title, or refuses because there is none.
func (store *Store) Find(title string) storeEffect[Book] {
	return direct.Run(func(bind *storing) Book {
		held := direct.Bind(bind, widened(store.books.Get[effect.Unit]()))
		index := slices.IndexFunc(held, sameTitle(title))
		if index < 0 {
			direct.Bind(bind, effect.For[effect.Unit, Fault]().
				Fail[Book](Fault{Kind: NotFound}))
		}
		return held[index]
	}).Named("find-book")
}

// storing is the binder the store's operations bind in.
//
// Direct style throughout, because every one of them reads the cell and then
// decides -- and as FlatMaps the deciding was nested inside the reading. None
// of these bodies holds a defer, which is the condition: in direct style a
// defer runs on an ordinary domain failure and not only on a panic.
type storing = direct.Binder[effect.Unit, Fault]

func sameTitle(title string) func(Book) bool {
	return func(held Book) bool { return held.Title == title }
}

// widened puts an operation that cannot fail into the store's failure channel,
// so a read and a refusal compose in one expression.
func widened[A any](fx effect.Effect[effect.Unit, effect.Never, A]) storeEffect[A] {
	return effect.For[effect.Unit, Fault]().WidenError(fx)
}
