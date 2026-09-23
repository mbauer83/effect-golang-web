package web

// What a kept answer is, and what it is filed under.

import (
	"bufio"
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/mbauer83/effect-golang/effect"
	"github.com/mbauer83/effect-golang/effect/cache"
)

// cacheKey is what an answer is kept under: the service, what the request was
// about, and the request itself.
//
// Readable rather than hashed, because the first thing anybody does with a
// cache that is behaving oddly is look at what is in it, and a digest tells
// them nothing. The query is encoded as net/url encodes it, which sorts it, so
// two requests differing only in the order they were built are one answer.
//
// The service's name is in it because two services read by one program answer
// differently at the same path, and what a request was about is in it because
// that is how everything about one thing is dropped together.
func (upstream *UpstreamClient) cacheKey(method string, path string, request ClientRequest) string {
	question := method + " " + path
	if len(request.Query) > 0 {
		question += "?" + request.Query.Encode()
	}
	return strings.Join([]string{upstream.terms.Name, request.About, question}, ":")
}

// lookupCache asks the store, and answers "nothing kept" whether that is because
// nothing is kept or because the store could not be read.
//
// The one absorbed failure here, and absorbed rather than ignored: a cache
// this program cannot read is a slower program, not a broken one, so the
// alternative -- failing a request because a cache is down -- is worse than
// asking the service. It is logged with the key, because a cache that has been
// unreachable for an hour is a rate limit about to be spent and has to be
// visible before that happens.
func lookupCache[R any](upstream *UpstreamClient, key string) effect.Effect[R, Fault, cache.Lookup] {
	return effect.Fold(
		cache.Read[R](upstream.store, key).CatchAll(logCacheFault[R, cache.Lookup]("reading", key)),
		func(effect.Cause[cache.Fault]) cache.Lookup { return cache.Lookup{} },
		func(lookup cache.Lookup) cache.Lookup { return lookup },
	).MapError(func(effect.Never) Fault { return Fault{} })
}

// writeCache keeps an answer worth keeping, and answers with it whether or not the
// keeping worked -- for the same reason and with the same log as recalling.
//
// Only a successful answer: a path that was briefly a 500 must not be a 500
// for the next hour.
func writeCache[R any](
	upstream *UpstreamClient,
	key string,
	about string,
) func(ClientResponse) effect.Effect[R, Fault, ClientResponse] {
	return func(response ClientResponse) effect.Effect[R, Fault, ClientResponse] {
		if !response.IsSuccessful() {
			return effect.Succeed[R, Fault](response)
		}
		entity, err := marshalResponse(response)
		if err != nil {
			return effect.Fail[R, ClientResponse](asFault("keeping the answer", err))
		}
		writeEntry := cache.Write[R](upstream.store, cache.Entry{
			Key: key, About: about, Entity: entity, Fresh: upstream.terms.TimeToLive,
		}).CatchAll(logCacheFault[R, effect.Unit]("keeping", key))
		return effect.Fold(writeEntry,
			func(effect.Cause[cache.Fault]) ClientResponse { return response },
			func(effect.Unit) ClientResponse { return response },
		).MapError(func(effect.Never) Fault { return Fault{} })
	}
}

func logCacheFault[R, A any](op string, key string) func(cache.Fault) effect.Effect[R, cache.Fault, A] {
	return func(why cache.Fault) effect.Effect[R, cache.Fault, A] {
		return effect.LogWarn[R, cache.Fault](
			"the answer store could not be used; asking the service instead",
			slog.String("doing", op),
			slog.String("key", key),
			slog.String("fault", why.Error()),
		).FlatMap(func(effect.Unit) effect.Effect[R, cache.Fault, A] {
			return effect.Fail[R, A](why)
		})
	}
}

// marshalResponse is a response as HTTP writes one.
//
// Because that is a format which already round-trips a status, a set of
// headers and a body, and is read back by the same standard library that wrote
// it. Keeping only the entity would lose the content type, and inventing a
// format to keep all three would be inventing one that already exists.
func marshalResponse(response ClientResponse) ([]byte, error) {
	httpResponse := http.Response{
		Status:        http.StatusText(response.Status),
		StatusCode:    response.Status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        response.Header,
		Body:          io.NopCloser(bytes.NewReader(response.Entity)),
		ContentLength: int64(len(response.Entity)),
	}
	buffer := bytes.Buffer{}
	if err := httpResponse.Write(&buffer); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// responseFrom is a kept answer as the response it was, and whether there was one.
//
// A kept answer that cannot be read back is treated as no answer: it is this
// program's own writing, so a failure here is a bug rather than a thing to
// report to a caller, and the service can still be asked.
func responseFrom(lookup cache.Lookup) (ClientResponse, bool) {
	if !lookup.Found {
		return ClientResponse{}, false
	}
	response, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(lookup.Entity)), nil)
	if err != nil {
		return ClientResponse{}, false
	}
	defer func() { _ = response.Body.Close() }()
	entity, err := io.ReadAll(response.Body)
	if err != nil {
		return ClientResponse{}, false
	}
	return ClientResponse{
		Status: response.StatusCode,
		Header: response.Header,
		Entity: entity,
	}, true
}
