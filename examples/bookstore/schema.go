package bookstore

import "github.com/mbauer83/effect-golang-web/schema"

// BookSchema describes one entry. The request decoder, the response encoder and
// the published contract are all projections of it.
var BookSchema = schema.Struct[Book]("Book",
	schema.FieldOf("title", schema.Text(),
		func(book Book) string { return book.Title },
		func(book *Book, title string) { book.Title = title }),
	schema.FieldOf("authors", schema.List(schema.Text()),
		func(book Book) []string { return book.Authors },
		func(book *Book, authors []string) { book.Authors = authors }),
	schema.FieldOf("pages", pagesSchema,
		func(book Book) int { return book.Pages },
		func(book *Book, pages int) { book.Pages = pages }),
)

// CatalogueSchema describes the collection as a document, so a list response is
// described rather than assembled.
var CatalogueSchema = schema.List(BookSchema)

// pagesSchema is the domain's rule, enforced where the shape is described so no
// handler has to repeat it.
var pagesSchema = schema.TransformOrFail(schema.Int(),
	func(pages int) (int, error) {
		if pages < 1 {
			return 0, errBlankBook
		}
		return pages, nil
	},
	func(pages int) (int, error) { return pages, nil },
)
