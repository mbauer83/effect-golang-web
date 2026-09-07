// Package openapi projects a set of route declarations into an OpenAPI 3.1
// document.
//
// The version is 3.1 because its schemas are JSON Schema 2020-12, which is what
// the schema layer already projects: one projection serves both, and nothing
// has to be translated into an older dialect that says almost the same thing.
//
// The document is a typed model rather than a map of a top type, for the reason
// the rest of this module avoids one: a projection that assembled maps could
// not be checked, and every consumer would have to re-discover what shape it
// produced. Rendering is explicit and ordered, so the same routes always
// produce the same bytes.
package openapi

import "github.com/mbauer83/effect-golang-web/schema/jsonschema"

// Info is what a document says about the API as a whole. Title and Version are
// required by the specification.
type Info struct {
	Title       string
	Version     string
	Description string
}

// Server is one place the API is served from.
type Server struct {
	URL         string
	Description string
}

// Document is a whole OpenAPI document.
type Document struct {
	Info    Info
	Servers []Server
	// Paths are in the order the routes were declared, because an author's
	// order is more useful to a reader than an alphabetical one and a stable
	// order is what makes a document diffable.
	Paths []Path
	// Components are the shapes referred to from more than one place, or named.
	Components map[string]jsonschema.Node
}

// Path is one path template and the operations on it.
type Path struct {
	Path       string
	Operations []Operation
}

// Operation is one method on one path.
type Operation struct {
	Method      string
	ID          string
	Summary     string
	Description string
	Parameters  []Parameter
	RequestBody *RequestBody
	// Responses are ordered by status, so the same routes render identically.
	Responses []Response
}

// Parameter is one value the operation reads from the request line or headers.
type Parameter struct {
	Name        string
	In          string
	Required    bool
	Description string
	Schema      jsonschema.Node
}

// RequestBody is the entity an operation expects.
type RequestBody struct {
	Required  bool
	MediaType string
	Schema    jsonschema.Node
}

// Response is one status an operation can answer with. Description is required
// by the specification, so it is never left empty.
type Response struct {
	Status      int
	Description string
	// MediaType and Schema are empty when the response carries no entity.
	MediaType string
	Schema    *jsonschema.Node
}

// ComponentPointer is where an OpenAPI document keeps its shapes, as distinct
// from the $defs a standalone schema uses.
func ComponentPointer(name string) string {
	return "#/components/schemas/" + name
}
