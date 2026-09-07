package schema

// Decoding JSON. The schema pulls from the token stream, with one exception:
// an object hands control over per field, because JSON delivers fields in
// document order and a schema declares them in its own.

import (
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"io"
	"time"
)

// DecodeJSON reads a value from JSON.
func DecodeJSON[A any](schema Schema[A], document []byte) (A, error) {
	return DecodeJSONFrom(schema, bytes.NewReader(document))
}

// DecodeJSONFrom reads a value from a JSON stream, which is what a request body
// wants.
func DecodeJSONFrom[A any](schema Schema[A], from io.Reader) (A, error) {
	decoder := jsontext.NewDecoder(from)
	return Decode(schema, &jsonSource{decoder: decoder})
}

// jsonSource answers a schema's reads from JSON tokens.
type jsonSource struct {
	decoder *jsontext.Decoder
}

func (source *jsonSource) Text() (string, error) {
	token, err := source.expect('"')
	if err != nil {
		return "", err
	}
	return token.String(), nil
}

func (source *jsonSource) Integer() (int64, error) {
	token, err := source.expect('0')
	if err != nil {
		return 0, err
	}
	parsed, err := token.Int()
	if err != nil {
		// A JSON number that is not a whole number, or does not fit.
		return 0, fail("expected an integer", err)
	}
	return parsed, nil
}

func (source *jsonSource) Number() (float64, error) {
	token, err := source.expect('0')
	if err != nil {
		return 0, err
	}
	parsed, err := token.Float()
	if err != nil {
		return 0, fail("expected a number", err)
	}
	return parsed, nil
}

func (source *jsonSource) Boolean() (bool, error) {
	token, err := source.decoder.ReadToken()
	if err != nil {
		return false, readFailure("expected a boolean", err)
	}
	if kind := token.Kind(); kind != 't' && kind != 'f' {
		return false, fail("expected a boolean, found "+describeKind(kind), nil)
	}
	return token.Bool(), nil
}

func (source *jsonSource) Bytes() ([]byte, error) {
	encoded, err := source.Text()
	if err != nil {
		return nil, err
	}
	return decodeTextAsBytes(encoded)
}

func (source *jsonSource) Timestamp() (time.Time, error) {
	encoded, err := source.Text()
	if err != nil {
		return time.Time{}, err
	}
	parsed, err := time.Parse(time.RFC3339, encoded)
	if err != nil {
		return time.Time{}, fail("expected an RFC 3339 timestamp", err)
	}
	return parsed, nil
}

// Null consumes a null when the next value is one, and otherwise leaves the
// stream alone so the caller can read the value it expected.
func (source *jsonSource) Null() (bool, error) {
	if source.decoder.PeekKind() != 'n' {
		return false, nil
	}
	if _, err := source.decoder.ReadToken(); err != nil {
		return false, readFailure("expected null", err)
	}
	return true, nil
}

func (source *jsonSource) ReadObject(decode func(name string) error) error {
	if _, err := source.expect('{'); err != nil {
		return err
	}
	for source.decoder.PeekKind() != '}' {
		name, err := source.Text()
		if err != nil {
			return err
		}
		if err := decode(name); err != nil {
			return err
		}
	}
	_, err := source.expect('}')
	return err
}

func (source *jsonSource) ReadList(decode func() error) error {
	if _, err := source.expect('['); err != nil {
		return err
	}
	for source.decoder.PeekKind() != ']' {
		if err := decode(); err != nil {
			return err
		}
	}
	_, err := source.expect(']')
	return err
}

// Skip discards the next value whatever its shape, which is how a decoder
// tolerates a field written by a newer version of its producer.
func (source *jsonSource) Skip() error {
	if err := source.decoder.SkipValue(); err != nil {
		return readFailure("could not skip a value", err)
	}
	return nil
}

// expect reads the next token and requires it to be of one kind, so every read
// reports the same way and no caller has to remember to check.
func (source *jsonSource) expect(kind jsontext.Kind) (jsontext.Token, error) {
	token, err := source.decoder.ReadToken()
	if err != nil {
		return jsontext.Token{}, readFailure("expected "+describeKind(kind), err)
	}
	if token.Kind() != kind {
		return jsontext.Token{}, fail(
			"expected "+describeKind(kind)+", found "+describeKind(token.Kind()), nil)
	}
	return token.Clone(), nil
}

// readFailure distinguishes a truncated document from a mis-shaped one, because
// the two have different causes and different fixes.
func readFailure(reason string, err error) error {
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return fail(reason+", but the document ended", err)
	}
	return fail(reason, err)
}

func describeKind(kind jsontext.Kind) string {
	switch kind {
	case '"':
		return "a string"
	case '0':
		return "a number"
	case 't', 'f':
		return "a boolean"
	case 'n':
		return "null"
	case '{':
		return "an object"
	case '}':
		return "the end of an object"
	case '[':
		return "an array"
	case ']':
		return "the end of an array"
	default:
		return "a value"
	}
}
