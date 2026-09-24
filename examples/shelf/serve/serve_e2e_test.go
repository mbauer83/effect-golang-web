package serve_test

// The catalogue serveCatalogue over HTTP from SQLite, as a client uses it. What
// matters is that one description of a book does every job: a request is result
// to the domain's rules and constructor, the document carries the API's names
// and not the table's, the table carries the mapping's, and the catalogue
// pages, sorts and searches as declared.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/mbauer83/effect-golang-sql/ddl"

	"github.com/mbauer83/effect-golang-web/examples/shelf/serve"
)

type client struct {
	t    *testing.T
	base string
}

func (c client) do(method string, path string, body string) (int, string) {
	c.t.Helper()
	request, err := http.NewRequest(method, c.base+path, strings.NewReader(body))
	if err != nil {
		c.t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		c.t.Fatal(err)
	}
	defer response.Body.Close()
	read, _ := io.ReadAll(response.Body)
	return response.StatusCode, string(read)
}

func book(isbn string, title string, author string, year int) string {
	return fmt.Sprintf(`{"isbn":%q,"title":%q,"author":%q,"pageCount":300,"year":%d,`+
		`"edition":{"format":"paperback","language":"en"}}`, isbn, title, author, year)
}

// serveCatalogue runs the catalogue on a fresh database until the test ends.
func serveCatalogue(t *testing.T) (client, string) {
	t.Helper()
	file := "file:" + t.TempDir() + "/shelf.db"
	// The test's own database: SQLite, in a file this test owns.
	return serveOn(t, serve.Database{Dialect: ddl.SQLite, Driver: "sqlite", Source: file}), file
}

// serveOn runs the catalogue over that database until the test ends.
func serveOn(t *testing.T, database serve.Database) client {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	within, stop := context.WithCancel(context.Background())
	finished := make(chan error, 1)
	go func() { finished <- serve.Serve(within, listener, database) }()
	t.Cleanup(func() {
		stop()
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Error("the server did not stop")
		}
	})
	c := client{t: t, base: "http://" + listener.Addr().String()}
	for attempt := 0; attempt < 50; attempt++ {
		if status, _ := c.do("GET", "/books", ""); status == http.StatusOK {
			return c
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the server did not start")
	return c
}

var shelf = []struct {
	isbn, title, author string
	year                int
}{
	{"9780156027328", "Solaris", "Stanislaw Lem", 2002},
	{"9780441013593", "Dune", "Frank Herbert", 2005},
	{"9780141439587", "Emma", "Jane Austen", 2003},
	{"9780307277671", "The Road", "Cormac McCarthy", 2007},
	{"9780679734529", "Solaris and Other Stories", "Stanislaw Lem", 1987},
}

func stockShelf(t *testing.T) (client, string) {
	c, file := serveCatalogue(t)
	for _, each := range shelf {
		if status, body := c.do("PUT", "/books/"+each.isbn, book(each.isbn, each.title, each.author, each.year)); status != http.StatusOK {
			t.Fatalf("saving %s: %d %s", each.title, status, body)
		}
	}
	return c, file
}

type page struct {
	Books []struct {
		ISBN      string `json:"isbn"`
		Title     string `json:"title"`
		PageCount int    `json:"pageCount"`
		Edition   struct {
			Format string `json:"format"`
		} `json:"edition"`
	} `json:"books"`
	Next     string `json:"next"`
	Previous string `json:"previous"`
}

func read(t *testing.T, c client, path string) page {
	t.Helper()
	status, body := c.do("GET", path, "")
	if status != http.StatusOK {
		t.Fatalf("%s: %d %s", path, status, body)
	}
	var result page
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Fatalf("%s: %v in %s", path, err, body)
	}
	return result
}

func titles(result page) string {
	names := make([]string, 0, len(result.Books))
	for _, each := range result.Books {
		names = append(names, each.Title)
	}
	return strings.Join(names, ", ")
}

func TestTheCatalogueIsOneDescriptionServed(t *testing.T) {
	c, file := stockShelf(t)

	// The document carries the API's names, and the edition as the object it
	// is; the shelf mark is the shop's business and is not sent.
	status, body := c.do("GET", "/books/9780441013593", "")
	if status != http.StatusOK || !strings.Contains(body, `"pageCount":300`) ||
		!strings.Contains(body, `"edition":{"format":"paperback","language":"en"}`) || strings.Contains(body, "shelf") {
		t.Errorf("expected the book as the API publishes it, got %d %s", status, body)
	}

	// The table carries the mapping's names: the ISBN's column named for the
	// number, snake_case, the edition flattened.
	database, err := sql.Open("sqlite", file)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var columns []string
	rows, err := database.Query(`SELECT name FROM pragma_table_info('book') ORDER BY cid`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var name string
		_ = rows.Scan(&name)
		columns = append(columns, name)
	}
	rows.Close()
	if got := strings.Join(columns, ","); !strings.HasPrefix(got, "isbn13,title,author,pages,year,edition_format,edition_language,shelf_mark") {
		t.Errorf("expected the mapping's columns, got %s", got)
	}

	// Pages by the declared sorts, continued by the cursor each page carries.
	first := read(t, c, "/books?size=2")
	second := read(t, c, "/books?size=2&after="+first.Next)
	if titles(first) != "Dune, Emma" || titles(second) != "Solaris, Solaris and Other Stories" {
		t.Errorf("expected the catalogue by title, two to a page, got [%s] then [%s]", titles(first), titles(second))
	}
	if back := read(t, c, "/books?size=2&before="+second.Previous); titles(back) != "Dune, Emma" {
		t.Errorf("expected the page before to be the first, got [%s]", titles(back))
	}
	if recent := read(t, c, "/books?sort=recent&size=1"); titles(recent) != "The Road" {
		t.Errorf("expected the most recent edition first, got [%s]", titles(recent))
	}

	// Searches, by the start of a title or by words anywhere.
	if matches := read(t, c, "/books?title=sol"); titles(matches) != "Solaris, Solaris and Other Stories" {
		t.Errorf("expected the titles beginning so, got [%s]", titles(matches))
	}
	if matches := read(t, c, "/books?q=lem+stories"); titles(matches) != "Solaris and Other Stories" {
		t.Errorf("expected the book holding both words, got [%s]", titles(matches))
	}
}

func TestARequestIsHeldToTheDomain(t *testing.T) {
	c, _ := stockShelf(t)
	cases := []struct{ name, path, body, says string }{
		{"a rule on a value", "/books/9780441013593", book("9780441013593", "", "Frank Herbert", 2005), "title"},
		{"the constructor's rule", "/books/9780441013594", book("9780441013594", "Dune", "Frank Herbert", 2005), "check digit"},
		{"a format the shop does not sell", "/books/9780441013593",
			strings.Replace(book("9780441013593", "Dune", "Frank Herbert", 2005), "paperback", "scroll", 1), "format"},
		{"another book's ISBN", "/books/9780141439587", book("9780441013593", "Dune", "Frank Herbert", 2005), "not the book's"},
	}
	for _, each := range cases {
		status, body := c.do("PUT", each.path, each.body)
		if status != http.StatusBadRequest || !strings.Contains(body, each.says) {
			t.Errorf("%s: expected it refused, saying %q; got %d %s", each.name, each.says, status, body)
		}
	}
	if status, body := c.do("GET", "/books?sort=price", ""); status != http.StatusBadRequest {
		t.Errorf("a sort not offered: expected it refused, got %d %s", status, body)
	}
	if status, body := c.do("GET", "/books?size=500", ""); status != http.StatusBadRequest {
		t.Errorf("a page larger than the largest: expected it refused, got %d %s", status, body)
	}
	if status, _ := c.do("DELETE", "/books/9780141439587", ""); status != http.StatusNoContent {
		t.Errorf("expected the book removed, got %d", status)
	}
	if status, _ := c.do("GET", "/books/9780141439587", ""); status != http.StatusNotFound {
		t.Errorf("expected a removed book not matches, got %d", status)
	}
}
