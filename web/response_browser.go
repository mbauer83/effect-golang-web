package web

// What a browser acts on beside an entity: being sent somewhere else, and
// keeping or dropping a cookie.
//
// Named as Effect's platform names them -- redirect, setCookie, expireCookie --
// because a reader arriving from there looks for exactly those words.

import (
	"errors"
	"maps"
	"net/http"
	"time"
)

// Redirect is a response sending the client to another address, 302 Found.
//
// Found because it is what a browser follows with a GET whatever the request
// was, which is what a sign-in flow's hops are. Another redirection status is
// WithStatus away, and the location is kept whatever the status becomes.
func Redirect(location string) Response {
	header := http.Header{}
	header.Set("Location", location)
	return Response{status: http.StatusFound, header: header, write: writeNothing}
}

// ErrUnwritableCookie is a cookie net/http would not send as given.
var ErrUnwritableCookie = errors.New("web: a cookie has a name and a value net/http can write")

// WithCookie returns the response with one more cookie set, leaving the
// original alone.
//
// Added rather than set, because a response may carry several cookies and each
// is its own Set-Cookie line. A cookie net/http cannot write is refused here
// rather than dropped: net/http writes an invalid cookie as nothing at all, so
// a mistyped name would otherwise be a sign-in that silently never sticks.
func (response Response) WithCookie(cookie *http.Cookie) (Response, error) {
	if cookie == nil {
		return Response{}, ErrUnwritableCookie
	}
	if err := cookie.Valid(); err != nil {
		return Response{}, errors.Join(ErrUnwritableCookie, err)
	}
	header := http.Header{}
	maps.Copy(header, response.header)
	header["Set-Cookie"] = append(append([]string{}, header["Set-Cookie"]...), cookie.String())
	response.header = header
	return response, nil
}

// ExpireCookie returns the response telling the client to drop a cookie.
//
// The cookie is named as it was set: a browser drops a cookie by name, path and
// domain together, so an expiry for another path is an expiry of nothing. Its
// value and lifetime are this function's to decide, and are replaced.
func (response Response) ExpireCookie(cookie http.Cookie) (Response, error) {
	cookie.Value = ""
	cookie.MaxAge = -1
	cookie.Expires = expiredLongAgo
	return response.WithCookie(&cookie)
}

// expiredLongAgo is the date an expired cookie carries beside its zero Max-Age,
// for the clients that read only Expires.
var expiredLongAgo = time.Unix(0, 0).UTC()
