// Package schema describes the shape of a value once, so that codecs, JSON
// Schema, OpenAPI documents, protobuf descriptors and SQL row mapping are all
// projections of one description rather than several descriptions that have to
// be kept in agreement.
//
// A schema is written rather than derived. Go has neither Scala's implicit
// derivation nor TypeScript's mapped types, so the fields of a struct are
// declared with a getter and a setter:
//
//	type Book struct {
//	    Title   string
//	    Authors []string
//	}
//
//	var BookSchema = schema.Struct[Book]("Book",
//	    schema.FieldOf("title", schema.Text(),
//	        func(book Book) string { return book.Title },
//	        func(book *Book, title string) { book.Title = title }),
//	    schema.FieldOf("authors", schema.List(schema.Text()),
//	        func(book Book) []string { return book.Authors },
//	        func(book *Book, authors []string) { book.Authors = authors }),
//	)
//
// That is more to write than a struct tag. It is also checked by the compiler,
// works when the wire shape differs from the Go shape, and needs no reflection.
// Derivation that produces these values is a later addition and not a
// replacement: a hand-written schema stays the honest path.
package schema

import (
	"github.com/mbauer83/effect-golang-web/schema/structure"
)

// Schema describes the shape of A and how to move a value of A in and out of a
// format.
//
// Its zero value is not usable; build one with a constructor in this package.
type Schema[A any] struct {
	node   structure.Node
	encode func(A, Sink) error
	decode func(Source) (A, error)
	// fault records a declaration mistake, such as two fields with one name.
	//
	// Validate is the single authority on whether a schema is usable: it
	// reports this and a schema that was never built at all, so no caller has
	// to know there are two ways to be unusable.
	//
	// Building a schema does not panic and does not return an error, for the
	// same reason building an effect does neither: a description is not the
	// place where things go wrong. The mistake is reported by Validate, and by
	// the first attempt to use the schema.
	fault error
}

// Structure returns the description a projection walks.
//
// It is exported, and the structure package with it, because a projection is an
// extension point rather than a fixed set. The built-in ones have no access a
// caller's would not.
func (schema Schema[A]) Structure() structure.Node {
	return schema.node
}

// Validate reports a declaration mistake in the schema, or nil. Call it at
// start-up to fail fast rather than on the first request.
func Validate[A any](schema Schema[A]) error {
	if schema.fault != nil {
		return schema.fault
	}
	if schema.encode == nil || schema.decode == nil {
		return zeroSchemaError[A]()
	}
	return nil
}

// Encode writes a value into a sink.
func Encode[A any](schema Schema[A], value A, into Sink) error {
	if err := Validate(schema); err != nil {
		return err
	}
	return schema.encode(value, into)
}

// Decode reads a value from a source.
func Decode[A any](schema Schema[A], from Source) (A, error) {
	if err := Validate(schema); err != nil {
		var missing A
		return missing, err
	}
	return schema.decode(from)
}

// of builds a schema from its three parts. It is the only constructor; every
// public one goes through it, so a schema can never hold a structure that
// disagrees with its codec.
func of[A any](
	node structure.Node,
	encode func(A, Sink) error,
	decode func(Source) (A, error),
) Schema[A] {
	return Schema[A]{node: node, encode: encode, decode: decode}
}

// faulted builds a schema that reports a declaration mistake whenever it is
// used, keeping whatever structure was declared so a projection still has
// something to show.
func faulted[A any](node structure.Node, err error) Schema[A] {
	return Schema[A]{node: node, fault: err}
}

// Named gives a schema a name, so projections that have reusable components
// describe it once and refer to it thereafter.
//
// It applies to an object or a union; naming a scalar or a list has no meaning
// in the projections and is ignored. It is a method because it modifies a
// schema rather than building one, and every modifier here is one -- a reader
// should not have to remember which of them wrap and which are called on what
// they change.
func (schema Schema[A]) Named(name string) Schema[A] {
	switch shape := schema.node.(type) {
	case structure.Object:
		shape.Name = name
		return of(shape, schema.encode, schema.decode)
	case structure.Union:
		shape.Name = name
		return of(shape, schema.encode, schema.decode)
	default:
		return schema
	}
}

// Documented attaches prose a projection can carry into its output.
func (schema Schema[A]) Documented(doc string) Schema[A] {
	switch shape := schema.node.(type) {
	case structure.Object:
		shape.Doc = doc
		return of(shape, schema.encode, schema.decode)
	case structure.Union:
		shape.Doc = doc
		return of(shape, schema.encode, schema.decode)
	default:
		return schema
	}
}

// Transform derives a schema for B from one for A.
//
// It is how a wire shape and a domain type stay separate: the schema describes
// what is on the wire, and the pair of functions moves between that and the
// type the program wants. A conversion that can fail belongs in TransformOrFail.
func Transform[A, B any](inner Schema[A], to func(A) B, from func(B) A) Schema[B] {
	// A transform of a faulted schema is faulted; there is nothing to convert.
	return TransformOrFail(inner,
		func(value A) (B, error) { return to(value), nil },
		func(value B) (A, error) { return from(value), nil },
	)
}

// TransformOrFail derives a schema for B from one for A with conversions that
// may reject a value, which is how a refinement -- a non-empty string, an
// in-range number, a parsable identifier -- is expressed.
func TransformOrFail[A, B any](
	inner Schema[A],
	to func(A) (B, error),
	from func(B) (A, error),
) Schema[B] {
	if fault := Validate(inner); fault != nil {
		return faulted[B](inner.node, fault)
	}
	return of(
		inner.node,
		func(value B, into Sink) error {
			underlying, err := from(value)
			if err != nil {
				return refused(err)
			}
			return Encode(inner, underlying, into)
		},
		func(source Source) (B, error) {
			underlying, err := Decode(inner, source)
			if err != nil {
				var missing B
				return missing, err
			}
			converted, err := to(underlying)
			if err != nil {
				var missing B
				return missing, refused(err)
			}
			return converted, nil
		},
	)
}

// Deferred describes a schema that has not been built yet.
//
// It is how a recursive type is declared, because Go cannot refer to a variable
// in its own initialiser:
//
//	var nodeSchema schema.Schema[Node]
//	nodeSchema = schema.Struct[Node]("Node",
//	    schema.FieldOf("label", schema.Text(), getLabel, setLabel),
//	    schema.FieldOf("children",
//	        schema.List(schema.Deferred(func() schema.Schema[Node] { return nodeSchema })),
//	        getChildren, setChildren),
//	)
//
// The structure it contributes is a Reference, which is what makes a projection
// of a recursive type terminate: the second time a name is reached, a pointer
// is emitted rather than the shape again.
//
// Validate cannot see through it. At the moment an enclosing schema is being
// built the deferred one does not exist yet, so asking about it would report a
// fault that is not real. A mistake behind a Deferred is therefore reported on
// first use rather than by Validate, which is the cost of expressing recursion
// at all.
func Deferred[A any](resolve func() Schema[A]) Schema[A] {
	return of[A](
		structure.Reference{Resolve: func() structure.Node { return resolve().node }},
		func(value A, into Sink) error { return Encode(resolve(), value, into) },
		func(from Source) (A, error) { return Decode(resolve(), from) },
	)
}
