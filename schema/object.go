package schema

import (
	"github.com/mbauer83/effect-golang-web/schema/structure"
)

// Field describes one member of A: its name on the wire, its shape, how to read
// it out of an A and how to write it into an A being built.
//
// The setter takes a pointer and the getter takes a value, so a struct is built
// from its zero value one field at a time. That is what avoids needing an
// N-argument constructor for every arity, and it is how a Go program builds a
// struct anyway.
type Field[A any] struct {
	name     string
	doc      string
	node     structure.Node
	optional bool
	number   int
	fault    error
	encode   func(A, Sink) error
	decode   func(*A, Source) error
	// present reports whether an optional field has a value to write. It is
	// nil for a required field, which always has one.
	present func(A) bool
	// derivable is the presence answer for a field whose absence can be seen
	// from the value itself, which is the case for a described field: the
	// member is in the object or it is not. Optional moves it into present.
	derivable func(A) bool
}

// FieldOf describes a required field.
func FieldOf[A, B any](
	name string,
	shape Schema[B],
	get func(A) B,
	set func(*A, B),
) Field[A] {
	return Field[A]{
		name:  name,
		node:  shape.node,
		fault: Validate(shape),
		encode: func(value A, into Sink) error {
			return Encode(shape, get(value), into)
		},
		decode: func(target *A, from Source) error {
			decoded, err := Decode(shape, from)
			if err != nil {
				return err
			}
			set(target, decoded)
			return nil
		},
	}
}

// OptionalFieldOf describes a field that may be absent.
//
// The getter reports whether the value is present, so absence is a decision the
// program makes rather than a zero value the schema has to guess about: an
// empty string that is meant to be sent is not the same as a field that is not
// there.
func OptionalFieldOf[A, B any](
	name string,
	shape Schema[B],
	get func(A) (B, bool),
	set func(*A, B),
) Field[A] {
	return Field[A]{
		name:     name,
		node:     shape.node,
		optional: true,
		fault:    Validate(shape),
		present: func(value A) bool {
			_, ok := get(value)
			return ok
		},
		encode: func(value A, into Sink) error {
			held, ok := get(value)
			if !ok {
				return into.Null()
			}
			return Encode(shape, held, into)
		},
		decode: func(target *A, from Source) error {
			absent, err := from.Null()
			if err != nil {
				return err
			}
			if absent {
				return nil
			}
			decoded, err := Decode(shape, from)
			if err != nil {
				return err
			}
			set(target, decoded)
			return nil
		},
	}
}

// Documented attaches prose a projection can carry into its output.
func (field Field[A]) Documented(doc string) Field[A] {
	field.doc = doc
	return field
}

// Numbered gives the field a number, for a wire that identifies fields by
// number rather than by name.
//
// It is a modifier and not a parameter of FieldOf because most schemas never
// meet such a wire, and a number every declaration had to carry would be noise
// in all of them. Where one is needed it is required rather than derived: a
// number is what protobuf's compatibility rests on, so the description is where
// it belongs and declaration order is not a stable substitute.
func (field Field[A]) Numbered(number int) Field[A] {
	if number < 1 {
		field.fault = fail("a field number is at least 1", nil)
		return field
	}
	field.number = number
	return field
}

// Optional marks a field that may be absent.
//
// It applies to a field whose presence is answerable: one describing a shape,
// where absence is the member not being there, and one already built by
// OptionalFieldOf. A field bound to a Go type with FieldOf cannot be made
// optional this way, because its getter returns a value and not a value and
// whether there is one -- and absence is a decision the program makes rather
// than a zero value the schema guesses at. That is why OptionalFieldOf takes a
// different getter rather than this taking none.
func (field Field[A]) Optional() Field[A] {
	switch {
	case field.present != nil:
		field.optional = true
	case field.derivable != nil:
		field.present = field.derivable
		field.optional = true
	default:
		field.fault = fail(
			"a bound field states presence in its getter; use OptionalFieldOf", nil)
	}
	return field
}

// Struct describes A as a fixed set of named fields.
//
// A named struct becomes a reusable component in projections that have them, so
// a type used by ten endpoints is described once. Pass an empty name to inline
// it instead.
func Struct[A any](name string, fields ...Field[A]) Schema[A] {
	node := structure.Object{Name: name, Fields: describeFields(fields)}
	if fault := firstFieldFault(fields); fault != nil {
		return faulted[A](node, fault)
	}

	required, byName := indexFields(fields)
	return of[A](
		node,
		func(value A, into Sink) error {
			return encodeFields(value, fields, into)
		},
		func(from Source) (A, error) {
			return decodeFields[A](from, byName, required)
		},
	)
}
