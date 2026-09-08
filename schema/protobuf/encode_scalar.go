package protobuf

// One scalar on the wire.
//
// The Precision is what decides the layout, which makes this the one place a
// Go-side detail reaches the wire: int32 and int64 are the same varint, but
// float and double are four bytes and eight, and a reader told the wrong one
// reads the wrong value. Everywhere else the Kind is enough.

import (
	"fmt"
	"time"

	"github.com/mbauer83/effect-golang-web/schema/dynamic"
	"github.com/mbauer83/effect-golang-web/schema/structure"
)

// scalar writes one scalar value under its field number.
//
// present says the field has explicit presence, which decides what happens to a
// zero. proto3 omits the zero of an implicit-presence field -- that is what
// makes a default cost no bytes -- and writes it for one with presence, because
// there the difference between zero and absent is the point.
func scalar(
	into *writer,
	number int,
	shape structure.Scalar,
	value dynamic.Value,
	present bool,
) error {
	switch shape.Kind {
	case structure.Text:
		held, ok := value.(dynamic.Text)
		if !ok {
			return mismatched("a string", value)
		}
		if held.Value == "" && !present {
			return nil
		}
		into.block(number, []byte(held.Value))
	case structure.Bytes:
		held, ok := value.(dynamic.Bytes)
		if !ok {
			return mismatched("bytes", value)
		}
		if len(held.Value) == 0 && !present {
			return nil
		}
		into.block(number, held.Value)
	case structure.Boolean:
		held, ok := value.(dynamic.Boolean)
		if !ok {
			return mismatched("a bool", value)
		}
		if !held.Value && !present {
			return nil
		}
		into.tag(number, varying)
		into.varint(boolean(held.Value))
	case structure.Integer:
		return whole(into, number, shape, value, present)
	case structure.Number:
		return fractional(into, number, shape, value, present)
	case structure.Timestamp:
		held, ok := value.(dynamic.Timestamp)
		if !ok {
			return mismatched("an instant", value)
		}
		if held.Value.IsZero() && !present {
			return nil
		}
		into.block(number, instant(held.Value))
	default:
		return fmt.Errorf("kind %v has no proto3 form", shape.Kind)
	}
	return nil
}

func whole(
	into *writer,
	number int,
	shape structure.Scalar,
	value dynamic.Value,
	present bool,
) error {
	held, ok := value.(dynamic.Integer)
	if !ok {
		return mismatched("a whole number", value)
	}
	if held.Value == 0 && !present {
		return nil
	}
	into.tag(number, varying)
	into.varint(varintOf(held.Value))
	return nil
}

// varintOf is the varint a whole number becomes.
//
// Two's complement, which sign-extends a negative to sixty-four bits: that is
// what protobuf does, and the reason a negative int32 costs ten bytes on the
// wire. The format's int32 and int64 are the same varint, so the width is a
// statement about range rather than about layout -- which is why the precision
// does not appear here even though it decides the projected type.
func varintOf(value int64) uint64 {
	return uint64(value)
}

func fractional(
	into *writer,
	number int,
	shape structure.Scalar,
	value dynamic.Value,
	present bool,
) error {
	held, ok := value.(dynamic.Number)
	if !ok {
		return mismatched("a number", value)
	}
	if held.Value == 0 && !present {
		return nil
	}
	if shape.Precision == structure.Float32Bits {
		into.tag(number, fourBytes)
		into.float(float32(held.Value))
		return nil
	}
	into.tag(number, eightBytes)
	into.double(held.Value)
	return nil
}

// packedOne writes one element of a packed field: its value, with no tag.
func packedOne(into *writer, shape structure.Scalar, value dynamic.Value) error {
	switch shape.Kind {
	case structure.Boolean:
		held, ok := value.(dynamic.Boolean)
		if !ok {
			return mismatched("a bool", value)
		}
		into.varint(boolean(held.Value))
	case structure.Integer:
		held, ok := value.(dynamic.Integer)
		if !ok {
			return mismatched("a whole number", value)
		}
		into.varint(varintOf(held.Value))
	case structure.Number:
		held, ok := value.(dynamic.Number)
		if !ok {
			return mismatched("a number", value)
		}
		if shape.Precision == structure.Float32Bits {
			into.float(float32(held.Value))
			return nil
		}
		into.double(held.Value)
	default:
		return fmt.Errorf("kind %v is not packable", shape.Kind)
	}
	return nil
}

// instant is the google.protobuf.Timestamp a time is: seconds in field 1 and
// nanoseconds in field 2, which is the well-known type's own definition.
func instant(moment time.Time) []byte {
	into := &writer{}
	if seconds := moment.Unix(); seconds != 0 {
		into.tag(1, varying)
		into.varint(uint64(seconds))
	}
	if nanos := moment.Nanosecond(); nanos != 0 {
		into.tag(2, varying)
		into.varint(uint64(nanos))
	}
	return into.bytes
}

func boolean(value bool) uint64 {
	if value {
		return 1
	}
	return 0
}

func mismatched(wanted string, value dynamic.Value) error {
	return fmt.Errorf("the description says %s and the value is a %T", wanted, value)
}
