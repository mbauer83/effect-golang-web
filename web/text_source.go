package web

import (
	"encoding/base64"
	"errors"
	"strconv"
	"time"
)

// textSource reads one scalar from the text a request carried it as.
//
// It is why a path segment, a query parameter and a header need no description
// of their own: the same Schema that describes a field of a JSON body describes
// a parameter, because a schema drives a Source and this is one. A format
// author outside this module implements the same contract.
type textSource struct {
	value string
}

func (source textSource) Text() (string, error) {
	return source.value, nil
}

func (source textSource) Integer() (int64, error) {
	parsed, err := strconv.ParseInt(source.value, 10, 64)
	if err != nil {
		return 0, errors.New("expected a whole number, found " + strconv.Quote(source.value))
	}
	return parsed, nil
}

func (source textSource) Number() (float64, error) {
	parsed, err := strconv.ParseFloat(source.value, 64)
	if err != nil {
		return 0, errors.New("expected a number, found " + strconv.Quote(source.value))
	}
	return parsed, nil
}

// Boolean accepts what a query string conventionally carries, which is more
// than Go's own parser: a bare flag with no value reads as true elsewhere and
// would be surprising to refuse here.
func (source textSource) Boolean() (bool, error) {
	if source.value == "" {
		return true, nil
	}
	parsed, err := strconv.ParseBool(source.value)
	if err != nil {
		return false, errors.New("expected true or false, found " + strconv.Quote(source.value))
	}
	return parsed, nil
}

func (source textSource) Bytes() ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(source.value)
	if err != nil {
		return nil, errors.New("expected base64, found " + strconv.Quote(source.value))
	}
	return decoded, nil
}

func (source textSource) Timestamp() (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, source.value)
	if err != nil {
		return time.Time{}, errors.New("expected an RFC 3339 timestamp, found " + strconv.Quote(source.value))
	}
	return parsed, nil
}

// Null is always false. A parameter that is present carries a value; one that
// is absent is not read at all, which is a different question and belongs to
// the codec.
func (textSource) Null() (bool, error) {
	return false, nil
}

func (textSource) ReadObject(func(string) error) error {
	return errCompoundParameter
}

func (textSource) ReadList(func() error) error {
	return errCompoundParameter
}

// Skip has nothing to discard: the value has already been read from the text.
func (textSource) Skip() error {
	return nil
}

// errCompoundParameter reports a schema that describes an object or a list
// being used where a request carries one scalar. Repeated query parameters and
// structured values are a separate feature, not this one behaving oddly.
var errCompoundParameter = errors.New(
	"a parameter carries a single value, and this schema describes a compound one")
