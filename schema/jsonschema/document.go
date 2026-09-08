// Package jsonschema projects a schema's structure into JSON Schema.
//
// The dialect is 2020-12, which is the one OpenAPI 3.1 uses, so the same
// projection serves both a standalone schema document and an OpenAPI
// component.
//
// The document is a typed model rather than a map of any, for the reason the
// rest of this module avoids top types: a projection that assembled maps could
// not be checked, and every consumer would have to re-discover what shape it
// produced. Rendering is explicit and ordered, so the same schema always
// produces the same bytes.
package jsonschema

import "sort"

// Node is one JSON Schema. Only the fields a projection of this module's
// structures can produce are present; a field left empty is not rendered.
type Node struct {
	// Ref, when set, replaces everything else: a reference is a whole schema.
	Ref string

	Type        string
	Format      string
	Description string

	// Properties are ordered as the structure declared them, because a reader
	// of the document benefits from the author's order and a diff benefits
	// from it being stable.
	Properties []Property
	Required   []string

	// Items describes a sequence's element.
	Items *Node
	// Values describes a mapping's value, rendered as additionalProperties.
	Values *Node
	// OneOf describes a choice: a union's variants, or a nullable shape and
	// null.
	OneOf []Node

	// Bounds are the constraints the shape carries.
	Bounds Bounds
}

// Bounds are the keywords that narrow which values of a type are admitted.
//
// A nil field is a keyword the shape does not carry, which is different from
// one carrying a zero: a minimum of nothing and a minimum of zero are not the
// same statement.
type Bounds struct {
	Minimum          *float64
	Maximum          *float64
	ExclusiveMinimum *float64
	ExclusiveMaximum *float64
	MinLength        *int
	MaxLength        *int
	Pattern          string
	MinItems         *int
	MaxItems         *int
}

// Property is one named member of an object schema.
type Property struct {
	Name   string
	Schema Node
}

// Document is a root schema together with the components it refers to.
type Document struct {
	Root       Node
	Components map[string]Node
}

// ComponentNames returns the component names in a stable order, so a caller
// rendering them produces the same output every time.
func (document Document) ComponentNames() []string {
	names := make([]string, 0, len(document.Components))
	for name := range document.Components {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
