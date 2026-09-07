package schema

import (
	"bytes"
	"encoding/json/jsontext"
	"io"
	"time"
)

// The JSON format, over the standard library's token API. The schema drives the
// writer and pulls from the reader, so a value never passes through an
// intermediate tree on its way to or from the wire.

// EncodeJSON writes a value as JSON.
func EncodeJSON[A any](schema Schema[A], value A) ([]byte, error) {
	var written bytes.Buffer
	if err := EncodeJSONTo(schema, value, &written); err != nil {
		return nil, err
	}
	return written.Bytes(), nil
}

// EncodeJSONTo writes a value as JSON to a writer, which is what a response
// body wants: no buffer the size of the document.
func EncodeJSONTo[A any](schema Schema[A], value A, to io.Writer) error {
	encoder := jsontext.NewEncoder(to)
	if err := Encode(schema, value, &jsonSink{encoder: encoder}); err != nil {
		return err
	}
	return nil
}

// jsonSink writes the calls a schema makes as JSON tokens.
type jsonSink struct {
	encoder *jsontext.Encoder
}

func (sink *jsonSink) Text(value string) error {
	return sink.encoder.WriteToken(jsontext.String(value))
}

func (sink *jsonSink) Integer(value int64) error {
	return sink.encoder.WriteToken(jsontext.Int(value))
}

func (sink *jsonSink) Number(value float64) error {
	return sink.encoder.WriteToken(jsontext.Float(value))
}

func (sink *jsonSink) Boolean(value bool) error {
	return sink.encoder.WriteToken(jsontext.Bool(value))
}

// Bytes is carried base64 encoded, which is what the schema's "byte"
// refinement tells a reader of the projection to expect.
func (sink *jsonSink) Bytes(value []byte) error {
	return sink.encoder.WriteToken(jsontext.String(encodeBytesAsText(value)))
}

// Timestamp is carried as RFC 3339, matching the "date-time" refinement.
func (sink *jsonSink) Timestamp(value time.Time) error {
	return sink.encoder.WriteToken(jsontext.String(value.Format(time.RFC3339Nano)))
}

func (sink *jsonSink) Null() error {
	return sink.encoder.WriteToken(jsontext.Null)
}

func (sink *jsonSink) BeginObject() error {
	return sink.encoder.WriteToken(jsontext.BeginObject)
}

func (sink *jsonSink) FieldName(name string) error {
	return sink.encoder.WriteToken(jsontext.String(name))
}

func (sink *jsonSink) EndObject() error {
	return sink.encoder.WriteToken(jsontext.EndObject)
}

func (sink *jsonSink) BeginList() error {
	return sink.encoder.WriteToken(jsontext.BeginArray)
}

func (sink *jsonSink) EndList() error {
	return sink.encoder.WriteToken(jsontext.EndArray)
}
