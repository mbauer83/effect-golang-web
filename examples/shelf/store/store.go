// Package store keeps books in a relational database.
//
// It says only where the table differs from the domain: the ISBN's column is
// named for the number it holds. Everything else follows -- the columns in
// snake_case, the edition flattened into edition_format and
// edition_language, a check for every rule the domain states, and a unique
// key on the identity -- and the listing declares what a reader may ask of
// the catalogue: two sorts, two searches, and pages of a size.
//
// Declared, not connected: every operation is an effect that requires the
// sql.Session it runs against, which a program supplies where it is wired.
package store

import (
	"github.com/mbauer83/effect-golang-sql/ddl"
	"github.com/mbauer83/effect-golang-sql/sql"

	"github.com/mbauer83/effect-golang-web/examples/shelf/domain"
)

var f = domain.BookFields

// Books is where books are kept, found by their ISBN.
var Books = sql.NewRepository(sql.Map(domain.BookSchema).Column(f.ISBN, "isbn13"), f.ISBN.Shape())

// Catalogue is the books read a page at a time: by title unless a reader asks
// for the most recent editions first, found by the start of a title or by
// words anywhere in the title or the author, twenty to a page unless asked.
var Catalogue = Books.Listing().
	Sort("title", Books.Of(f.Title).Ascending()).
	Sort("recent", Books.Of(f.Year).Descending()).
	PageSize(20, 100).
	MaxPage(50).
	IndexedBy(sql.DerivedIndex).
	WithSearch(
		sql.NewSearch("title", sql.PrefixMatch, "title"),
		sql.NewSearch("words", sql.FullTextMatch, "title", "author"))

// CreateStatements make the table, the indexes the catalogue is read by, and
// what its searches need.
func CreateStatements(dialect ddl.Dialect) ([]string, error) {
	table := Books.TableName()
	statements, err := ddl.Create(dialect, Books.Structure())
	if err != nil {
		return nil, err
	}
	indexes, err := ddl.CreateIndexes(dialect, table, Catalogue.Indexes()...)
	if err != nil {
		return nil, err
	}
	searches, err := ddl.CreateSearches(dialect, table, Catalogue.Searches()...)
	if err != nil {
		return nil, err
	}
	return append(append(statements, indexes...), searches...), nil
}
