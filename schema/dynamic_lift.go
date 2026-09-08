package schema

// Carrying each scalar case into the universal representation and back. One
// pair per case, and nothing else needs to know the sum has cases.

import (
	"time"

	"github.com/mbauer83/effect-golang-web/schema/dynamic"
)

func toText(value string) (dynamic.Value, error) { return dynamic.Text{Value: value}, nil }

func fromText(value dynamic.Value) (string, error) {
	held, is := value.(dynamic.Text)
	if !is {
		return "", fail("is not text", nil)
	}
	return held.Value, nil
}

func toInteger(value int64) (dynamic.Value, error) { return dynamic.Integer{Value: value}, nil }

// fromInteger accepts a number whose value is whole, because a buffered value
// came from a format that had to guess which of the two a number was.
func fromInteger(value dynamic.Value) (int64, error) {
	switch held := value.(type) {
	case dynamic.Integer:
		return held.Value, nil
	case dynamic.Number:
		if whole := int64(held.Value); float64(whole) == held.Value {
			return whole, nil
		}
		return 0, fail("is not a whole number", nil)
	default:
		return 0, fail("is not a whole number", nil)
	}
}

func toNumber(value float64) (dynamic.Value, error) { return dynamic.Number{Value: value}, nil }

func fromNumber(value dynamic.Value) (float64, error) {
	switch held := value.(type) {
	case dynamic.Number:
		return held.Value, nil
	case dynamic.Integer:
		return float64(held.Value), nil
	default:
		return 0, fail("is not a number", nil)
	}
}

func toBoolean(value bool) (dynamic.Value, error) { return dynamic.Boolean{Value: value}, nil }

func fromBoolean(value dynamic.Value) (bool, error) {
	held, is := value.(dynamic.Boolean)
	if !is {
		return false, fail("is not a boolean", nil)
	}
	return held.Value, nil
}

func toBytes(value []byte) (dynamic.Value, error) { return dynamic.Bytes{Value: value}, nil }

func fromBytes(value dynamic.Value) ([]byte, error) {
	held, is := value.(dynamic.Bytes)
	if !is {
		return nil, fail("is not a byte string", nil)
	}
	return held.Value, nil
}

func toTimestamp(value time.Time) (dynamic.Value, error) {
	return dynamic.Timestamp{Value: value}, nil
}

func fromTimestamp(value dynamic.Value) (time.Time, error) {
	held, is := value.(dynamic.Timestamp)
	if !is {
		return time.Time{}, fail("is not a timestamp", nil)
	}
	return held.Value, nil
}
