package protobuf

// Reading a value from the protobuf wire, driven by its description.
//
// The wire identifies a field by number and says nothing about its name, so the
// description is what turns one into the other. A number the description does
// not know is skipped, which is the property protobuf is chosen for: a message
// written by a newer program stays readable.
//
// Repeated fields are gathered rather than read in place. The format permits
// the same number to appear more than once and in any order -- that is how an
// unpacked repeated field is written, and a packed one may even be split -- so a
// reader that expected them together would be reading a message no writer
// promised.

import (
	"fmt"

	"github.com/mbauer83/effect-golang-web/schema/dynamic"
	"github.com/mbauer83/effect-golang-web/schema/structure"
)

// read is the value a message's bytes are, under a description.
func read(node structure.Node, bytes []byte) (dynamic.Value, error) {
	switch shape := node.(type) {
	case structure.Object:
		return readObject(shape, bytes)
	case structure.Union:
		return readUnion(shape, bytes)
	case structure.Reference:
		if shape.Resolve == nil {
			return nil, fmt.Errorf("%q is referred to and not described", shape.Name)
		}
		return read(shape.Resolve(), bytes)
	default:
		return nil, fmt.Errorf("protobuf transfers messages, and %T is not one", node)
	}
}

// occurrence is one appearance of a field on the wire.
type occurrence struct {
	kind  wireType
	bytes []byte
	fixed uint64
}

// gathered reads the message into its fields' occurrences, in the order they
// appeared, skipping the numbers the description does not know.
func gathered(known map[int]bool, bytes []byte) (map[int][]occurrence, error) {
	from := &reader{bytes: bytes}
	found := map[int][]occurrence{}
	for !from.done() {
		number, kind, err := from.tag()
		if err != nil {
			return nil, err
		}
		if !known[number] {
			if err := from.skip(kind); err != nil {
				return nil, err
			}
			continue
		}
		appearance, err := appearing(from, kind)
		if err != nil {
			return nil, fmt.Errorf("field %d: %w", number, err)
		}
		found[number] = append(found[number], appearance)
	}
	return found, nil
}

func appearing(from *reader, kind wireType) (occurrence, error) {
	switch kind {
	case varying:
		value, err := from.varint()
		return occurrence{kind: kind, fixed: value}, err
	case eightBytes:
		value, err := from.fixed64()
		return occurrence{kind: kind, fixed: value}, err
	case fourBytes:
		value, err := from.fixed32()
		return occurrence{kind: kind, fixed: uint64(value)}, err
	case counted:
		held, err := from.block()
		return occurrence{kind: kind, bytes: held}, err
	default:
		return occurrence{}, fmt.Errorf("wire type %d is not one this format defines", kind)
	}
}

func readObject(object structure.Object, bytes []byte) (dynamic.Value, error) {
	known := map[int]bool{}
	for _, member := range object.Fields {
		if member.Number < 1 {
			return nil, fmt.Errorf("field %q of %s: %w", member.Name, object.Name, errUnnumbered)
		}
		known[member.Number] = true
	}
	found, err := gathered(known, bytes)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", object.Name, err)
	}

	// In the order the description declares them, not the order the wire
	// carried them: the representation's objects are ordered, and a reader that
	// forwarded what it received should send the same thing twice.
	held := dynamic.Object{Fields: make([]dynamic.Field, 0, len(object.Fields))}
	for _, member := range object.Fields {
		value, present, err := readMember(member, found[member.Number])
		if err != nil {
			return nil, fmt.Errorf("field %q of %s: %w", member.Name, object.Name, err)
		}
		if !present {
			continue
		}
		held.Fields = append(held.Fields, dynamic.Field{Name: member.Name, Value: value})
	}
	return held, nil
}

// readMember is the value a field's occurrences are, and whether it was there.
//
// A field the wire carried nothing for is the zero of its kind when it has
// implicit presence, and absent when it has explicit presence. That is not a
// default invented here: proto3's ordinary field writes no bytes for its zero,
// so absent and zero are the same message and there is no reading in which the
// field was not sent. What `optional` buys is exactly the difference, and a
// message field has it inherently.
//
// The description's own rules then apply to whatever resulted, which is where a
// zero gets refused: a count of at least one rejects the zero the wire implied
// exactly as it would reject one the wire spelled out.
func readMember(member structure.Field, found []occurrence) (dynamic.Value, bool, error) {
	switch shape := member.Node.(type) {
	case structure.Sequence:
		return readRepeated(shape, found)
	case structure.Mapping:
		return readEntries(shape, found)
	case structure.Nullable:
		// A nullable field has explicit presence, so an absent one stays
		// absent rather than becoming its kind's zero.
		return readMember(structure.Field{
			Name: member.Name, Node: shape.Inner, Number: member.Number,
			Optional: true,
		}, found)
	default:
		if len(found) == 0 {
			if member.Optional {
				return nil, false, nil
			}
			value, implied := zeroOf(member.Node)
			return value, implied, nil
		}
		// The last one wins, which is what protobuf says of a repeated
		// appearance of a non-repeated field.
		value, err := readSingle(member.Node, found[len(found)-1])
		return value, err == nil, err
	}
}

func readSingle(node structure.Node, appearance occurrence) (dynamic.Value, error) {
	switch shape := node.(type) {
	case structure.Scalar:
		return readScalar(shape, appearance)
	case structure.Object, structure.Union, structure.Reference:
		if appearance.kind != counted {
			return nil, fmt.Errorf("a message is length-delimited, and this field is wire type %d",
				appearance.kind)
		}
		return read(node, appearance.bytes)
	default:
		return nil, fmt.Errorf("%T has no proto3 form", shape)
	}
}
