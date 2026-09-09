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

// filed is what an answer is kept under: the service, what the request was
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
func (careful *Careful) filed(method string, path string, requesting Requesting) string {
	question := method + " " + path
	if len(requesting.Query) > 0 {
		question += "?" + requesting.Query.Encode()
	}
	return strings.Join([]string{careful.terms.Named, requesting.About, question}, ":")
}

// recalling asks the store, and answers "nothing kept" whether that is because
// nothing is kept or because the store could not be read.
//
// The one absorbed failure here, and absorbed rather than ignored: a cache
// this program cannot read is a slower program, not a broken one, so the
// alternative -- failing a request because a cache is down -- is worse than
// asking the service. It is logged with the key, because a cache that has been
// unreachable for an hour is a rate limit about to be spent and has to be
// visible before that happens.
func recalling[R any](careful *Careful, filed string) effect.Effect[R, Fault, cache.Kept] {
	return effect.Fold(
		cache.Read[R](careful.keeping, filed).CatchAll(complaining[R, cache.Kept]("reading", filed)),
		func(effect.Cause[cache.Fault]) cache.Kept { return cache.Kept{} },
		func(kept cache.Kept) cache.Kept { return kept },
	).MapError(func(effect.Never) Fault { return Fault{} })
}

// filing keeps an answer worth keeping, and answers with it whether or not the
// keeping worked -- for the same reason and with the same log as recalling.
//
// Only a successful answer: a path that was briefly a 500 must not be a 500
// for the next hour.
func filing[R any](
	careful *Careful,
	filed string,
	about string,
) func(Received) effect.Effect[R, Fault, Received] {
	return func(received Received) effect.Effect[R, Fault, Received] {
		if !received.IsSuccessful() {
			return effect.Succeed[R, Fault](received)
		}
		entity, err := written(received)
		if err != nil {
			return effect.Fail[R, Received](asFault("keeping the answer", err))
		}
		kept := cache.Write[R](careful.keeping, cache.Filing{
			Key: filed, About: about, Entity: entity, Fresh: careful.terms.Fresh,
		}).CatchAll(complaining[R, effect.Unit]("keeping", filed))
		return effect.Fold(kept,
			func(effect.Cause[cache.Fault]) Received { return received },
			func(effect.Unit) Received { return received },
		).MapError(func(effect.Never) Fault { return Fault{} })
	}
}

func complaining[R, A any](doing string, filed string) func(cache.Fault) effect.Effect[R, cache.Fault, A] {
	return func(why cache.Fault) effect.Effect[R, cache.Fault, A] {
		return effect.LogWarn[R, cache.Fault](
			"the answer store could not be used; asking the service instead",
			slog.String("doing", doing),
			slog.String("key", filed),
			slog.String("fault", why.Error()),
		).FlatMap(func(effect.Unit) effect.Effect[R, cache.Fault, A] {
			return effect.Fail[R, A](why)
		})
	}
}

// written is a response as HTTP writes one.
//
// Because that is a format which already round-trips a status, a set of
// headers and a body, and is read back by the same standard library that wrote
// it. Keeping only the entity would lose the content type, and inventing a
// format to keep all three would be inventing one that already exists.
func written(received Received) ([]byte, error) {
	response := http.Response{
		Status:        http.StatusText(received.Status),
		StatusCode:    received.Status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        received.Header,
		Body:          io.NopCloser(bytes.NewReader(received.Entity)),
		ContentLength: int64(len(received.Entity)),
	}
	kept := bytes.Buffer{}
	if err := response.Write(&kept); err != nil {
		return nil, err
	}
	return kept.Bytes(), nil
}

// replayed is a kept answer as the response it was, and whether there was one.
//
// A kept answer that cannot be read back is treated as no answer: it is this
// program's own writing, so a failure here is a bug rather than a thing to
// report to a caller, and the service can still be asked.
func replayed(kept cache.Kept) (Received, bool) {
	if !kept.Found {
		return Received{}, false
	}
	response, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(kept.Entity)), nil)
	if err != nil {
		return Received{}, false
	}
	defer func() { _ = response.Body.Close() }()
	entity, err := io.ReadAll(response.Body)
	if err != nil {
		return Received{}, false
	}
	return Received{
		Status: response.StatusCode,
		Header: response.Header,
		Entity: entity,
	}, true
}
