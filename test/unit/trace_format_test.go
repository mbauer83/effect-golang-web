package unit

// A second format, so that "one description serves every format" is something
// this module demonstrates rather than asserts. It is also the clearest
// statement of what a format author has to implement: a Sink is these twelve
// calls, and a Source is their mirror.
//
// The format is a flat token trace. It is not useful on a wire, which is the
// point: it shares nothing with JSON but the schema.

import (
	"encoding/base64"
	"strconv"
	"strings"
	"time"

	"github.com/mbauer83/effect-golang-web/schema"
)

type traceSink struct{ tokens []string }

func (sink *traceSink) put(token string) error {
	sink.tokens = append(sink.tokens, token)
	return nil
}

func (sink *traceSink) Text(value string) error { return sink.put("s:" + value) }
func (sink *traceSink) Integer(value int64) error {
	return sink.put("i:" + strconv.FormatInt(value, 10))
}
func (sink *traceSink) Boolean(value bool) error    { return sink.put("b:" + strconv.FormatBool(value)) }
func (sink *traceSink) Null() error                 { return sink.put("null") }
func (sink *traceSink) BeginObject() error          { return sink.put("{") }
func (sink *traceSink) FieldName(name string) error { return sink.put("k:" + name) }
func (sink *traceSink) EndObject() error            { return sink.put("}") }
func (sink *traceSink) BeginList() error            { return sink.put("[") }
func (sink *traceSink) EndList() error              { return sink.put("]") }

func (sink *traceSink) Number(value float64) error {
	return sink.put("n:" + strconv.FormatFloat(value, 'g', -1, 64))
}

func (sink *traceSink) Bytes(value []byte) error {
	return sink.put("y:" + base64.StdEncoding.EncodeToString(value))
}

func (sink *traceSink) Timestamp(value time.Time) error {
	return sink.put("t:" + value.UTC().Format(time.RFC3339Nano))
}

type traceSource struct {
	tokens []string
	at     int
}

func (source *traceSource) take(prefix string, wanted string) (string, error) {
	if source.at >= len(source.tokens) {
		return "", errTraceExhausted
	}
	token := source.tokens[source.at]
	if !strings.HasPrefix(token, prefix) {
		return "", errTraceWanted(wanted, token)
	}
	source.at++
	return strings.TrimPrefix(token, prefix), nil
}

func (source *traceSource) Text() (string, error) { return source.take("s:", "text") }
func (source *traceSource) peek() string {
	if source.at >= len(source.tokens) {
		return ""
	}
	return source.tokens[source.at]
}

func (source *traceSource) Integer() (int64, error) {
	token, err := source.take("i:", "an integer")
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(token, 10, 64)
}

func (source *traceSource) Number() (float64, error) {
	token, err := source.take("n:", "a number")
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(token, 64)
}

func (source *traceSource) Boolean() (bool, error) {
	token, err := source.take("b:", "a boolean")
	if err != nil {
		return false, err
	}
	return strconv.ParseBool(token)
}

func (source *traceSource) Bytes() ([]byte, error) {
	token, err := source.take("y:", "bytes")
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(token)
}

func (source *traceSource) Timestamp() (time.Time, error) {
	token, err := source.take("t:", "a timestamp")
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse(time.RFC3339Nano, token)
}

// Null answers whether the value is absent and consumes it if so, which is the
// contract that lets one schema describe a nullable without the format knowing
// what is inside it.
func (source *traceSource) Null() (bool, error) {
	if source.peek() != "null" {
		return false, nil
	}
	source.at++
	return true, nil
}

func (source *traceSource) ReadObject(decode func(name string) error) error {
	if _, err := source.take("{", "an object"); err != nil {
		return err
	}
	for strings.HasPrefix(source.peek(), "k:") {
		name, err := source.take("k:", "a field name")
		if err != nil {
			return err
		}
		if err := decode(name); err != nil {
			return err
		}
	}
	_, err := source.take("}", "the end of the object")
	return err
}

func (source *traceSource) ReadList(decode func() error) error {
	if _, err := source.take("[", "a list"); err != nil {
		return err
	}
	for source.peek() != "]" {
		if source.peek() == "" {
			return errTraceExhausted
		}
		if err := decode(); err != nil {
			return err
		}
	}
	_, err := source.take("]", "the end of the list")
	return err
}

// Skip steps over one whole value, however deep, which is what lets a decoder
// tolerate a field it does not know.
func (source *traceSource) Skip() error {
	depth := 0
	for {
		token := source.peek()
		if token == "" {
			return errTraceExhausted
		}
		source.at++
		switch token {
		case "{", "[":
			depth++
		case "}", "]":
			depth--
		}
		if depth == 0 && !strings.HasPrefix(token, "k:") {
			return nil
		}
	}
}

// traced encodes with the trace sink and decodes the result back.
func traced[A any](shape schema.Schema[A], value A) ([]string, A, error) {
	sink := &traceSink{}
	if err := schema.Encode(shape, value, sink); err != nil {
		var missing A
		return nil, missing, err
	}
	decoded, err := schema.Decode(shape, &traceSource{tokens: sink.tokens})
	return sink.tokens, decoded, err
}
