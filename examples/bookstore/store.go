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
	"context"
	"errors"
	"slices"
	"sync"

	"github.com/mbauer83/effect-golang/effect"
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
// Its operations are effects, not calls. That is not ceremony: this stands in
// for a database, and a database blocks, fails and has to be cancellable. A
// store whose methods returned values and errors would teach the wrong shape
// for the thing it stands in for -- and it did, until Add's typed refusal had
// to be dug back out of an error with errors.As at the boundary.
//
// It is safe for concurrent use because a server handles requests concurrently.
// The lock is inside the evaluation, where it belongs: the effect is the
// description, and nothing is held while one is being composed.
type Store struct {
	mutex sync.RWMutex
	books []Book
}

// NewStore builds a store holding the given books.
func NewStore(books ...Book) *Store {
	return &Store{books: slices.Clone(books)}
}

// All lists the collection.
//
// It hands back a copy, so a reader cannot see a later write through the slice
// it was given.
func (store *Store) All() storeEffect[[]Book] {
	return reading(store, func() effect.Exit[Fault, []Book] {
		return effect.ExitSuccess[Fault](slices.Clone(store.books))
	}).Named("list-books")
}

// Add appends a book, or refuses because the store already holds that title.
//
// The refusal is the application's own, in the application's own vocabulary,
// and it travels in the failure channel rather than as an error a caller has to
// interrogate. Nothing here knows it will become a status.
func (store *Store) Add(book Book) storeEffect[effect.Unit] {
	return writing(store, func() effect.Exit[Fault, effect.Unit] {
		for _, held := range store.books {
			if held.Title == book.Title {
				return effect.ExitFailure[Fault, effect.Unit](Fault{
					Kind: AlreadyHeld,
					Err:  errors.New(book.Title + " is already held"),
				})
			}
		}
		store.books = append(store.books, book)
		return effect.ExitSuccess[Fault](effect.Unit{})
	}).Named("add-book")
}

// Find returns the book with the given title, or refuses because there is none.
func (store *Store) Find(title string) storeEffect[Book] {
	return reading(store, func() effect.Exit[Fault, Book] {
		for _, book := range store.books {
			if book.Title == title {
				return effect.ExitSuccess[Fault](book)
			}
		}
		return effect.ExitFailure[Fault, Book](Fault{Kind: NotFound})
	}).Named("find-book")
}

// reading and writing hold the lock for exactly as long as the step they are
// given, so every operation acquires it the same way and none of them can
// forget to let it go. Which of the two an operation uses is the only thing it
// has to decide.
func reading[A any](store *Store, step func() effect.Exit[Fault, A]) storeEffect[A] {
	return effect.From(func(context.Context, effect.Unit) effect.Exit[Fault, A] {
		store.mutex.RLock()
		defer store.mutex.RUnlock()
		return step()
	})
}

func writing[A any](store *Store, step func() effect.Exit[Fault, A]) storeEffect[A] {
	return effect.From(func(context.Context, effect.Unit) effect.Exit[Fault, A] {
		store.mutex.Lock()
		defer store.mutex.Unlock()
		return step()
	})
}
