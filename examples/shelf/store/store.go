// Package store keeps books in a relational database.
//
// It says only where the table differs from the domain: the ISBN's column is
// named for the number it holds. Everything else follows -- the columns in
// snake_case, the edition flattened into edition_format and
// edition_language, a check for every rule the domain states, and a unique
// key on the identity -- and the listing declares what a reader may ask of
// the catalogue: two sorts, two searches, and pages of a size.
package store

import (
	"github.com/mbauer83/effect-golang-sql/ddl"
	"github.com/mbauer83/effect-golang-sql/sql"
	"github.com/mbauer83/effect-golang/effect"

	"github.com/mbauer83/effect-golang-web/examples/shelf/domain"
)

var f = domain.BookFields

// books is where books are kept, found by their ISBN.
var books = sql.NewRepository(sql.Map(domain.BookSchema).Column(f.ISBN, "isbn13"), f.ISBN.Shape())

// Catalogue is the books read a page at a time: by title unless a reader asks
// for the most recent editions first, found by the start of a title or by
// words anywhere in the title or the author, twenty to a page unless asked.
var Catalogue = books.Listing().
	Sort("title", books.Of(f.Title).Ascending()).
	Sort("recent", books.Of(f.Year).Descending()).
	PageSize(20, 100).
	MaxPage(50).
	IndexedBy(sql.DerivedIndex).
	WithSearch(
		sql.NewSearch("title", sql.PrefixMatch, "title"),
		sql.NewSearch("words", sql.FullTextMatch, "title", "author"))

type storeEffect[A any] = effect.Effect[effect.Unit, sql.Fault, A]

// Store is the books of one database, spelled in its dialect.
type Store struct {
	database sql.Querier
	dialect  ddl.Dialect
}

// NewStore is the books of that database.
func NewStore(database sql.Querier, dialect ddl.Dialect) Store {
	return Store{database: database, dialect: dialect}
}

// CreateStatements make the table, the indexes the catalogue is read by, and
// what its searches need.
func CreateStatements(dialect ddl.Dialect) ([]string, error) {
	table := books.TableName()
	statements, err := ddl.Create(dialect, books.Structure())
	if err != nil {
		return nil, err
	}
	searches, err := ddl.CreateSearches(dialect, table, Catalogue.Searches()...)
	if err != nil {
		return nil, err
	}
	indexes, err := ddl.CreateIndexes(dialect, table, Catalogue.Indexes()...)
	if err != nil {
		return nil, err
	}
	return append(append(statements, indexes...), searches...), nil
}

// Save keeps the book, replacing one of the same ISBN.
func (store Store) Save(book domain.Book) storeEffect[domain.Book] {
	return books.Save[effect.Unit](store.database, store.dialect, book).As(book)
}

// Find is the book of that ISBN; a fault that is sql.ErrNoRows when there is
// none.
func (store Store) Find(isbn domain.ISBN) storeEffect[domain.Book] {
	return books.Find[effect.Unit](store.database, store.dialect, isbn)
}

// Remove removes the book of that ISBN, and says whether there was one.
func (store Store) Remove(isbn domain.ISBN) storeEffect[bool] {
	return books.Delete[effect.Unit](store.database, store.dialect, isbn).
		Map(func(outcome sql.Outcome) bool { return outcome.RowsAffected > 0 })
}

// Page is one page of the catalogue.
func (store Store) Page(query sql.PageQuery) storeEffect[sql.Page[domain.Book]] {
	return Catalogue.Page[effect.Unit](store.database, store.dialect, query)
}
