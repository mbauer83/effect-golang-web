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
	"sync"
)

// Book is one entry.
//
//schema:generate
type Book struct {
	// Title is what the book is called.
	Title string `json:"title" schema:"minLength=1,maxLength=200"`
	// Authors are credited in the order the book credits them.
	Authors []string `json:"authors" schema:"maxItems=64"`
	// Pages is how many pages the book has, and there is at least one.
	Pages int `json:"pages" schema:"min=1,max=20000"`
}

// Store holds the collection. It is safe for concurrent use because a server
// handles requests concurrently, and a store that was not would be a defect
// waiting for load rather than a design.
type Store struct {
	mutex sync.RWMutex
	books []Book
}

// NewStore builds a store holding the given books.
func NewStore(books ...Book) *Store {
	return &Store{books: slices.Clone(books)}
}

// All returns a copy, so a reader cannot see a later write through the slice it
// was given.
func (store *Store) All() []Book {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	return slices.Clone(store.books)
}

// Add appends a book, or refuses because the store already holds that title.
//
// The refusal is the application's own, in the application's own vocabulary. It
// is not a status, and nothing here knows that it will become one.
func (store *Store) Add(book Book) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	for _, held := range store.books {
		if held.Title == book.Title {
			return Fault{Kind: AlreadyHeld, Err: errors.New(book.Title + " is already held")}
		}
	}
	store.books = append(store.books, book)
	return nil
}

// Find returns the book with the given title.
func (store *Store) Find(title string) (Book, bool) {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	for _, book := range store.books {
		if book.Title == title {
			return book, true
		}
	}
	return Book{}, false
}
