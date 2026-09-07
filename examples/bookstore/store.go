// Package bookstore is a complete HTTP program on the web core.
//
// It serves a small collection over JSON, decoding and encoding through one
// schema, and its lifetime is a scope: cancelling the caller's context shuts
// the server down and lets the requests already in flight finish.
//
// Dispatch here is a switch on the method and the path. That is deliberate at
// this stage -- routing, path codecs and typed endpoints are the next step, and
// writing this by hand once is the clearest statement of what they will
// replace.
package bookstore

import (
	"slices"
	"sync"
)

// Book is one entry.
type Book struct {
	Title   string
	Authors []string
	Pages   int
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

// Add appends a book and reports how many the store then holds.
func (store *Store) Add(book Book) int {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.books = append(store.books, book)
	return len(store.books)
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
