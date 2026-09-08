package protobuf

// The protobuf wire format's own primitives.
//
// Four wire types and a varint are the whole of it. This is here rather than
// taken from a library because the schema layer carries no third-party
// dependency: the format is a published specification, and the canonical
// implementation appears in the tests, where it checks that what this writes is
// what protobuf reads.

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

// wireType says how a field's bytes are laid out. The five the format defines
// are these four and the two deprecated group markers, which nothing emits.
type wireType uint8

const (
	// varying is a variable-length integer: ints, bools, enums.
	varying wireType = 0
	// eightBytes is a fixed 64-bit value: a double, or a fixed64.
	eightBytes wireType = 1
	// counted is a length and then that many bytes: strings, bytes, messages,
	// and a packed repeated field.
	counted wireType = 2
	// fourBytes is a fixed 32-bit value: a float, or a fixed32.
	fourBytes wireType = 5
)

// writer accumulates one message's bytes.
//
// A message is buffered because its length precedes it: a nested message's tag
// carries the byte count, so there is nothing to write until the whole of it is
// known. That is the format's requirement rather than a choice made here, and
// it is the reason this codec works over a whole value rather than through the
// streaming sink the other formats use.
type writer struct {
	bytes []byte
}

func (into *writer) varint(value uint64) {
	into.bytes = binary.AppendUvarint(into.bytes, value)
}

func (into *writer) tag(number int, kind wireType) {
	into.varint(uint64(number)<<3 | uint64(kind))
}

func (into *writer) block(number int, held []byte) {
	into.tag(number, counted)
	into.varint(uint64(len(held)))
	into.bytes = append(into.bytes, held...)
}

func (into *writer) fixed64(value uint64) {
	into.bytes = binary.LittleEndian.AppendUint64(into.bytes, value)
}

func (into *writer) fixed32(value uint32) {
	into.bytes = binary.LittleEndian.AppendUint32(into.bytes, value)
}

func (into *writer) double(value float64) {
	into.fixed64(math.Float64bits(value))
}

func (into *writer) float(value float32) {
	into.fixed32(math.Float32bits(value))
}

// reader walks one message's bytes.
type reader struct {
	bytes []byte
	at    int
}

func (from *reader) done() bool {
	return from.at >= len(from.bytes)
}

func (from *reader) varint() (uint64, error) {
	value, read := binary.Uvarint(from.bytes[from.at:])
	if read <= 0 {
		return 0, errTruncated
	}
	from.at += read
	return value, nil
}

// tag reads a field's number and layout.
func (from *reader) tag() (int, wireType, error) {
	key, err := from.varint()
	if err != nil {
		return 0, 0, err
	}
	number := int(key >> 3)
	if number < 1 {
		return 0, 0, fmt.Errorf("field number %d is not a field number", number)
	}
	return number, wireType(key & 7), nil
}

func (from *reader) block() ([]byte, error) {
	length, err := from.varint()
	if err != nil {
		return nil, err
	}
	if uint64(len(from.bytes)-from.at) < length {
		return nil, errTruncated
	}
	held := from.bytes[from.at : from.at+int(length)]
	from.at += int(length)
	return held, nil
}

func (from *reader) fixed64() (uint64, error) {
	if len(from.bytes)-from.at < 8 {
		return 0, errTruncated
	}
	value := binary.LittleEndian.Uint64(from.bytes[from.at:])
	from.at += 8
	return value, nil
}

func (from *reader) fixed32() (uint32, error) {
	if len(from.bytes)-from.at < 4 {
		return 0, errTruncated
	}
	value := binary.LittleEndian.Uint32(from.bytes[from.at:])
	from.at += 4
	return value, nil
}

// skip discards a field of the given layout.
//
// It is what makes a message written by a newer program readable by an older
// one, which is the property protobuf is chosen for: an unknown field is passed
// over rather than being an error.
func (from *reader) skip(kind wireType) error {
	switch kind {
	case varying:
		_, err := from.varint()
		return err
	case eightBytes:
		_, err := from.fixed64()
		return err
	case fourBytes:
		_, err := from.fixed32()
		return err
	case counted:
		_, err := from.block()
		return err
	default:
		return fmt.Errorf("wire type %d is not one this format defines", kind)
	}
}

var errTruncated = errors.New("the message ends in the middle of a field")
