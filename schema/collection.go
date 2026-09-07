package schema

import (
	"maps"
	"slices"

	"github.com/mbauer83/effect-golang-web/schema/structure"
)

// List describes an ordered, variable-length sequence.
//
// A decoded list is never nil: an empty document array decodes to an empty
// slice, so a caller can range over the result without checking.
func List[A any](element Schema[A]) Schema[[]A] {
	node := structure.Sequence{Element: element.node}
	if fault := Validate(element); fault != nil {
		return faulted[[]A](node, fault)
	}

	return of[[]A](
		node,
		func(values []A, into Sink) error {
			if err := into.BeginList(); err != nil {
				return err
			}
			for index, value := range values {
				if err := Encode(element, value, into); err != nil {
					return within(listIndex(index), err)
				}
			}
			return into.EndList()
		},
		func(from Source) ([]A, error) {
			decoded := make([]A, 0)
			err := from.ReadList(func() error {
				value, err := Decode(element, from)
				if err != nil {
					return within(listIndex(len(decoded)), err)
				}
				decoded = append(decoded, value)
				return nil
			})
			if err != nil {
				return nil, err
			}
			return decoded, nil
		},
	)
}

// Map describes a variable set of string-keyed values.
//
// Keys are strings because that is what every wire format this module targets
// can express as an object key. A map with structured keys is a list of pairs,
// and saying so is better than pretending otherwise.
//
// Encoding sorts the keys, so the same value always produces the same document
// and a golden test is possible.
func Map[A any](value Schema[A]) Schema[map[string]A] {
	node := structure.Mapping{
		Key:   structure.Scalar{Kind: structure.Text},
		Value: value.node,
	}
	if fault := Validate(value); fault != nil {
		return faulted[map[string]A](node, fault)
	}

	return of[map[string]A](
		node,
		func(entries map[string]A, into Sink) error {
			if err := into.BeginObject(); err != nil {
				return err
			}
			for _, key := range sortedKeys(entries) {
				if err := into.FieldName(key); err != nil {
					return err
				}
				if err := Encode(value, entries[key], into); err != nil {
					return within(key, err)
				}
			}
			return into.EndObject()
		},
		func(from Source) (map[string]A, error) {
			decoded := make(map[string]A)
			err := from.ReadObject(func(key string) error {
				held, err := Decode(value, from)
				if err != nil {
					return within(key, err)
				}
				decoded[key] = held
				return nil
			})
			if err != nil {
				return nil, err
			}
			return decoded, nil
		},
	)
}

// Nullable describes a value a format may carry as null, represented as a
// pointer because that is how Go spells "this may be absent".
//
// It is not the same as an optional field. A nullable value is present and
// null; an optional field is not there at all. Formats and readers treat those
// differently, so the schema does too.
func Nullable[A any](inner Schema[A]) Schema[*A] {
	if fault := Validate(inner); fault != nil {
		return faulted[*A](inner.node, fault)
	}

	return of[*A](
		structure.Nullable{Inner: inner.node},
		func(value *A, into Sink) error {
			if value == nil {
				return into.Null()
			}
			return Encode(inner, *value, into)
		},
		func(from Source) (*A, error) {
			absent, err := from.Null()
			if err != nil {
				return nil, err
			}
			if absent {
				return nil, nil
			}
			held, err := Decode(inner, from)
			if err != nil {
				return nil, err
			}
			return &held, nil
		},
	)
}

// sortedKeys orders a map's keys so encoding is deterministic. Go randomises map
// iteration deliberately, and a document that changes shape between runs cannot
// be compared in a test or cached by an intermediary.
func sortedKeys[A any](entries map[string]A) []string {
	return slices.Sorted(maps.Keys(entries))
}
