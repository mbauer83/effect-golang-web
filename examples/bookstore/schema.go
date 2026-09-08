package bookstore

import "github.com/mbauer83/effect-golang-web/schema"

// BookSchema is generated from the Book struct; see schema_generated.go.
//
//go:generate go run github.com/mbauer83/effect-golang-web/cmd/schemagen -package .

// CatalogueSchema describes the collection as a document, so a list response is
// described rather than assembled.
var CatalogueSchema = schema.List(BookSchema)
