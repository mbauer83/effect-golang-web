package schema

import "time"

// A schema drives encoding and pulls decoding, so a format implements these two
// contracts once and can then handle every schema. There is no intermediate
// tree: the schema calls straight into the format's writer, and reads straight
// from its reader.

// Sink receives the parts of a value as a schema encodes it.
//
// A schema calls these in the order its structure prescribes: BeginObject, then
// FieldName and a value for each field, then EndObject. An implementation may
// assume that order and does not need to validate it.
type Sink interface {
	Text(value string) error
	Integer(value int64) error
	Number(value float64) error
	Boolean(value bool) error
	Bytes(value []byte) error
	Timestamp(value time.Time) error

	// Null writes the absence of a value, for an optional field a format
	// represents explicitly rather than by omission.
	Null() error

	BeginObject() error
	// FieldName precedes each field's value.
	FieldName(name string) error
	EndObject() error

	BeginList() error
	EndList() error
}

// Source produces the parts of a value as a schema decodes it.
//
// Reading is driven by the schema, not by the document, with one exception:
// ReadObject hands over control per field, because a format may deliver fields
// in an order the schema did not declare.
type Source interface {
	Text() (string, error)
	Integer() (int64, error)
	Number() (float64, error)
	Boolean() (bool, error)
	Bytes() ([]byte, error)
	Timestamp() (time.Time, error)

	// Null reports whether the next value is the absence of one, consuming it
	// when it is.
	Null() (bool, error)

	// ReadObject calls decode once per field present, with that field's name.
	// The callback decodes the field's value or skips it; a name the schema
	// does not know is the callback's business, not the source's.
	ReadObject(decode func(name string) error) error

	// ReadList calls decode once per element, in order.
	ReadList(decode func() error) error

	// Skip discards the next value, whatever shape it has. It is how a decoder
	// tolerates a field it does not know.
	Skip() error
}
