package web

import (
	"errors"

	"github.com/mbauer83/effect-golang-schema/schema"
)

// Entity reads the request body as JSON, described by a schema.
//
// It decodes straight from the body's reader rather than reading it first, so a
// large document is not materialised only to be walked. That is what the sink
// and source contracts are for.
//
// A body a schema refuses is a rejection rather than the application's failure:
// the request never reaches the handler, and the handler's failure type stays
// about the application.
func Entity[A any](shape schema.Schema[A]) Codec[A] {
	content := &Content{MediaType: "application/json", Node: shape.Structure()}
	if fault := schema.Validate(shape); fault != nil {
		return Codec[A]{entity: content, fault: faultOf("declaring the request body", fault)}
	}
	return Codec[A]{
		entity: content,
		decode: func(request Request) (A, error) {
			body := request.Source().Body
			if body == nil {
				var zero A
				return zero, Fault{Op: "reading the request body", Err: errNoEntity}
			}
			value, err := schema.DecodeJSONFrom(shape, body)
			if err != nil {
				var zero A
				return zero, Fault{Op: "reading the request body", Err: err}
			}
			return value, nil
		},
	}
}

var errNoEntity = errors.New("it is required and the request carried none")
