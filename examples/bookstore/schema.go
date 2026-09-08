package bookstore

import "github.com/mbauer83/effect-golang-web/schema"

// BookSchema is generated from the Book struct; see schema_generated.go.
//
//go:generate go run github.com/mbauer83/effect-golang-web/cmd/schemagen -package .

// CatalogueSchema describes the collection as a document, so a list response is
// described rather than assembled.
var CatalogueSchema = schema.List(BookSchema)

// pagesSchema is the domain's rule, enforced where the shape is described so no
// handler has to repeat it.
//
// It is written by hand and the struct points at it with a use= tag, because a
// refinement is not something a generator can infer: an int is not the same
// thing as a page count, and only the domain knows the difference.
var pagesSchema = schema.TransformOrFail(schema.Int(),
	func(pages int) (int, error) {
		if pages < 1 {
			return 0, errBlankBook
		}
		return pages, nil
	},
	func(pages int) (int, error) { return pages, nil },
)
