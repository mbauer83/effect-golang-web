package schema

// How a struct's fields are described, indexed, written and read.
//
// Separate from the declaration surface because these are the mechanics: the
// order the fields are written in, the map a decoder looks a name up in, and
// what happens to a name nobody declared.

import (
	"strings"

	"github.com/mbauer83/effect-golang-web/schema/structure"
)

func describeFields[A any](fields []Field[A]) []structure.Field {
	described := make([]structure.Field, 0, len(fields))
	for _, field := range fields {
		described = append(described, structure.Field{
			Name:     field.name,
			Doc:      field.doc,
			Node:     field.node,
			Optional: field.optional,
			Number:   field.number,
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
