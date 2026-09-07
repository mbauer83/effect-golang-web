package web

import (
	"errors"
	"net/textproto"
	"strings"

	"github.com/mbauer83/effect-golang-web/schema"
)

// PathParam reads a captured segment of the request path.
//
// A path parameter is always required: a path that did not carry it would not
// have matched the route.
func PathParam[A any](name string, shape schema.Schema[A]) Codec[A] {
	return required(name, InPath, shape, func(request Request) (string, bool) {
		return request.Capture(name)
	})
}

// QueryParam reads a required query-string parameter.
func QueryParam[A any](name string, shape schema.Schema[A]) Codec[A] {
	return required(name, InQuery, shape, queryValue(name))
}

// OptionalQueryParam reads a query-string parameter that may be absent,
// yielding nil when it is.
//
// Absent and empty are different: ?page= carries an empty value, and a caller
// that wants to tell those apart can.
func OptionalQueryParam[A any](name string, shape schema.Schema[A]) Codec[*A] {
	return optional(name, InQuery, shape, queryValue(name))
}

// HeaderParam reads a required header.
func HeaderParam[A any](name string, shape schema.Schema[A]) Codec[A] {
	return required(name, InHeader, shape, headerValue(name))
}

// OptionalHeaderParam reads a header that may be absent, yielding nil when it
// is.
func OptionalHeaderParam[A any](name string, shape schema.Schema[A]) Codec[*A] {
	return optional(name, InHeader, shape, headerValue(name))
}

// Describing attaches prose to a codec's parameter, for the published document.
//
// It applies to a codec that reads exactly one parameter, because prose about
// "the parameters" would not tell a reader which one it meant.
func Describing[A any](codec Codec[A], doc string) Codec[A] {
	if len(codec.parameters) != 1 {
		codec.fault = faulted("describing a parameter", errNotOneParameter)
		return codec
	}
	described := append([]Parameter{}, codec.parameters...)
	described[0].Doc = doc
	codec.parameters = described
	return codec
}

// required builds a codec for a parameter that must be there.
func required[A any](
	name string,
	in Location,
	shape schema.Schema[A],
	read func(Request) (string, bool),
) Codec[A] {
	parameter := Parameter{Name: name, In: in, Required: true, Node: shape.Structure()}
	if fault := parameterFault(name, shape); fault != nil {
		return Codec[A]{parameters: []Parameter{parameter}, fault: fault}
	}
	return Codec[A]{
		parameters: []Parameter{parameter},
		decode: func(request Request) (A, error) {
			carried, present := read(request)
			if !present {
				var missing A
				return missing, rejecting(parameter, errAbsentParameter)
			}
			decoded, err := schema.Decode(shape, textSource{value: carried})
			if err != nil {
				var missing A
				return missing, rejecting(parameter, err)
			}
			return decoded, nil
		},
	}
}

// optional builds a codec for a parameter that may be absent.
func optional[A any](
	name string,
	in Location,
	shape schema.Schema[A],
	read func(Request) (string, bool),
) Codec[*A] {
	parameter := Parameter{Name: name, In: in, Node: shape.Structure()}
	if fault := parameterFault(name, shape); fault != nil {
		return Codec[*A]{parameters: []Parameter{parameter}, fault: fault}
	}
	return Codec[*A]{
		parameters: []Parameter{parameter},
		decode: func(request Request) (*A, error) {
			carried, present := read(request)
			if !present {
				return nil, nil
			}
			decoded, err := schema.Decode(shape, textSource{value: carried})
			if err != nil {
				return nil, rejecting(parameter, err)
			}
			return &decoded, nil
		},
	}
}

func queryValue(name string) func(Request) (string, bool) {
	return func(request Request) (string, bool) {
		values, present := request.Query()[name]
		if !present || len(values) == 0 {
			return "", false
		}
		return values[0], true
	}
}

func headerValue(name string) func(Request) (string, bool) {
	return func(request Request) (string, bool) {
		if _, present := request.Header()[textproto.CanonicalMIMEHeaderKey(name)]; !present {
			return "", false
		}
		return request.Header().Get(name), true
	}
}

func parameterFault[A any](name string, shape schema.Schema[A]) error {
	if strings.TrimSpace(name) == "" {
		return faulted("declaring a parameter", errNamelessParameter)
	}
	if fault := schema.Validate(shape); fault != nil {
		return faulted("declaring the parameter "+name, fault)
	}
	return nil
}

// rejecting names the parameter a rejection is about, because "expected a whole
// number" is not something a client can act on and "expected a whole number in
// the query parameter page" is.
func rejecting(parameter Parameter, err error) error {
	return Fault{
		Doing: "reading the " + string(parameter.In) + " parameter " + parameter.Name,
		Err:   err,
	}
}

var (
	errAbsentParameter   = errors.New("it is required and was not given")
	errNamelessParameter = errors.New("a parameter has no name")
	errNotOneParameter   = errors.New("prose applies to a codec that reads exactly one parameter")
)
