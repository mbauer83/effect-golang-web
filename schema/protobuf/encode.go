package protobuf

// Writing a value on the protobuf wire, driven by its description.
//
// The value is the universal representation rather than a Go type, for the
// reason the SQL package reads a row that way: a protobuf message is a set of
// named values, which is an object, so the dynamic bridge does the crossing and
// this package needs no second vocabulary. What it adds is the numbers, which
// only the description has.

import (
	"fmt"

	"github.com/mbauer83/effect-golang-web/schema/dynamic"
	"github.com/mbauer83/effect-golang-web/schema/structure"
)

// written is one message's bytes.
func written(node structure.Node, value dynamic.Value) ([]byte, error) {
	switch shape := node.(type) {
	case structure.Object:
		return writtenObject(shape, value)
	case structure.Union:
		return writtenUnion(shape, value)
	case structure.Reference:
		if shape.Resolve == nil {
			return nil, fmt.Errorf("%q is referred to and not described", shape.Name)
		}
		return written(shape.Resolve(), value)
	default:
		return nil, fmt.Errorf("protobuf transfers messages, and %T is not one", node)
	}
}

func writtenObject(object structure.Object, value dynamic.Value) ([]byte, error) {
	held, isObject := value.(dynamic.Object)
	if !isObject {
		return nil, fmt.Errorf("%s is a message, and the value is a %T", object.Name, value)
	}

	into := &writer{}
	for _, member := range object.Fields {
		if member.Number < 1 {
			return nil, fmt.Errorf("field %q of %s: %w", member.Name, object.Name, errUnnumbered)
		}
		carried, present := held.Member(member.Name)
		if !present {
			continue
		}
		if err := field(into, member, carried); err != nil {
			return nil, fmt.Errorf("field %q of %s: %w", member.Name, object.Name, err)
		}
	}
	return into.bytes, nil
}

// writtenUnion writes the chosen variant, and only it.
//
// A union's value is the one-member object its wire form is, and the member's
// name is the variant's -- so the chosen variant is what the value already
// says, and the number is what the description adds.
func writtenUnion(union structure.Union, value dynamic.Value) ([]byte, error) {
	if union.Discriminator != "" {
		return nil, errDiscriminated
	}
	held, isObject := value.(dynamic.Object)
	if !isObject {
		return nil, fmt.Errorf("%s is a oneof, and the value is a %T", union.Name, value)
	}
	chosen, only := held.Only()
	if !only {
		return nil, fmt.Errorf("%s is a choice of one, and the value chose %d",
			union.Name, len(held.Fields))
	}

	for _, variant := range union.Variants {
		if variant.Name != chosen.Name {
			continue
		}
		if variant.Number < 1 {
			return nil, fmt.Errorf("variant %q of %s: %w", variant.Name, union.Name, errUnnumbered)
		}
		into := &writer{}
		err := field(into,
			structure.Field{Name: variant.Name, Node: variant.Node, Number: variant.Number},
			chosen.Value)
		return into.bytes, err
	}
	return nil, fmt.Errorf("%s has no variant named %q", union.Name, chosen.Name)
}

// field writes one field: its tag, and its value in the layout its shape has.
func field(into *writer, member structure.Field, value dynamic.Value) error {
	switch shape := member.Node.(type) {
	case structure.Sequence:
		return repeated(into, member.Number, shape, value)
	case structure.Mapping:
		return entries(into, member.Number, shape, value)
	case structure.Nullable:
		if _, absent := value.(dynamic.Absent); absent {
			// Null is the field not being on the wire. proto3 has no null: a
			// field with explicit presence is present or it is not, and that
			// is the same distinction.
			return nil
		}
		return field(into, structure.Field{
			Name: member.Name, Node: shape.Inner, Number: member.Number,
			Optional: true,
		}, value)
	default:
		return single(into, member, value)
	}
}

// single writes one non-repeated value.
func single(into *writer, member structure.Field, value dynamic.Value) error {
	if _, absent := value.(dynamic.Absent); absent {
		return nil
	}
	switch shape := member.Node.(type) {
	case structure.Scalar:
		return scalar(into, member.Number, shape, value, member.Optional)
	case structure.Object, structure.Union, structure.Reference:
		nested, err := written(member.Node, value)
		if err != nil {
			return err
		}
		into.block(member.Number, nested)
		return nil
	default:
		return fmt.Errorf("%T has no proto3 form", shape)
	}
}

// repeated writes a list.
//
// Numeric and boolean elements are packed into one length-delimited field,
// which is what proto3 does by default and what a reader expects; strings,
// bytes and messages are written one field each, because they are already
// length-delimited and packing them would be a second length nobody reads.
func repeated(
	into *writer,
	number int,
	sequence structure.Sequence,
	value dynamic.Value,
) error {
	held, isList := value.(dynamic.List)
	if !isList {
		return fmt.Errorf("a repeated field takes a list, and the value is a %T", value)
	}
	if packable(sequence.Element) {
		return packed(into, number, sequence.Element, held)
	}
	for index, element := range held.Elements {
		if err := single(into,
			structure.Field{Node: sequence.Element, Number: number}, element); err != nil {
			return fmt.Errorf("element %d: %w", index, err)
		}
	}
	return nil
}

// packed writes the elements into one length-delimited field, with no tag of
// their own.
func packed(
	into *writer,
	number int,
	element structure.Node,
	held dynamic.List,
) error {
	if len(held.Elements) == 0 {
		// An empty repeated field is written as nothing at all, which is how
		// proto3 says a list is empty: there is no other way to say it.
		return nil
	}
	elements := &writer{}
	scalar := element.(structure.Scalar)
	for index, value := range held.Elements {
		if err := packedOne(elements, scalar, value); err != nil {
			return fmt.Errorf("element %d: %w", index, err)
		}
	}
	into.block(number, elements.bytes)
	return nil
}

// packable reports whether a shape's elements go in a packed field: the numeric
// and boolean scalars, and nothing else.
func packable(element structure.Node) bool {
	scalar, isScalar := element.(structure.Scalar)
	if !isScalar {
		return false
	}
	switch scalar.Kind {
	case structure.Integer, structure.Number, structure.Boolean:
		return true
	default:
		return false
	}
}

// entries writes a map as the repeated message proto3 says it is: one message
// per entry, with the key in field 1 and the value in field 2.
func entries(
	into *writer,
	number int,
	mapping structure.Mapping,
	value dynamic.Value,
) error {
	held, isObject := value.(dynamic.Object)
	if !isObject {
		return fmt.Errorf("a map takes an object, and the value is a %T", value)
	}
	for _, entry := range held.Fields {
		pair := &writer{}
		if err := scalar(pair, 1,
			structure.Scalar{Kind: structure.Text}, dynamic.Text{Value: entry.Name}, false); err != nil {
			return err
		}
		if err := single(pair,
			structure.Field{Node: mapping.Value, Number: 2}, entry.Value); err != nil {
			return fmt.Errorf("entry %q: %w", entry.Name, err)
		}
		into.block(number, pair.bytes)
	}
	return nil
}
