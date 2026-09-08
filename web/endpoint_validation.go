package web

// What makes an endpoint unusable, reported where it is declared.
//
// Separate from the declaration itself because it is the whole of one job: an
// endpoint is checked once, at the moment it is written, so a mistyped
// parameter name is a start-up error and not a rejection on every request that
// reaches it.

import (
	"errors"
)

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
