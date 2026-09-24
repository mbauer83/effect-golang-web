package unit

// A page of a list, as a client asks for one. What matters is that the query
// reads as the request it is, that a sort the list does not offer or two
// positions at once is the client's mistake, and that the declaration says
// what may be asked.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mbauer83/effect-golang-schema/schema"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

func pageRoute() web.Route[effect.Unit, Refusal] {
	return web.Handle(
		web.Declare(http.MethodGet, "/books", web.PageParams("recent", "title"), web.Returns(http.StatusOK, schema.Text())),
		func(request web.PageRequest) webEffect[string] {
			return effect.For[effect.Unit, Refusal]().Succeed(fmt.Sprintf("%s|%s|%s|%d|%d",
				request.Sort, request.After, request.Before, request.Number, request.Size))
		})
}

func TestAPageRequestReadsAsTheRequestItIs(t *testing.T) {
	received := dispatched(t, httptest.NewRequest(http.MethodGet, "/books?sort=title&after=c1&size=5", nil), pageRoute())
	if body := answered(t, received); body != `"title|c1||0|5"` {
		t.Fatalf("expected the request as asked, got %s", body)
	}
}

func TestAPageTheListDoesNotOfferIsTheClientsMistake(t *testing.T) {
	for _, query := range []string{"sort=year", "after=c1&page=2", "size=0"} {
		received := dispatched(t, httptest.NewRequest(http.MethodGet, "/books?"+query, nil), pageRoute())
		if received.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: expected a refusal, got %d", query, received.StatusCode)
		}
	}
}

func TestAPagesParametersAreDeclared(t *testing.T) {
	names := map[string]bool{}
	for _, parameter := range pageRoute().Declaration().Parameters {
		names[parameter.Name] = true
	}
	for _, name := range []string{"sort", "after", "before", "page", "size"} {
		if !names[name] {
			t.Errorf("expected %s declared, got %v", name, names)
		}
	}
}
