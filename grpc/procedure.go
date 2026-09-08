package grpc

// A procedure: what it is called, and the two shapes it moves.

import (
	"strings"

	"github.com/mbauer83/effect-golang-schema/schema"
	"github.com/mbauer83/effect-golang-schema/schema/structure"
)

// Procedure is one unary procedure of a service.
//
// Only unary for now, and the reason is worth saying rather than leaving to be
// discovered: a streaming procedure is a Stream on one side or both, which is a
// different shape from a Handler and needs the same treatment a websocket
// needed -- the boundary interpreting rather than answering. It is a thing to
// add when there is a caller for it, on that seam.
type Procedure[In, Out any] struct {
	service  string
	method   string
	doc      string
	request  schema.Schema[In]
	response schema.Schema[Out]
	fault    error
}

// Unary declares a procedure.
//
// service is the fully-qualified proto service name -- "logistics.v1.Shipping"
// -- because that is what forms the path a gRPC client calls, and what appears
// in the .proto file another language generates from.
func Unary[In, Out any](
	service string,
	method string,
	request schema.Schema[In],
	response schema.Schema[Out],
) Procedure[In, Out] {
	procedure := Procedure[In, Out]{
		service:  service,
		method:   method,
		request:  request,
		response: response,
	}
	switch {
	case strings.TrimSpace(service) == "":
		procedure.fault = faulted("declaring a procedure", method, errNoService)
	case strings.TrimSpace(method) == "":
		procedure.fault = faulted("declaring a procedure", service, errNoMethod)
	}
	if procedure.fault == nil {
		procedure.fault = firstSchemaFault(procedure)
	}
	return procedure
}

// Documented attaches prose the projected .proto carries.
func (procedure Procedure[In, Out]) Documented(doc string) Procedure[In, Out] {
	procedure.doc = doc
	return procedure
}

// Path is what a client calls, which is gRPC's own shape: a slash, the
// fully-qualified service name, a slash, the method.
func (procedure Procedure[In, Out]) Path() string {
	return "/" + procedure.service + "/" + procedure.method
}

// Service is the fully-qualified service name.
func (procedure Procedure[In, Out]) Service() string {
	return procedure.service
}

// Method is the procedure's own name.
func (procedure Procedure[In, Out]) Method() string {
	return procedure.method
}

// Doc is the prose attached to it.
func (procedure Procedure[In, Out]) Doc() string {
	return procedure.doc
}

// Shapes are the request's and the response's descriptions, which is what a
// .proto projection needs of a procedure.
func (procedure Procedure[In, Out]) Shapes() (structure.Node, structure.Node) {
	return procedure.request.Structure(), procedure.response.Structure()
}

// Fault is why the procedure cannot be served, if it cannot.
//
// A declaration mistake is reported at composition time rather than on the
// first call, which is the same rule the routing tree follows.
func (procedure Procedure[In, Out]) Fault() error {
	return procedure.fault
}

// firstSchemaFault reports a fault either schema inherited from its own
// declaration. A procedure whose request cannot be validated cannot be served,
// and the first call is the wrong place to find out.
func firstSchemaFault[In, Out any](procedure Procedure[In, Out]) error {
	if err := schema.Validate(procedure.request); err != nil {
		return faulted("declaring a procedure", procedure.Path(), err)
	}
	if err := schema.Validate(procedure.response); err != nil {
		return faulted("declaring a procedure", procedure.Path(), err)
	}
	return nil
}
