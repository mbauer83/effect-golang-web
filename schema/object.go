package schema

import (
	"strings"

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

func describeFields[A any](fields []Field[A]) []structure.Field {
	described := make([]structure.Field, 0, len(fields))
	for _, field := range fields {
		described = append(described, structure.Field{
			Name:     field.name,
			Doc:      field.doc,
			Node:     field.node,
			Optional: field.optional,
		})
	}
	return described
}

// firstFieldFault reports a duplicate name, an empty name, or a fault inherited
// from a field's own schema. Two fields with one name would make encoding and
// decoding disagree, so it is a declaration mistake and not a precedence rule.
func firstFieldFault[A any](fields []Field[A]) error {
	seen := make(map[string]bool, len(fields))
	for _, field := range fields {
		switch {
		case strings.TrimSpace(field.name) == "":
			return fail("a field has no name", nil)
		case seen[field.name]:
			return fail("two fields are named "+field.name, nil)
		case field.fault != nil:
			return within(field.name, field.fault)
		}
		seen[field.name] = true
	}
	return nil
}

func indexFields[A any](fields []Field[A]) (required []string, byName map[string]Field[A]) {
	byName = make(map[string]Field[A], len(fields))
	for _, field := range fields {
		byName[field.name] = field
		if !field.optional {
			required = append(required, field.name)
		}
	}
	return required, byName
}

func encodeFields[A any](value A, fields []Field[A], into Sink) error {
	if err := into.BeginObject(); err != nil {
		return err
	}
	for _, field := range fields {
		// An absent optional field is omitted rather than written as null.
		// Omission is what a reader of the projection is told to expect, and it
		// is what a document written by hand would do.
		if field.present != nil && !field.present(value) {
			continue
		}
		if err := into.FieldName(field.name); err != nil {
			return err
		}
		if err := field.encode(value, into); err != nil {
			return within(field.name, err)
		}
	}
	return into.EndObject()
}

func decodeFields[A any](from Source, byName map[string]Field[A], required []string) (A, error) {
	var built A
	seen := make(map[string]bool, len(byName))

	err := from.ReadObject(func(name string) error {
		field, known := byName[name]
		if !known {
			// An unknown field is tolerated. A schema that rejected one could
			// not read a document written by a newer version of its producer.
			return from.Skip()
		}
		seen[name] = true
		return within(name, field.decode(&built, from))
	})
	if err != nil {
		var missing A
		return missing, err
	}

	for _, name := range required {
		if !seen[name] {
			var missing A
			return missing, within(name, fail("required field is missing", nil))
		}
	}
	return built, nil
}
