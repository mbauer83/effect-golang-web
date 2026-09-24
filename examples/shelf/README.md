# shelf

A catalogue of books served over HTTP from a relational database, from one
description of a book.

- `domain` is what a book is: the aggregate, its fields and its rules, once.
  A book is made by its constructor, which is also what a row or a request
  decodes through.
- `store` says only where the table differs: the ISBN's column is `isbn13`.
  The rest follows: snake_case columns, the edition flattened into
  `edition_format` and `edition_language`, a check for each of the domain's
  rules, and a listing with two sorts, two searches, and page sizes.
- `api` says only where the API differs: the shelf mark is not published and
  the page count is `pageCount`. Documents are camelCase and the edition stays
  an object.
- `serve` runs it. `serve_e2e_test.go` shows each of the above from a client's
  side.

A nested module, so that effect-golang-web does not depend on
effect-golang-sql. It requires the released effect-golang-sql v0.6.0 and
effect-golang-web v0.6.0.

```sh
go run ./cmd/shelf        # :8080, or SHELF_ADDRESS
curl -X PUT localhost:8080/books/9780441013593 -d '{"isbn":"9780441013593","title":"Dune","author":"Frank Herbert","pageCount":412,"year":2005,"edition":{"format":"paperback","language":"en"}}'
curl 'localhost:8080/books?q=herbert&size=10'
```
