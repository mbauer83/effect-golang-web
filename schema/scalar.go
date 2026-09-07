package schema

import (
	"encoding/base64"
	"math"
	"time"

	"github.com/mbauer83/effect-golang-web/schema/structure"
)

// The scalar schemas. Each pairs one structure node with the sink call and the
// source call for that shape, so a format never has to guess what a value is.

// Text describes a UTF-8 string.
func Text() Schema[string] {
	return of(
		structure.Scalar{Kind: structure.Text},
		func(value string, into Sink) error { return into.Text(value) },
		func(from Source) (string, error) { return from.Text() },
	)
}

// Formatted describes a string a projection should refine, such as "uuid",
// "email" or "date-time". The refinement is a hint for readers of the
// projection; it is not validated here, because a claim a schema cannot enforce
// should not look like one it does.
func Formatted(format string) Schema[string] {
	return of(
		structure.Scalar{Kind: structure.Text, Format: format},
		func(value string, into Sink) error { return into.Text(value) },
		func(from Source) (string, error) { return from.Text() },
	)
}

// Int describes a platform int, narrowed on decode so a value that does not fit
// is rejected rather than silently truncated.
func Int() Schema[int] {
	return TransformOrFail(Int64(),
		func(value int64) (int, error) {
			narrowed := int(value)
			if int64(narrowed) != value {
				return 0, fail("integer does not fit in an int", nil)
			}
			return narrowed, nil
		},
		func(value int) (int64, error) { return int64(value), nil },
	)
}

// Int64 describes a 64-bit signed integer.
func Int64() Schema[int64] {
	return of(
		structure.Scalar{Kind: structure.Integer},
		func(value int64, into Sink) error { return into.Integer(value) },
		func(from Source) (int64, error) { return from.Integer() },
	)
}

// Float64 describes a 64-bit floating-point number.
//
// NaN and the infinities are rejected on encode: JSON cannot represent them, and
// a schema that silently produced invalid JSON would be worse than one that
// says so.
func Float64() Schema[float64] {
	return of(
		structure.Scalar{Kind: structure.Number},
		func(value float64, into Sink) error {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return fail("number is not finite", nil)
			}
			return into.Number(value)
		},
		func(from Source) (float64, error) { return from.Number() },
	)
}

// Bool describes a boolean.
func Bool() Schema[bool] {
	return of(
		structure.Scalar{Kind: structure.Boolean},
		func(value bool, into Sink) error { return into.Boolean(value) },
		func(from Source) (bool, error) { return from.Boolean() },
	)
}

// Bytes describes an opaque byte string. A text format carries it base64
// encoded, which is what the "byte" refinement records for readers of a
// projection.
func Bytes() Schema[[]byte] {
	return of(
		structure.Scalar{Kind: structure.Bytes, Format: "byte"},
		func(value []byte, into Sink) error { return into.Bytes(value) },
		func(from Source) ([]byte, error) { return from.Bytes() },
	)
}

// Time describes an instant. A text format carries it as RFC 3339.
func Time() Schema[time.Time] {
	return of(
		structure.Scalar{Kind: structure.Timestamp, Format: "date-time"},
		func(value time.Time, into Sink) error { return into.Timestamp(value) },
		func(from Source) (time.Time, error) { return from.Timestamp() },
	)
}

// encodeBytesAsText and decodeTextAsBytes are the conversion a text format uses
// for Bytes, kept here so every text format agrees on it.
func encodeBytesAsText(value []byte) string {
	return base64.StdEncoding.EncodeToString(value)
}

func decodeTextAsBytes(value string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, fail("not valid base64", err)
	}
	return decoded, nil
}
