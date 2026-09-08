package bookstore

import (
	"net/http"

	"github.com/mbauer83/effect-golang-schema/schema"
	"github.com/mbauer83/effect-golang-web/openapi"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

type storeEffect[A any] = effect.Effect[effect.Unit, Fault, A]

// Surface assembles the bookstore's routes.
//
// Two routes that could match the same request, a path parameter the path does
// not capture, or a schema that cannot encode what it says it will are all
// reported here, at start-up, rather than by a request in production.
func Surface(store *Store) (web.Routes[effect.Unit, Fault], error) {
	return web.NewRoutes(routes(store)...)
}

// Published is the surface together with the route that serves its contract.
//
// The contract describes the API's routes and not the route that serves it,
// which is the ordinary arrangement: a client reads the document to learn what
// it may ask for, and it already knows where the document is.
func Published(store *Store) (web.Routes[effect.Unit, Fault], error) {
	described, err := Surface(store)
	if err != nil {
		return web.Routes[effect.Unit, Fault]{}, err
	}
	contract, err := Contract(described)
	if err != nil {
		return web.Routes[effect.Unit, Fault]{}, err
	}
	return web.NewRoutes(append(routes(store), publishing(contract))...)
}

// Contract projects the surface into an OpenAPI document.
//
// Nothing is written twice: the parameters, the entity and the response shapes
// come from the same declarations that dispatch a request.
func Contract(surface web.Routes[effect.Unit, Fault]) ([]byte, error) {
	return openapi.Describe(
		openapi.Info{
			Title:       "Bookstore",
			Version:     "1.0.0",
			Description: "A small catalogue, served from one description.",
		},
		surface.Declarations(),
	).Render()
}

func routes(store *Store) []web.Route[effect.Unit, Fault] {
	return []web.Route[effect.Unit, Fault]{
		listBooks(store),
		addBook(store),
		findBook(store),
	}
}

// publishing serves the contract as it stands. Its entity is already encoded,
// so it is answered as the bytes it is rather than through a schema.
func publishing(contract []byte) web.Route[effect.Unit, Fault] {
	return web.Handle(
		web.GET("/openapi.json", web.Nothing(),
			web.ReturnsRaw(http.StatusOK, "application/json")).
			Summary("The contract this API is served from"),
		func(effect.Unit) storeEffect[[]byte] {
			return effect.For[effect.Unit, Fault]().Succeed(contract)
		},
	)
}

// The three declarations, as values.
//
// Exported because a declaration is not only the server's. It dispatches a
// request, it projects into the contract above, and a client calls it -- which
// is the whole reason an Endpoint is separate from its handler, and is only
// demonstrable if the same value is reachable from both sides.
var (
	// ListBooks reads the catalogue.
	ListBooks = web.GET("/books", web.Nothing(),
		web.Returns(http.StatusOK, CatalogueSchema)).
		Summary("List the catalogue").
		Describe("Every entry, in the order the store holds them.")

	// AddBook adds one, identified by its title.
	AddBook = web.POST("/books", web.Entity(BookSchema),
		web.Returns(http.StatusCreated, BookSchema)).
		Summary("Add a book").
		Describe("The entity is the book to add; its title identifies it.").
		Failing(http.StatusConflict, "the store already holds that title")

	// FindBook reads one by title.
	FindBook = web.GET("/books/{title}",
		web.PathParam("title", schema.Text()).Documented("the title to look for"),
		web.Returns(http.StatusOK, BookSchema)).
		Summary("Find a book by title").
		Failing(http.StatusNotFound, "no book with that title is held")
)

func listBooks(store *Store) web.Route[effect.Unit, Fault] {
	return web.Handle(ListBooks, func(effect.Unit) storeEffect[[]Book] { return store.All() })
}

func addBook(store *Store) web.Route[effect.Unit, Fault] {
	return web.Handle(AddBook,
		func(book Book) storeEffect[Book] { return store.Add(book).As(book) })
}

func findBook(store *Store) web.Route[effect.Unit, Fault] {
	return web.Handle(FindBook, store.Find)
}
