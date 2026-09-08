package schema

// Reading a JSON value without being told its shape.
//
// This is the one place in the package that reads a document rather than being
// driven by a description, and it exists for one reason: a union told apart by
// a field inside it cannot be decoded any other way, because the field may
// arrive after the ones whose meaning it settles.
//
// It is a capability of the format and not of the schema, which is why it is an
// optional interface. JSON has a document to read; a format that streams does
// not, and says so.

import (
	"encoding/json/jsontext"
	"strings"

	"github.com/mbauer83/effect-golang-web/schema/dynamic"
)

// Buffer reads the next value whole.
func (source *jsonSource) Buffer() (dynamic.Value, error) {
	switch kind := source.decoder.PeekKind(); kind {
	case '{':
		return source.bufferObject()
	case '[':
		return source.bufferList()
	case 'n':
		if _, err := source.decoder.ReadToken(); err != nil {
			return nil, readFailure("reading a null", err)
		}
		return dynamic.Absent{}, nil
	case '"', '0', 't', 'f':
		return source.bufferScalar(kind)
	default:
		return nil, readFailure("reading a value", errUnreadableValue)
	}
}

// bufferedNumber reads a number as whole where it is written as one. A number
// too large for an int64 is kept as a number rather than refused: what it is
// for is up to the schema that reads it back.
func bufferedNumber(token jsontext.Token) (dynamic.Value, error) {
	if !strings.ContainsAny(token.String(), ".eE") {
		if whole, err := token.Int(); err == nil {
			return dynamic.Integer{Value: whole}, nil
		}
	}
	fraction, err := token.Float()
	if err != nil {
		return nil, fail("expected a number", err)
	}
	return dynamic.Number{Value: fraction}, nil
}

func (source *jsonSource) bufferObject() (dynamic.Value, error) {
	if _, err := source.decoder.ReadToken(); err != nil {
		return nil, readFailure("reading an object", err)
	}
	object := dynamic.Object{}
	for source.decoder.PeekKind() == '"' {
		token, err := source.decoder.ReadToken()
		if err != nil {
			return nil, readFailure("reading a field name", err)
		}
		// The name is taken out of the token before anything else is read: a
		// token is only good until the next call to the decoder.
		name := token.String()
		held, err := source.Buffer()
		if err != nil {
			return nil, within(name, err)
		}
		object.Fields = append(object.Fields, dynamic.Field{Name: name, Value: held})
	}
	if _, err := source.decoder.ReadToken(); err != nil {
		return nil, readFailure("reading the end of an object", err)
	}
	return object, nil
}

func (source *jsonSource) bufferList() (dynamic.Value, error) {
	if _, err := source.decoder.ReadToken(); err != nil {
		return nil, readFailure("reading a list", err)
	}
	list := dynamic.List{Elements: []dynamic.Value{}}
	for source.decoder.PeekKind() != ']' {
		held, err := source.Buffer()
		if err != nil {
			return nil, within(listIndex(len(list.Elements)), err)
		}
		list.Elements = append(list.Elements, held)
	}
	if _, err := source.decoder.ReadToken(); err != nil {
		return nil, readFailure("reading the end of a list", err)
	}
	return list, nil
}

// bufferScalar reads one scalar. A number is read as whole where it is written
// as one, because JSON does not distinguish the two and the schema that will
// read it back does -- so guessing from the text is closer than guessing from
// the value.
func (source *jsonSource) bufferScalar(kind jsontext.Kind) (dynamic.Value, error) {
	token, err := source.decoder.ReadToken()
	if err != nil {
		return nil, readFailure("reading "+describeKind(kind), err)
	}
	switch kind {
	case '"':
		return dynamic.Text{Value: token.String()}, nil
	case 't', 'f':
		return dynamic.Boolean{Value: token.Bool()}, nil
	default:
		return bufferedNumber(token)
	}
}
