// Package api serves the catalogue over HTTP.
//
// A book is published as the domain's own description, projected: without the
// shelf mark, which is the shop's business, and with the page count under the
// name clients know it by. Nothing else is restated -- the edition is sent as
// the object it is, the rules a request is held to are the domain's, and a
// book decoded from a request is made by the domain's constructor.
package api

import (
	"errors"
	"net/http"

	"github.com/mbauer83/effect-golang-schema/schema"
	"github.com/mbauer83/effect-golang-schema/schema/naming"
	"github.com/mbauer83/effect-golang-sql/sql"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"

	"github.com/mbauer83/effect-golang-web/examples/shelf/domain"
	"github.com/mbauer83/effect-golang-web/examples/shelf/store"
)

var f = domain.BookFields

// BookShape is a book as a client sees it.
var BookShape = domain.BookSchema.
	Omit(f.ShelfMark).
	Rename(f.Pages, "pageCount").
	Describe(f.ISBN, "the thirteen digits of the ISBN, without hyphens").
	WithDescription("a book the shop sells")

// BookPage is one page of the catalogue, with the cursors that continue it.
type BookPage struct {
	Books    []domain.Book
	Next     string
	Previous string
}

// BookPageShape is a page as a client sees it: the books, and a cursor each
// way, empty at either end.
var BookPageShape = schema.Struct[BookPage]("bookPage",
	schema.FieldAt("books", schema.List(BookShape), func(page *BookPage) *[]domain.Book { return &page.Books }),
	schema.FieldAt("next", schema.Text(), func(page *BookPage) *string { return &page.Next }),
	schema.FieldAt("previous", schema.Text(), func(page *BookPage) *string { return &page.Previous }))

// CatalogueQuery is what a client asks of the catalogue: a page, and what to search
// for.
type CatalogueQuery struct {
	Page  web.PageRequest
	Words string
	Title string
}

var catalogueQuery = web.Convert(
	web.Zip(web.PageParams("title", "recent"), web.Zip(
		web.OptionalQueryParam("q", schema.Text()).WithDescription("words the title or author holds"),
		web.OptionalQueryParam("title", schema.Text()).WithDescription("what the title begins with"))),
	func(params effect.Product[web.PageRequest, effect.Product[*string, *string]]) (CatalogueQuery, error) {
		valueOf := func(value *string) string {
			if value == nil {
				return ""
			}
			return *value
		}
		return CatalogueQuery{Page: params.First, Words: valueOf(params.Second.First), Title: valueOf(params.Second.Second)}, nil
	})

var isbn = web.PathParam("isbn", domain.ISBNSchema).WithDescription("the book's ISBN")

// The endpoints, as values a server dispatches and a client calls.
var (
	ListBooks = web.GET("/books", catalogueQuery, web.Returns(http.StatusOK, BookPageShape)).
			WithSummary("Read the catalogue a page at a time")
	SaveBook = web.PUT("/books/{isbn}", web.Zip(isbn, web.Entity(BookShape)), web.Returns(http.StatusOK, BookShape)).
			WithSummary("Keep a book, replacing one of the same ISBN").
			WithFailure(http.StatusBadRequest, "the ISBN in the path is not the book's")
	FindBook = web.GET("/books/{isbn}", isbn, web.Returns(http.StatusOK, BookShape)).
			WithSummary("Find a book by its ISBN").
			WithFailure(http.StatusNotFound, "no book of that ISBN is kept")
	RemoveBook = web.DELETE("/books/{isbn}", isbn, web.ReturnsNothing(http.StatusNoContent)).
			WithSummary("Remove a book").
			WithFailure(http.StatusNotFound, "no book of that ISBN is kept")
)

type apiEffect[A any] = effect.Effect[effect.Unit, Fault, A]

// Surface is the catalogue's routes, sending and reading documents in
// camelCase.
func Surface(books store.Store) (web.Routes[effect.Unit, Fault], error) {
	routes, err := web.NewRoutes(
		web.Handle(ListBooks, func(query CatalogueQuery) apiEffect[BookPage] {
			return withFault(books.Page(pageQuery(query))).Map(func(page sql.Page[domain.Book]) BookPage {
				return BookPage{Books: page.Items, Next: string(page.Next), Previous: string(page.Previous)}
			})
		}),
		web.Handle(SaveBook, func(valueOf effect.Product[domain.ISBN, domain.Book]) apiEffect[domain.Book] {
			if valueOf.First != valueOf.Second.ISBN() {
				return effect.For[effect.Unit, Fault]().Fail[domain.Book](Fault{Kind: Refused, Err: errors.New("the ISBN in the path is not the book's")})
			}
			return withFault(books.Save(valueOf.Second))
		}),
		web.Handle(FindBook, func(isbn domain.ISBN) apiEffect[domain.Book] { return withFault(books.Find(isbn)) }),
		web.Handle(RemoveBook, func(isbn domain.ISBN) apiEffect[effect.Unit] {
			return withFault(books.Remove(isbn)).FlatMap(func(removed bool) apiEffect[effect.Unit] {
				if !removed {
					return effect.For[effect.Unit, Fault]().Fail[effect.Unit](Fault{Kind: NotFound})
				}
				return effect.For[effect.Unit, Fault]().Succeed(effect.Unit{})
			})
		}),
	)
	return routes.WithNaming(naming.CamelCase), err
}

// pageQuery is a client's request as the catalogue reads it.
func pageQuery(query CatalogueQuery) sql.PageQuery {
	return sql.PageQuery{
		Where: sql.And(store.Catalogue.Match("words", query.Words), store.Catalogue.Match("title", query.Title)),
		Sort:  query.Page.Sort, Size: query.Page.Size, Number: query.Page.Number,
		After: sql.PageCursor(query.Page.After), Before: sql.PageCursor(query.Page.Before),
	}
}
