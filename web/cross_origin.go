package web

// Which other origins a browser may let read this surface's answers.
//
// A browser refuses to let a page at one origin read an answer from another
// unless the answer says it may, so a single-page application served from its
// own origin cannot talk to an API on a different port without this -- the
// request is sent, the answer arrives, and the browser throws it away. There
// is nothing a client can do about that: it is the answer that has to speak.
//
// It lives at the boundary rather than in a route or a middleware because the
// boundary is the only place that sees every answer. A page has to be able to
// read a 401 to know to sign in again and a 400 to know what it sent wrong,
// and both of those are produced after a handler has failed -- past where
// anything wrapping the handler can still add a header.

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

// CrossOrigin is what a browser at another origin may do with this surface.
//
// The zero value shares nothing, which is the right default: a surface that
// has not been told which origins may read it is a surface only its own origin
// may read.
type CrossOrigin struct {
	// AllowedOrigins are the origins allowed, spelled as a browser sends them --
	// scheme, host and port, as in "https://films.example" or
	// "http://localhost:5173". Compared exactly and echoed back one at a
	// time, because an answer names the origin that asked rather than the
	// list: a cache holding one answer for several origins is how a page at
	// one of them reads an answer meant for another.
	AllowedOrigins []string
	// AllowedHeaders are the request headers a browser may send beyond the few it
	// considers safe. "Authorization" and "Content-Type" are the two this
	// kind of API needs, and neither is on the safe list.
	AllowedHeaders []string
	// ExposedHeaders are the answer's headers a page may read beyond the few it can
	// read anyway. Empty for an API whose answers are all body.
	ExposedHeaders []string
	// MaxAge is how long a browser may skip asking permission again for the
	// same path and method. Zero leaves it to the browser, which asks every
	// time: correct, and one extra round trip per request.
	MaxAge time.Duration
}

// IsStated reports whether this shares anything at all.
func (crossOrigin CrossOrigin) IsStated() bool { return len(crossOrigin.AllowedOrigins) > 0 }

// allows reports whether an origin is one of the stated ones.
//
// Exact, and no wildcard: a surface that answered every origin would let any
// page anybody visits read what this one's readers can read. A deployment that
// genuinely serves everybody states the origins it serves.
func (crossOrigin CrossOrigin) allows(origin string) bool {
	return origin != "" && slices.Contains(crossOrigin.AllowedOrigins, origin)
}

// allowedOrigin is the origin of a request a browser wants an answer for, and whether
// this surface shares with it.
func (crossOrigin CrossOrigin) allowedOrigin(request *http.Request) (string, bool) {
	origin := request.Header.Get("Origin")
	if !crossOrigin.allows(origin) {
		return "", false
	}
	return origin, true
}

// isPreflight reports whether this request is a browser asking permission
// rather than asking for anything.
//
// A preflight is an OPTIONS carrying the method the page means to use. Every
// request to an API like this one is preflighted, because sending a bearer
// token is itself the thing a browser asks permission for.
func isPreflight(request *http.Request) bool {
	return request.Method == http.MethodOptions &&
		request.Header.Get("Access-Control-Request-Method") != ""
}

// preflightResponse is the answer to a preflight: yes, for the method and headers
// that were asked about.
//
// The method is echoed rather than enumerated, and that is not a shortcut. The
// question a browser asks is "may this page use PUT here", and whether PUT is
// a method this path has is a question the routing already answers -- so a
// permitted preflight followed by a 405 tells a page exactly what is wrong,
// where a refused preflight tells it only that something is.
func (crossOrigin CrossOrigin) preflightResponse(request *http.Request, origin string) Response {
	answer := Empty(http.StatusNoContent).
		WithHeader("Access-Control-Allow-Origin", origin).
		WithHeader("Access-Control-Allow-Methods", request.Header.Get("Access-Control-Request-Method")).
		WithHeader("Vary", "Origin, Access-Control-Request-Method, Access-Control-Request-Headers")
	if headers := crossOrigin.headersFor(request); headers != "" {
		answer = answer.WithHeader("Access-Control-Allow-Headers", headers)
	}
	if crossOrigin.MaxAge > 0 {
		answer = answer.WithHeader("Access-Control-Max-Age",
			strconv.Itoa(int(crossOrigin.MaxAge.Seconds())))
	}
	return answer
}

// headersFor are the stated headers, or the ones asked about when none were
// stated.
//
// Asking about a header a surface did not state is refused by naming the
// stated ones, which is the browser's business to compare -- so a deployment
// that states its headers gets them checked, and one that has not stated any
// is not silently answering yes to everything: with no Headers stated and none
// asked about, nothing is said at all.
func (crossOrigin CrossOrigin) headersFor(request *http.Request) string {
	if len(crossOrigin.AllowedHeaders) > 0 {
		return strings.Join(crossOrigin.AllowedHeaders, ", ")
	}
	return request.Header.Get("Access-Control-Request-Headers")
}

// share is a response a page at this origin may read.
//
// Vary because the answer names one origin: a cache that kept it without this
// would hand a page at one origin the answer that named another, and the
// browser would refuse it.
func (crossOrigin CrossOrigin) share(response Response, origin string) Response {
	shared := response.
		WithHeader("Access-Control-Allow-Origin", origin).
		WithHeader("Vary", varyByOrigin(response))
	if len(crossOrigin.ExposedHeaders) > 0 {
		shared = shared.WithHeader("Access-Control-Expose-Headers",
			strings.Join(crossOrigin.ExposedHeaders, ", "))
	}
	return shared
}

// varyByOrigin is the response's own Vary with Origin among it.
//
// Added to rather than replacing, because a handler that varies by Accept or
// by Accept-Encoding said something true about its answer and this is saying
// one more thing about the same answer -- and a cache told only the second
// would serve a compressed body to a client that cannot read one.
func varyByOrigin(response Response) string {
	already := response.Header().Values("Vary")
	for _, stated := range already {
		for _, name := range strings.Split(stated, ",") {
			if strings.EqualFold(strings.TrimSpace(name), "Origin") {
				return strings.Join(already, ", ")
			}
		}
	}
	if len(already) == 0 {
		return "Origin"
	}
	return strings.Join(already, ", ") + ", Origin"
}
