// Package domain is what a book is, said once.
//
// The schema below is the only description of a book in the program. The store
// maps it to a table and the API publishes a projection of it; each says only
// where it differs, and every rule about a value -- a title's length, the
// pattern of an ISBN -- is stated here and nowhere else, so the table's checkDigitHolds
// and the API's refusals are this one rule.
package domain

import (
	"errors"
	"fmt"

	"github.com/mbauer83/effect-golang-schema/schema"
)

// ISBN is a book's identity: the thirteen digits of its International Standard
// Book Number, without hyphens.
type ISBN string

// Edition is which form a book takes: a value with no identity of its own, so a
// table keeps it as columns of the book that holds it.
type Edition struct {
	Format   string
	Language string
}

// Book is one book. Its members are its own: a book is made by NewBook, or
// decoded -- which is NewBook too -- so every book is one whose ISBN checkDigitHolds.
type Book struct {
	isbn      ISBN
	title     string
	author    string
	pages     int
	year      int
	edition   Edition
	shelfMark string
}

// NewBook is a book, refused when its ISBN's check digit does not hold: a rule
// about the whole number, which no constraint on its characters can state.
func NewBook(isbn ISBN, title string, author string, pages int, year int, edition Edition, shelfMark string) (Book, error) {
	if !checkDigitHolds(isbn) {
		return Book{}, fmt.Errorf("%w: %s", ErrCheckDigit, isbn)
	}
	return Book{isbn: isbn, title: title, author: author, pages: pages, year: year, edition: edition, shelfMark: shelfMark}, nil
}

// ErrCheckDigit is an ISBN whose last digit is not the one its others make.
var ErrCheckDigit = errors.New("the ISBN's check digit does not hold")

func (book Book) ISBN() ISBN        { return book.isbn }
func (book Book) Title() string     { return book.title }
func (book Book) Author() string    { return book.author }
func (book Book) Pages() int        { return book.pages }
func (book Book) Year() int         { return book.year }
func (book Book) Edition() Edition  { return book.edition }
func (book Book) ShelfMark() string { return book.shelfMark }

// checkDigitHolds says the ISBN-13 check digit holds: the digits weighted 1, 3, 1, 3 ...
// sum to a multiple of ten.
func checkDigitHolds(isbn ISBN) bool {
	if len(isbn) != 13 {
		return false
	}
	sum := 0
	for at, digit := range []byte(isbn) {
		weight := 1
		if at%2 == 1 {
			weight = 3
		}
		sum += int(digit-'0') * weight
	}
	return sum%10 == 0
}

// BookFields are a book's members, as every projection of one names them.
var BookFields = struct {
	ISBN      schema.Field[Book, ISBN]
	Title     schema.Field[Book, string]
	Author    schema.Field[Book, string]
	Pages     schema.Field[Book, int]
	Year      schema.Field[Book, int]
	Edition   schema.Field[Book, Edition]
	ShelfMark schema.Field[Book, string]
}{
	ISBN:    schema.FieldOf("isbn", ISBNSchema, Book.ISBN).Identity(),
	Title:   schema.FieldOf("title", schema.Text().Check(schema.MinLength(1), schema.MaxLength(200)), Book.Title),
	Author:  schema.FieldOf("author", schema.Text().Check(schema.MinLength(1), schema.MaxLength(100)), Book.Author),
	Pages:   schema.FieldOf("pages", count(1), Book.Pages),
	Year:    schema.FieldOf("year", count(1450), Book.Year).WithDescription("the year this edition was published"),
	Edition: schema.FieldOf("edition", EditionSchema, Book.Edition),
	ShelfMark: schema.FieldOf("shelfMark", schema.Text().Check(schema.MaxLength(16)), Book.ShelfMark).
		WithDescription("where the shop keeps it"),
}

// BookSchema describes a book, and is the only description of one. A book
// decodes through NewBook, so what decodes -- from a row or a request -- is a
// book whose ISBN checkDigitHolds.
var BookSchema = schema.Object("book", func(values schema.Values) (Book, error) {
	f := BookFields
	return NewBook(f.ISBN.Of(values), f.Title.Of(values), f.Author.Of(values), f.Pages.Of(values),
		f.Year.Of(values), f.Edition.Of(values), f.ShelfMark.Of(values))
}, BookFields.ISBN, BookFields.Title, BookFields.Author, BookFields.Pages, BookFields.Year,
	BookFields.Edition, BookFields.ShelfMark,
).WithDescription("one book the shop sells")

// EditionSchema is an edition: a format the shop sells, and a language tag.
var EditionSchema = schema.Struct[Edition]("edition",
	schema.FieldAt("format", schema.Text().Check(schema.Pattern(`^(hardback|paperback|ebook)$`)),
		func(edition *Edition) *string { return &edition.Format }),
	schema.FieldAt("language", schema.Text().Check(schema.MinLength(2), schema.MaxLength(8)),
		func(edition *Edition) *string { return &edition.Language }))

// ISBNSchema is an ISBN as thirteen digits.
var ISBNSchema = schema.Transform(schema.Text().Check(schema.Pattern(`^[0-9]{13}$`)),
	func(isbn string) ISBN { return ISBN(isbn) },
	func(isbn ISBN) string { return string(isbn) })

// count is a whole number of at least least, kept in 32 bits.
func count(least int32) schema.Schema[int] {
	return schema.Transform(schema.Int32().Check(schema.AtLeast(least)),
		func(value int32) int { return int(value) },
		func(value int) int32 { return int32(value) })
}
