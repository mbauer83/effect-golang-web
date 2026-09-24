package serve_test

// The catalogue served from Postgres: the program the other tests serve from
// SQLite, given a deployment's database instead.

import (
	"database/sql"
	"net/http"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/mbauer83/effect-golang-sql/ddl"

	"github.com/mbauer83/effect-golang-web/examples/shelf/serve"
)

// The same program given a deployment's database: Postgres, with the table a
// run before this one left dropped first.
func TestTheCatalogueIsServedFromPostgres(t *testing.T) {
	address := os.Getenv("SHELF_POSTGRES_URL")
	if address == "" {
		t.Skip("set SHELF_POSTGRES_URL to serve the catalogue from a real postgres")
	}
	reset, err := sql.Open("pgx", address)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reset.Exec(`DROP TABLE IF EXISTS "book"`); err != nil {
		t.Fatal(err)
	}
	reset.Close()
	c := serveOn(t, serve.Database{Dialect: ddl.Postgres, Driver: "pgx", Source: address})
	for _, each := range shelf {
		if status, body := c.do("PUT", "/books/"+each.isbn, book(each.isbn, each.title, each.author, each.year)); status != http.StatusOK {
			t.Fatalf("saving %s: %d %s", each.title, status, body)
		}
	}
	if found := read(t, c, "/books?q=lem+stories"); titles(found) != "Solaris and Other Stories" {
		t.Errorf("expected the book holding both words, got [%s]", titles(found))
	}
	if found := read(t, c, "/books?title=sol&size=1"); titles(found) != "Solaris" || found.Next == "" {
		t.Errorf("expected the first title beginning so, and a page after it; got [%s]", titles(found))
	}
}
