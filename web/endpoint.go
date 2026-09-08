package web

import (
	"errors"
	"net/http"

	"github.com/mbauer83/effect-golang-schema/schema"
	"github.com/mbauer83/effect-golang/effect"
)

// Output describes what a route answers with when it succeeds.
type Output[Out any] struct {
	status  int
	content *Content
	encode  func(Out) (Response, error)
	fault   error
}

// Returns answers with a value encoded through its schema.
func Returns[Out any](status int, shape schema.Schema[Out]) Output[Out] {
	content := &Content{MediaType: "application/json", Node: shape.Structure()}
	if fault := schema.Validate(shape); fault != nil {
		return Output[Out]{status: status, content: content,
			fault: faulted("declaring the response body", fault)}
	}
	return Output[Out]{
		status:  status,
		content: content,
		encode:  func(value Out) (Response, error) { return JSON(status, shape, value) },
	}
}

// ReturnsRaw answers with an entity that is already encoded.
//
// It is for a payload this program did not build from a value: a published
// contract, a file, an image. The document says what media type it is and says
// nothing about its shape, because there is no description to say it from --
// which is exactly what a content entry with no schema means.
func ReturnsRaw(status int, mediaType string) Output[[]byte] {
	return Output[[]byte]{
		status:  status,
		content: &Content{MediaType: mediaType},
		encode: func(entity []byte) (Response, error) {
			return Bytes(status, mediaType, entity), nil
		},
	}
}

// ReturnsNothing answers with a status and no entity, which is what a delete or
// an accepted command says.
func ReturnsNothing(status int) Output[effect.Unit] {
	return Output[effect.Unit]{
		status: status,
		encode: func(effect.Unit) (Response, error) { return Empty(status), nil },
	}
}

// FailureResponse records a status a route can answer with when the application
// refuses, and why.
//
// It is documentation. The mapping from a failure to a status lives at the
// boundary, once, and this is how a published document says what that mapping
// will produce.
type FailureResponse struct {
	Status int
	Doc    string
}

// Declaration is everything about a route except how it is handled: what it
// accepts, what it answers with, and what it says about itself.
//
// A projection walks this. Separating it from the handler is what makes three
// things possible from one value -- dispatch, a published document and
// eventually a typed client -- where a route carrying only a function could
// give none of them.
type Declaration struct {
	Method     string
	Path       string
	Summary    string
	Doc        string
	Parameters []Parameter
	Entity     *Content
	Status     int
	Content    *Content
	Failures   []FailureResponse
}

// Endpoint declares what a route accepts and returns. Its handler is separate.
type Endpoint[In, Out any] struct {
	method   string
	segments []segment
	input    Codec[In]
	output   Output[Out]
	summary  string
	doc      string
	failures []FailureResponse
	fault    error
}

// Declare builds an endpoint for any method.
func Declare[In, Out any](
	method string,
	path string,
	input Codec[In],
	output Output[Out],
) Endpoint[In, Out] {
	segments, err := parsePattern(path)
	return Endpoint[In, Out]{
		method:   method,
		segments: segments,
		input:    input,
		output:   output,
		fault:    firstEndpointFault(method, err, segments, input, output),
	}
}

// GET declares an endpoint that reads.
func GET[In, Out any](path string, input Codec[In], output Output[Out]) Endpoint[In, Out] {
	return Declare(http.MethodGet, path, input, output)
}

// POST declares an endpoint that creates or commands.
func POST[In, Out any](path string, input Codec[In], output Output[Out]) Endpoint[In, Out] {
	return Declare(http.MethodPost, path, input, output)
}

// PUT declares an endpoint that replaces.
func PUT[In, Out any](path string, input Codec[In], output Output[Out]) Endpoint[In, Out] {
	return Declare(http.MethodPut, path, input, output)
}

// PATCH declares an endpoint that amends.
func PATCH[In, Out any](path string, input Codec[In], output Output[Out]) Endpoint[In, Out] {
	return Declare(http.MethodPatch, path, input, output)
}

// DELETE declares an endpoint that removes.
func DELETE[In, Out any](path string, input Codec[In], output Output[Out]) Endpoint[In, Out] {
	return Declare(http.MethodDelete, path, input, output)
}

// Summary is the one line a document lists the endpoint by.
func (endpoint Endpoint[In, Out]) Summary(summary string) Endpoint[In, Out] {
	endpoint.summary = summary
	return endpoint
}

// Describe is the prose a document shows beneath the summary.
func (endpoint Endpoint[In, Out]) Describe(doc string) Endpoint[In, Out] {
	endpoint.doc = doc
	return endpoint
}

// Failing records a status the endpoint can answer with when the application
// refuses, for the published document.
func (endpoint Endpoint[In, Out]) Failing(status int, doc string) Endpoint[In, Out] {
	endpoint.failures = append(append([]FailureResponse{}, endpoint.failures...),
		FailureResponse{Status: status, Doc: doc})
	return endpoint
}

// Declaration is what the endpoint says about itself.
func (endpoint Endpoint[In, Out]) Declaration() Declaration {
	return Declaration{
		Method:     endpoint.method,
		Path:       renderPattern(endpoint.segments),
		Summary:    endpoint.summary,
		Doc:        endpoint.doc,
		Parameters: endpoint.input.parameters,
		Entity:     endpoint.input.entity,
		Status:     endpoint.output.status,
		Content:    endpoint.output.content,
		Failures:   endpoint.failures,
	}
}

// ValidateEndpoint reports a declaration mistake in the endpoint, or nil. It is
// the single authority on whether one is usable: it reports a mistake in a
// declared endpoint and one that was never declared at all, so no caller has to
// know there are two ways to be unusable.
func ValidateEndpoint[In, Out any](endpoint Endpoint[In, Out]) error {
	if endpoint.fault != nil {
		return endpoint.fault
	}
	if endpoint.method == "" || endpoint.output.encode == nil {
		return faulted("using an endpoint", errZeroEndpoint)
	}
	return nil
}

// firstEndpointFault reports what would make the endpoint unusable, at the
// moment it is declared rather than on the first request that reaches it.
func firstEndpointFault[In, Out any](
	method string,
	pathErr error,
	segments []segment,
	input Codec[In],
	output Output[Out],
) error {
	switch {
	case method == "":
		return faulted("declaring an endpoint", errNamelessMethod)
	case pathErr != nil:
		return pathErr
	}
	if fault := ValidateCodec(input); fault != nil {
		return fault
	}
	if output.fault != nil {
		return output.fault
	}
	if output.encode == nil {
		return faulted("declaring an endpoint", errNoOutput)
	}
	if output.status < 100 || output.status > 599 {
		return faulted("declaring an endpoint", errUnstatusedOutput)
	}
	return capturesMatchParameters(segments, input.parameters)
}

// capturesMatchParameters reports a path parameter the path does not capture.
// A mistyped name would otherwise be a rejection on every request, discovered
// in production rather than at start-up.
//
// The other direction is not a mistake: a pattern often needs a variable
// segment whose value the handler has no use for, and requiring a reader for
// every capture would make that unexpressible.
func capturesMatchParameters(segments []segment, parameters []Parameter) error {
	captured := make(map[string]bool)
	for _, part := range segments {
		if part.kind != literalSegment {
			captured[part.text] = true
		}
	}
	for _, parameter := range parameters {
		if parameter.In == InPath && !captured[parameter.Name] {
			return faulted("declaring an endpoint",
				errors.New("the path parameter "+parameter.Name+" is not captured by the path"))
		}
	}
	return nil
}

var (
	errZeroEndpoint     = errors.New("the zero Endpoint declares nothing and cannot be used")
	errNamelessMethod   = errors.New("an endpoint has a method")
	errNoOutput         = errors.New("an endpoint says what it answers with; use Returns or ReturnsNothing")
	errUnstatusedOutput = errors.New("an endpoint answers with a status between 100 and 599")
)
