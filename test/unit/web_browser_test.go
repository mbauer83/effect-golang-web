package unit

// What a browser acts on: a redirect, a cookie kept, a cookie dropped, and an
// answer whose value decides all three. The claims that matter are that a
// second cookie does not replace the first, that a cookie net/http would not
// write is refused rather than dropped, and that a rendered answer is
// documented with every status it can take.

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

func TestARedirectIsFoundAndKeepsItsLocationWhateverItsStatus(t *testing.T) {
	received := sent(t, web.Redirect("/login"))
	if received.StatusCode != http.StatusFound || received.Header.Get("Location") != "/login" {
		t.Fatalf("expected 302 to /login, got %d to %q",
			received.StatusCode, received.Header.Get("Location"))
	}

	seeOther := sent(t, web.Redirect("/done").WithStatus(http.StatusSeeOther))
	if seeOther.StatusCode != http.StatusSeeOther || seeOther.Header.Get("Location") != "/done" {
		t.Fatalf("expected 303 to /done, got %d to %q",
			seeOther.StatusCode, seeOther.Header.Get("Location"))
	}
}

func TestASecondCookieIsAddedBesideTheFirstAndTheOriginalIsLeftAlone(t *testing.T) {
	original := web.Redirect("/")
	first, err := original.WithCookie(&http.Cookie{Name: "session", Value: "one"})
	if err != nil {
		t.Fatal(err)
	}
	both, err := first.WithCookie(&http.Cookie{Name: "theme", Value: "dark"})
	if err != nil {
		t.Fatal(err)
	}

	if got := sent(t, both).Header.Values("Set-Cookie"); len(got) != 2 {
		t.Fatalf("expected two Set-Cookie lines, got %q", got)
	}
	if got := original.Header().Values("Set-Cookie"); len(got) != 0 {
		t.Fatalf("expected the original untouched, got %q", got)
	}
	if got := first.Header().Values("Set-Cookie"); len(got) != 1 {
		t.Fatalf("expected the first derivation untouched, got %q", got)
	}
}

func TestACookieNetHTTPWouldNotWriteIsRefusedRatherThanDropped(t *testing.T) {
	_, err := web.Empty(http.StatusOK).WithCookie(&http.Cookie{Name: "no spaces allowed", Value: "x"})
	if !errors.Is(err, web.ErrUnwritableCookie) {
		t.Fatalf("expected ErrUnwritableCookie, got %v", err)
	}
	if _, err := web.Empty(http.StatusOK).WithCookie(nil); !errors.Is(err, web.ErrUnwritableCookie) {
		t.Fatalf("expected a nil cookie refused, got %v", err)
	}
}

func TestAnExpiredCookieIsNamedAsItWasSetAndCarriesNoValue(t *testing.T) {
	expired, err := web.Empty(http.StatusOK).ExpireCookie(
		http.Cookie{Name: "session", Value: "secret", Path: "/", HttpOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	line := sent(t, expired).Header.Get("Set-Cookie")
	for _, part := range []string{"session=;", "Path=/", "Max-Age=0", "HttpOnly"} {
		if !strings.Contains(line, part) {
			t.Fatalf("expected %q in %q", part, line)
		}
	}
	if strings.Contains(line, "secret") {
		t.Fatalf("expected the value dropped, got %q", line)
	}
}

// signIn is a value that renders as a redirect with a cookie, which is the
// shape a sign-in's last hop has.
type signIn struct {
	destination string
	session     string
}

func renderSignIn(answer signIn) (web.Response, error) {
	return web.Redirect(answer.destination).
		WithCookie(&http.Cookie{Name: "session", Value: answer.session, Path: "/"})
}

func signInRoute() web.Route[effect.Unit, Refusal] {
	return web.Handle(
		web.GET("/callback", web.Nothing(),
			web.ReturnsResponse(http.StatusFound, "text/plain", renderSignIn)).
			WithAlternative(http.StatusSeeOther, "when the flow was started by a form"),
		func(effect.Unit) webEffect[signIn] {
			return effect.For[effect.Unit, Refusal]().
				Succeed(signIn{destination: "/parts", session: "abc"})
		},
	)
}

func TestARenderedAnswerIsTheResponseItsValueRendersAs(t *testing.T) {
	received := dispatched(t, httptest.NewRequest(http.MethodGet, "/callback", nil), signInRoute())

	if received.StatusCode != http.StatusFound || received.Header.Get("Location") != "/parts" {
		t.Fatalf("expected 302 to /parts, got %d to %q",
			received.StatusCode, received.Header.Get("Location"))
	}
	if got := received.Header.Get("Set-Cookie"); !strings.HasPrefix(got, "session=abc") {
		t.Fatalf("expected the session cookie, got %q", got)
	}
}

func TestAnOutputThatDoesNotSayHowToRenderIsRefusedAtAssembly(t *testing.T) {
	route := web.Handle(
		web.GET("/nowhere", web.Nothing(), web.ReturnsResponse[signIn](http.StatusOK, "text/html", nil)),
		func(effect.Unit) webEffect[signIn] {
			return effect.For[effect.Unit, Refusal]().Succeed(signIn{})
		},
	)
	if err := assembled(route); err == nil {
		t.Fatal("expected a rendered output with no rendering to be refused")
	}
}

func TestADocumentListsAnAlternativeAmongTheAnswersAndNotTheRefusals(t *testing.T) {
	document, rendered := published(t, signInRoute())
	accepted(t, rendered)

	statuses := []int{}
	for _, path := range document.Paths {
		for _, operation := range path.Operations {
			for _, response := range operation.Responses {
				statuses = append(statuses, response.Status)
			}
		}
	}
	if len(statuses) != 2 || statuses[0] != http.StatusFound || statuses[1] != http.StatusSeeOther {
		t.Fatalf("expected 302 and then 303, got %v", statuses)
	}
	if !strings.Contains(string(rendered), "when the flow was started by a form") {
		t.Fatalf("expected the alternative's description in the document:\n%s", rendered)
	}
}
