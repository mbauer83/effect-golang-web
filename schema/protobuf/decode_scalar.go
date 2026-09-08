package protobuf

// One scalar off the wire, a union's chosen member, and the zero a map entry
// falls back to.

import (
	"fmt"
	"math"
	"time"

	"github.com/mbauer83/effect-golang-web/schema/dynamic"
	"github.com/mbauer83/effect-golang-web/schema/structure"
)

// readUnion reads the one member of a oneof that was written.
//
// Protobuf identifies the chosen member by its field number, so the choice is
// in the wire and needs no discriminating field -- which is why a description
// carrying one is refused rather than projected. If several were written, the
// last wins, which is what protobuf says of a oneof.
func readUnion(union structure.Union, bytes []byte) (dynamic.Value, error) {
	if union.Discriminator != "" {
		return nil, errDiscriminated
	}
	byNumber := map[int]structure.Variant{}
	known := map[int]bool{}
	for _, variant := range union.Variants {
		if variant.Number < 1 {
			return nil, fmt.Errorf("variant %q of %s: %w", variant.Name, union.Name, errUnnumbered)
		}
		byNumber[variant.Number] = variant
		known[variant.Number] = true
	}

	found, err := gathered(known, bytes)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", union.Name, err)
	}

	chosen, last := lastWritten(union, found)
	if !last {
		return nil, fmt.Errorf("%s is a choice of one, and the message chose none", union.Name)
	}
	variant := byNumber[chosen]
	value, err := readSingle(variant.Node, found[chosen][len(found[chosen])-1])
	if err != nil {
		return nil, fmt.Errorf("variant %q of %s: %w", variant.Name, union.Name, err)
	}
	// A union's value is the one-member object its wire form is, and the
	// member's name is the variant's, which is what the rest of this module
	// already reads.
	return dynamic.Object{Fields: []dynamic.Field{{Name: variant.Name, Value: value}}}, nil
}

// lastWritten is the variant number written last, which is the one that counts.
//
// The order the variants were declared in is not the order the wire carried
// them, so this walks the declaration and takes the one that was present --
// which is unambiguous because a well-formed oneof carries exactly one.
func lastWritten(union structure.Union, found map[int][]occurrence) (int, bool) {
	chosen, written := 0, false
	for _, variant := range union.Variants {
		if len(found[variant.Number]) > 0 {
			chosen, written = variant.Number, true
		}
	}
	return chosen, written
}

func readScalar(shape structure.Scalar, appearance occurrence) (dynamic.Value, error) {
	switch shape.Kind {
	case structure.Text:
		if appearance.kind != counted {
			return nil, layout("a string", appearance.kind)
		}
		return dynamic.Text{Value: string(appearance.bytes)}, nil
	case structure.Bytes:
		if appearance.kind != counted {
			return nil, layout("bytes", appearance.kind)
		}
		return dynamic.Bytes{Value: appearance.bytes}, nil
	case structure.Boolean:
		if appearance.kind != varying {
			return nil, layout("a bool", appearance.kind)
		}
		return dynamic.Boolean{Value: appearance.fixed != 0}, nil
	case structure.Integer:
		if appearance.kind != varying {
			return nil, layout("a whole number", appearance.kind)
		}
		return dynamic.Integer{Value: int64(appearance.fixed)}, nil
	case structure.Number:
		return readFractional(shape, appearance)
	case structure.Timestamp:
		if appearance.kind != counted {
			return nil, layout("an instant", appearance.kind)
		}
		return readInstant(appearance.bytes)
	default:
		return nil, fmt.Errorf("kind %v has no proto3 form", shape.Kind)
	}
}

// readFractional reads a float or a double.
//
// The width is the description's, not the wire's: four bytes and eight are
// different layouts, and a reader told the wrong one reads a different number
// rather than failing -- so the field is refused when the two disagree.
func readFractional(shape structure.Scalar, appearance occurrence) (dynamic.Value, error) {
	if shape.Precision == structure.Float32Bits {
		if appearance.kind != fourBytes {
			return nil, layout("a float", appearance.kind)
		}
		return dynamic.Number{Value: float64(math.Float32frombits(uint32(appearance.fixed)))}, nil
	}
	if appearance.kind != eightBytes {
		return nil, layout("a double", appearance.kind)
	}
	return dynamic.Number{Value: math.Float64frombits(appearance.fixed)}, nil
}

// readInstant reads a google.protobuf.Timestamp: seconds in field 1 and
// nanoseconds in field 2.
func readInstant(bytes []byte) (dynamic.Value, error) {
	found, err := gathered(map[int]bool{1: true, 2: true}, bytes)
	if err != nil {
		return nil, err
	}
	seconds, nanos := int64(0), int64(0)
	if held := found[1]; len(held) > 0 {
		seconds = int64(held[len(held)-1].fixed)
	}
	if held := found[2]; len(held) > 0 {
		nanos = int64(held[len(held)-1].fixed)
	}
	return dynamic.Timestamp{Value: time.Unix(seconds, nanos).UTC()}, nil
}

// unpacked reads the elements out of a packed field, which carries them with no
// tags of their own.
func unpacked(shape structure.Scalar, bytes []byte) ([]dynamic.Value, error) {
	from := &reader{bytes: bytes}
	elements := []dynamic.Value{}
	for !from.done() {
		value, err := unpackedOne(from, shape)
		if err != nil {
			return nil, err
		}
		elements = append(elements, value)
	}
	return elements, nil
}

func unpackedOne(from *reader, shape structure.Scalar) (dynamic.Value, error) {
	switch {
	case shape.Kind == structure.Boolean:
		held, err := from.varint()
		return dynamic.Boolean{Value: held != 0}, err
	case shape.Kind == structure.Integer:
		held, err := from.varint()
		return dynamic.Integer{Value: int64(held)}, err
	case shape.Precision == structure.Float32Bits:
		held, err := from.fixed32()
		return dynamic.Number{Value: float64(math.Float32frombits(held))}, err
	default:
		held, err := from.fixed64()
		return dynamic.Number{Value: math.Float64frombits(held)}, err
	}
}

// zeroOf is the value proto3 says a field of that shape holds when the wire
// carries nothing for it, and whether it says anything at all.
//
// This is not a default invented here: it is what the format means. A field
// with implicit presence -- proto3's ordinary field -- writes no bytes when it
// holds its zero, so absent and zero are the same message and there is no
// reading in which the field was not sent. A field with explicit presence
// (optional, or a message) is different: there absent is a state of its own,
// which is what the keyword buys.
//
// The description's own rules still apply to the value that results, which is
// where a zero gets refused: a page count of at least one rejects the zero the
// wire implied, exactly as it would reject a zero the wire spelled out.
func zeroOf(node structure.Node) (dynamic.Value, bool) {
	scalar, isScalar := node.(structure.Scalar)
	if !isScalar {
		// A message field has explicit presence in proto3, so there is no
		// zero message: absent is absent.
		return dynamic.Absent{}, false
	}
	switch scalar.Kind {
	case structure.Integer:
		return dynamic.Integer{Value: 0}, true
	case structure.Number:
		return dynamic.Number{Value: 0}, true
	case structure.Boolean:
		return dynamic.Boolean{Value: false}, true
	case structure.Bytes:
		return dynamic.Bytes{Value: []byte{}}, true
	case structure.Timestamp:
		// A Timestamp is a message, so the same rule as any other.
		return dynamic.Absent{}, false
	default:
		return dynamic.Text{Value: ""}, true
	}
}

func layout(wanted string, kind wireType) error {
	return fmt.Errorf("the description says %s, and this field is wire type %d", wanted, kind)
}
