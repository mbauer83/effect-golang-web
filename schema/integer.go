package schema

// The precise numeric schemas.
//
// The wire carries two numeric shapes and Go has twelve. A description that
// only said "a whole number" would leave a generator to guess a width and
// leave a published contract stating a range wider than the program will
// accept, so each Go type has its own constructor.
//
// Two things follow from the constructor being typed, and both are the point.
// A bound outside the type's range is a compile error rather than a runtime
// surprise -- AtMost(Int8(), 200) does not build, because 200 is not an int8 --
// and the range the type implies is recorded, so the contract states it without
// anyone writing it down.
//
// The wire shape is derived and not declared: every one of these is an integer
// or a number on the wire, and a format reads that and needs to know nothing
// about the width.

import (
	"math"

	"github.com/mbauer83/effect-golang-web/schema/structure"
)

// Int8 describes a signed 8-bit integer.
func Int8() Schema[int8] {
	return narrowed[int8](structure.Int8Bits, math.MinInt8, math.MaxInt8)
}

// Int16 describes a signed 16-bit integer.
func Int16() Schema[int16] {
	return narrowed[int16](structure.Int16Bits, math.MinInt16, math.MaxInt16)
}

// Int32 describes a signed 32-bit integer.
func Int32() Schema[int32] {
	return narrowed[int32](structure.Int32Bits, math.MinInt32, math.MaxInt32)
}

// Uint8 describes an unsigned 8-bit integer.
func Uint8() Schema[uint8] {
	return narrowed[uint8](structure.Uint8Bits, 0, math.MaxUint8)
}

// Uint16 describes an unsigned 16-bit integer.
func Uint16() Schema[uint16] {
	return narrowed[uint16](structure.Uint16Bits, 0, math.MaxUint16)
}

// Uint32 describes an unsigned 32-bit integer.
func Uint32() Schema[uint32] {
	return narrowed[uint32](structure.Uint32Bits, 0, math.MaxUint32)
}

// Uint describes a platform unsigned integer.
//
// Its upper bound is the largest signed 64-bit value, not the largest unsigned
// one: the wire carries a signed integer, so a value above that cannot be
// expressed at all and a schema that claimed otherwise would be lying about
// what it can carry.
func Uint() Schema[uint] {
	return narrowed[uint](structure.UintBits, 0, math.MaxInt64)
}

// Uint64 describes an unsigned 64-bit integer, bounded as Uint is and for the
// same reason.
func Uint64() Schema[uint64] {
	return narrowed[uint64](structure.Uint64Bits, 0, math.MaxInt64)
}

// Float32 describes a 32-bit floating-point number.
//
// Decoding rejects a value the width cannot hold, rather than quietly turning
// it into an infinity.
func Float32() Schema[float32] {
	return constrainedFloat(
		TransformOrFail(Float64(),
			func(value float64) (float32, error) {
				if value < -math.MaxFloat32 || value > math.MaxFloat32 {
					return 0, fail("number does not fit in a float32", nil)
				}
				return float32(value), nil
			},
			func(value float32) (float64, error) { return float64(value), nil },
		),
		structure.Float32Bits, -math.MaxFloat32, math.MaxFloat32)
}

// whole is the Go integer types a precise schema is written over.
type whole interface {
	~int8 | ~int16 | ~int32 | ~uint8 | ~uint16 | ~uint32 | ~uint | ~uint64
}

// narrowed builds a schema for a Go integer narrower than the wire's, refusing
// a value the type cannot hold and recording the range it implies.
func narrowed[A whole](precision structure.Precision, lowest int64, highest int64) Schema[A] {
	converted := TransformOrFail(Int64(),
		func(value int64) (A, error) {
			if value < lowest || value > highest {
				return 0, fail("integer does not fit in a "+precision.String(), nil)
			}
			return A(value), nil
		},
		func(value A) (int64, error) { return int64(value), nil },
	)
	return withPrecision(
		AtMost(AtLeast(converted, A(lowest)), A(highest)),
		precision)
}

// constrainedFloat records a float's width and the range it implies.
func constrainedFloat(
	converted Schema[float32],
	precision structure.Precision,
	lowest float32,
	highest float32,
) Schema[float32] {
	return withPrecision(AtMost(AtLeast(converted, lowest), highest), precision)
}

// withPrecision records the Go representation in the description. The wire
// shape is untouched: a format reads the Kind, and this is for a generator and
// for a projection that can state the range.
func withPrecision[A any](shape Schema[A], precision structure.Precision) Schema[A] {
	scalar, isScalar := shape.node.(structure.Scalar)
	if !isScalar {
		return faulted[A](shape.node, fail("a precision applies to a scalar", nil))
	}
	// The wire kind is derived from the width, so the two cannot be set to
	// disagree: a width on text is a description contradicting itself.
	if kind, numeric := precision.Numeric(); !numeric || kind != scalar.Kind {
		return faulted[A](shape.node,
			fail("a width of "+precision.String()+" does not describe "+scalar.Kind.String(), nil))
	}
	scalar.Precision = precision
	return of(scalar, shape.encode, shape.decode)
}
