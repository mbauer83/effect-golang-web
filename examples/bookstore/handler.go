package bookstore

import (
	"context"
	"net/http"
	"strings"

	"github.com/mbauer83/effect-golang-web/schema"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

type storeEffect[A any] = effect.Effect[effect.Unit, Fault, A]

// Handler is the bookstore's handler. It dispatches on the method and the path,
// which is what the routing step exists to replace.
func Handler(store *Store) web.Handler[effect.Unit, Fault] {
	return func(request web.Request) storeEffect[web.Response] {
		switch {
		case request.Method() == http.MethodGet && request.Path() == "/books":
			return list(store)
		case request.Method() == http.MethodPost && request.Path() == "/books":
			return add(store, request)
		case request.Method() == http.MethodGet:
			return find(store, request.Path())
		default:
			return effect.For[effect.Unit, Fault]().
				Succeed(web.Empty(http.StatusMethodNotAllowed))
		}
	}
}

// list answers with the whole collection, described by the same schema that
// decodes one.
func list(store *Store) storeEffect[web.Response] {
	return responding(http.StatusOK, CatalogueSchema, store.All()).Named("list-books")
}

// add decodes an entry and stores it. The decode is a stage in the same failure
// channel as the read, because a client that sent a bad entity and a client
// whose connection died are both the request's problem rather than the store's.
func add(store *Store, request web.Request) storeEffect[web.Response] {
	return reading(request).
		FlatMap(func(document []byte) storeEffect[Book] { return decoding(document) }).
		FlatMap(func(book Book) storeEffect[web.Response] {
			store.Add(book)
			return responding(http.StatusCreated, BookSchema, book)
		}).
		Named("add-book")
}

// find answers with one entry, or refuses in the application's own vocabulary
// rather than choosing a status.
func find(store *Store, path string) storeEffect[web.Response] {
	title, addressed := strings.CutPrefix(path, "/books/")
	if !addressed || title == "" {
		return missing()
	}
	book, held := store.Find(title)
	if !held {
		return missing()
	}
	return responding(http.StatusOK, BookSchema, book).Named("find-book")
}

func missing() storeEffect[web.Response] {
	return effect.For[effect.Unit, Fault]().Fail[web.Response](Fault{Kind: NotFound})
}

func reading(request web.Request) storeEffect[[]byte] {
	return web.Body[effect.Unit](request).
		MapError(func(fault web.Fault) Fault { return Fault{Kind: Unreadable, Err: fault} })
}

func decoding(document []byte) storeEffect[Book] {
	return effect.Try(
		func(context.Context, effect.Unit) (Book, error) {
			return schema.DecodeJSON(BookSchema, document)
		},
		func(err error) Fault { return Fault{Kind: Unacceptable, Err: err} },
	).Named("decode-book")
}

// responding encodes a value through its schema. An encoding failure is a
// defect rather than a typed failure: the value came from this program, so a
// schema that cannot describe it is a mistake here and not something a client
// can be told about.
func responding[A any](status int, shape schema.Schema[A], value A) storeEffect[web.Response] {
	return effect.From(func(context.Context, effect.Unit) effect.Exit[Fault, web.Response] {
		response, err := web.JSON(status, shape, value)
		if err != nil {
			return effect.ExitCause[Fault, web.Response](
				effect.DieCause[Fault](effect.Defect{Value: err}),
			)
		}
		return effect.ExitSuccess[Fault](response)
	})
}
