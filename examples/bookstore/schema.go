package bookstore

import "github.com/mbauer83/effect-golang-web/schema"

// CatalogueSchema describes the collection as a document, so a list response is
// described rather than assembled.
var CatalogueSchema = schema.List(BookSchema)
