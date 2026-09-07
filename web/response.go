package web

import (
	"io"
	"maps"
	"net/http"
	"strconv"

	"github.com/mbauer83/effect-golang-web/schema"
)

// Response is what a handler produces.
//
// It carries a status and headers a caller can still read and change -- which
// is what middleware needs -- and one function that writes the entity. Having
// one writing form rather than a byte slice beside a stream beside a delegate
// is what keeps this from becoming a set of mutually exclusive modes.
type Response struct {
	// status is 0 when the writer decides, which is the case for a delegated
	// http.Handler: it writes its own.
	status int
	header http.Header
	write  func(http.ResponseWriter, *http.Request) error
}

// Empty is a response with a status and no entity.
func Empty(status int) Response {
	return Response{status: status, header: http.Header{}, write: writeNothing}
}

// Text is a plain-text response.
func Text(status int, body string) Response {
	return Bytes(status, "text/plain; charset=utf-8", []byte(body))
}

// Bytes is a response with an entity already in hand, so its length is known
// and can be declared.
func Bytes(status int, contentType string, body []byte) Response {
	header := http.Header{}
	header.Set("Content-Type", contentType)
	header.Set("Content-Length", strconv.Itoa(len(body)))
	return Response{
		status: status,
		header: header,
		write: func(writer http.ResponseWriter, _ *http.Request) error {
			_, err := writer.Write(body)
			return err
		},
	}
}

// JSON encodes a value through its schema.
//
// The entity is encoded before anything is written, because a status cannot be
// taken back once it has gone out. An encoding failure is therefore reported to
// the caller rather than becoming a truncated body.
func JSON[A any](status int, shape schema.Schema[A], value A) (Response, error) {
	document, err := schema.EncodeJSON(shape, value)
	if err != nil {
		return Response{}, faulted("encoding the response body", err)
	}
	return Bytes(status, "application/json", document), nil
}

// Streaming is a response whose entity is produced as it is written, for the
// case where materialising it first is the wrong trade. Its length is not
// declared, because it is not known.
//
// A failure part-way through cannot change the status, which has already gone
// out. It is reported to the boundary, which records it.
func Streaming(status int, contentType string, body func(io.Writer) error) Response {
	header := http.Header{}
	header.Set("Content-Type", contentType)
	return Response{
		status: status,
		header: header,
		write: func(writer http.ResponseWriter, _ *http.Request) error {
			return body(writer)
		},
	}
}

// Delegate hands the response to an existing http.Handler.
//
// The handler writes its own status, headers and entity, so nothing is buffered
// and a streaming or flushing handler stays exactly what it was. Headers set on
// this response are applied first, which is how a wrapper adds one without
// taking the handler apart.
func Delegate(handler http.Handler) Response {
	return Response{
		header: http.Header{},
		write: func(writer http.ResponseWriter, request *http.Request) error {
			handler.ServeHTTP(writer, request)
			return nil
		},
	}
}

// Status is the response's status, or 0 when the writer decides.
func (response Response) Status() int {
	return response.status
}

// Header is the response's headers. The returned map is the response's own, so
// a caller that means to keep the response unchanged should use WithHeader.
func (response Response) Header() http.Header {
	return response.header
}

// WithStatus returns the response with a different status.
func (response Response) WithStatus(status int) Response {
	response.status = status
	return response
}

// WithHeader returns the response with one header set, leaving the original
// alone -- which is what lets middleware add a header to a response it does not
// own.
func (response Response) WithHeader(name string, value string) Response {
	replaced := http.Header{}
	maps.Copy(replaced, response.header)
	replaced.Set(name, value)
	response.header = replaced
	return response
}

// WriteTo sends the response. The request is passed through because a delegated
// handler needs it, and because a writer may consult it.
func (response Response) WriteTo(writer http.ResponseWriter, request *http.Request) error {
	for name, values := range response.header {
		writer.Header()[name] = values
	}
	if response.status != 0 {
		writer.WriteHeader(response.status)
	}
	if response.write == nil {
		return nil
	}
	return response.write(writer, request)
}

func writeNothing(http.ResponseWriter, *http.Request) error { return nil }
