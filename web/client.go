package web

// Calling a server, which is the one side of HTTP this module did not have.
//
// Every other transport here has both: websocket.Dial, amqp091.Connect,
// amqp10.Connect, grpc.Dial. A program that answered requests in this idiom
// had to leave it to make one, and lose the typed failure channel, the schema
// codecs and the observer on the way out.
//
// What this does not do is configure a transport. TLS, proxies, pooling,
// redirects, timeouts and HTTP/2 are the http.Client's, which is the division
// grpc.Dial already makes here: how a deployment reaches a peer is not a
// property of a call.

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/mbauer83/effect-golang-schema/schema"
	"github.com/mbauer83/effect-golang/effect"
)

// Client sends requests to one base address.
type Client struct {
	client  *http.Client
	address string
}

// Dial makes a client for a base address -- "http://host:8080". Each call's
// path is appended to it.
//
// A plain constructor and not a scoped effect, because an http.Client owns no
// lifetime this package can end: it has no Close, and its idle connections are
// its own business. grpc.Dial says the same thing for the same reason.
func Dial(client *http.Client, address string) *Client {
	return &Client{client: client, address: strings.TrimSuffix(address, "/")}
}

// Requesting is the parts of a request the caller supplies.
//
// Every field is optional. A GET with nothing to say is the zero value, which
// is why this is a struct and not five arguments.
type Requesting struct {
	// Path fills an endpoint's captured segments by name, so a caller writes
	// {"title": "Zionomicon"} rather than building the path itself. Fetch
	// takes the path whole and reads nothing here.
	Path map[string]string
	// Query is the query string, encoded as net/url encodes it.
	Query url.Values
	// Header is sent as given. Content-Type comes from MediaType instead, so
	// the entity and the type that describes it cannot disagree.
	Header http.Header
	// Entity is the request body, already encoded, and MediaType says what it
	// is. Carrying fills both from a value and its schema.
	Entity    []byte
	MediaType string
}

// Received is one response, read whole.
type Received struct {
	Status int
	Header http.Header
	Entity []byte
}

// Carrying returns the request with a value encoded through its schema as the
// entity.
//
// A package function because a method cannot introduce the type parameter its
// own argument needs -- the rule the whole module follows.
func Carrying[A any](
	requesting Requesting,
	shape schema.Schema[A],
	value A,
) (Requesting, error) {
	document, err := schema.EncodeJSON(shape, value)
	if err != nil {
		return Requesting{}, faulted("encoding the request body", err)
	}
	requesting.Entity = document
	requesting.MediaType = "application/json"
	return requesting, nil
}

// Fetch sends a request to a path and reads the whole response.
//
// The entity is read and the body closed within the effect, so there is no
// open response for a caller to forget -- which is the leak net/http's client
// is best known for. A response read as a stream is deliberately absent: it
// needs a scope to own the body and a Stream to pull it, and it can be added
// when something asks for it rather than guessed at now.
//
// Cancellation, retry and observation come from the interpretation rather than
// from here: the context is the runtime's, so Retry and a Schedule compose over
// this as they do over anything.
func Fetch[R any](
	client *Client,
	method string,
	path string,
	requesting Requesting,
) effect.Effect[R, Fault, Received] {
	return effect.Try(
		func(ctx context.Context, _ R) (Received, error) {
			return exchange(ctx, client, method, path, requesting)
		},
		func(err error) Fault { return asFault("calling "+method+" "+path, err) },
	).Named("fetch")
}

func exchange(
	ctx context.Context,
	client *Client,
	method string,
	path string,
	requesting Requesting,
) (Received, error) {
	if client == nil || client.client == nil {
		return Received{}, errNoHTTPClient
	}
	address := client.address + path
	if len(requesting.Query) > 0 {
		address += "?" + requesting.Query.Encode()
	}

	var entity io.Reader
	if len(requesting.Entity) > 0 {
		entity = bytes.NewReader(requesting.Entity)
	}
	request, err := http.NewRequestWithContext(ctx, method, address, entity)
	if err != nil {
		return Received{}, err
	}
	for name, values := range requesting.Header {
		request.Header[http.CanonicalHeaderKey(name)] = values
	}
	if requesting.MediaType != "" {
		request.Header.Set("Content-Type", requesting.MediaType)
	}

	response, err := client.client.Do(request)
	if err != nil {
		return Received{}, err
	}
	defer func() { _ = response.Body.Close() }()

	read, err := io.ReadAll(response.Body)
	if err != nil {
		return Received{}, err
	}
	return Received{Status: response.StatusCode, Header: response.Header, Entity: read}, nil
}

// asFault keeps one Fault rather than wrapping a Fault in a Fault, so a caller
// reading Doing sees what actually failed and not the outermost stage.
func asFault(doing string, err error) Fault {
	var already Fault
	if errors.As(err, &already) {
		return already
	}
	return Fault{Doing: doing, Err: err}
}

var errNoHTTPClient = errors.New("a client needs an http.Client; see web.Dial")
