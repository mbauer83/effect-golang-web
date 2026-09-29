package unit

// Reading a request into a struct, and what a web fault says it is.

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mbauer83/effect-golang-schema/schema"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect/fault"
)

type search struct {
	Text  string
	Page  *int
	Trace *string
}

func searchCodec() web.Codec[search] {
	return web.Struct(
		web.FieldOf(web.QueryParam("q", schema.Text()), func(s *search, q string) { s.Text = q }),
		web.FieldOf(web.OptionalQueryParam("page", schema.Int()), func(s *search, page *int) { s.Page = page }),
		web.FieldOf(web.OptionalHeaderParam("X-Trace", schema.Text()).WithDescription("a correlation id"),
			func(s *search, trace *string) { s.Trace = trace }),
	)
}

func TestAStructCodecReadsEachFieldFromItsPlace(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/films?q=alien&page=2", nil)
	request.Header.Set("X-Trace", "abc")
	found, err := read(t, searchCodec(), request, nil)
	if err != nil || found.Text != "alien" || found.Page == nil || *found.Page != 2 ||
		found.Trace == nil || *found.Trace != "abc" {
		t.Fatalf("expected every field read, got %+v, %v", found, err)
	}
	absent, err := read(t, searchCodec(), httptest.NewRequest(http.MethodGet, "/films?q=alien", nil), nil)
	if err != nil || absent.Page != nil || absent.Trace != nil {
		t.Errorf("expected absent optional fields to stay nil, got %+v, %v", absent, err)
	}
}

func TestAStructCodecDeclaresEveryField(t *testing.T) {
	parameters := searchCodec().Parameters()
	if len(parameters) != 3 || parameters[0].Name != "q" || !parameters[0].Required ||
		parameters[2].In != web.InHeader || parameters[2].Description != "a correlation id" {
		t.Errorf("expected the three fields declared in order, got %+v", parameters)
	}
}

func TestAStructCodecRefusesTwoFieldsReadingOneParameter(t *testing.T) {
	twice := web.Struct(
		web.FieldOf(web.QueryParam("q", schema.Text()), func(s *search, q string) { s.Text = q }),
		web.FieldOf(web.QueryParam("q", schema.Text()), func(s *search, q string) { s.Text = q }),
	)
	if err := web.ValidateCodec(twice); err == nil || fault.KindOf(err) != fault.Unreadable {
		t.Errorf("expected a declaration mistake, got %v", err)
	}
}

func TestARequestMissingARequiredFieldIsUnacceptable(t *testing.T) {
	_, err := read(t, searchCodec(), httptest.NewRequest(http.MethodGet, "/films", nil), nil)
	if fault.KindOf(err) != fault.Unacceptable {
		t.Errorf("expected unacceptable, got %v (%s)", err, fault.KindOf(err))
	}
	_, err = read(t, searchCodec(), httptest.NewRequest(http.MethodGet, "/films?q=a&page=two", nil), nil)
	if fault.KindOf(err) != fault.Unreadable {
		t.Errorf("expected a value that does not decode to be unreadable, got %s", fault.KindOf(err))
	}
}

func TestARefusalIsTheKindItsStatusSays(t *testing.T) {
	for status, kind := range map[int]fault.Kind{
		http.StatusNotFound: fault.Missing, http.StatusTooManyRequests: fault.Unavailable,
		http.StatusBadGateway: fault.Unavailable, http.StatusForbidden: fault.Unacceptable,
	} {
		failure := web.Fault{Op: "call the catalog", Err: web.Refusal{Status: status}}
		if failure.Kind() != kind {
			t.Errorf("%d: expected %s, got %s", status, kind, failure.Kind())
		}
	}
	if (web.Fault{Op: "call the catalog", Err: errors.New("connection refused")}).Kind() != fault.Unavailable {
		t.Error("expected a connection that failed to be unavailable")
	}
}

func TestAnAbsentParameterReadsAsItsFallback(t *testing.T) {
	page := web.OrElse(web.OptionalQueryParam("page", schema.Int()), 1)
	absent, err := read(t, page, httptest.NewRequest(http.MethodGet, "/films", nil), nil)
	given, _ := read(t, page, httptest.NewRequest(http.MethodGet, "/films?page=3", nil), nil)
	if err != nil || absent != 1 || given != 3 || page.Parameters()[0].Required {
		t.Errorf("expected 1 when absent and 3 when given, optional in the document; got %d, %d, %v", absent, given, err)
	}
}
