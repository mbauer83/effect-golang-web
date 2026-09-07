package unit

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mbauer83/effect-golang-web/schema"
)

// Book is the worked example from the package documentation, so the tests and
// the documentation describe the same type.
type Book struct {
	Title    string
	Authors  []string
	Pages    int
	Subtitle string
	HasIndex bool
}

var bookSchema = schema.Struct[Book]("Book",
	schema.FieldOf("title", schema.Text(),
		func(book Book) string { return book.Title },
		func(book *Book, title string) { book.Title = title }),
	schema.FieldOf("authors", schema.List(schema.Text()),
		func(book Book) []string { return book.Authors },
		func(book *Book, authors []string) { book.Authors = authors }),
	schema.FieldOf("pages", schema.Int(),
		func(book Book) int { return book.Pages },
		func(book *Book, pages int) { book.Pages = pages }),
	schema.OptionalFieldOf("subtitle", schema.Text(),
		func(book Book) (string, bool) { return book.Subtitle, book.Subtitle != "" },
		func(book *Book, subtitle string) { book.Subtitle = subtitle }),
	schema.FieldOf("hasIndex", schema.Bool(),
		func(book Book) bool { return book.HasIndex },
		func(book *Book, has bool) { book.HasIndex = has }),
)

func TestASchemaRoundTripsAValue(t *testing.T) {
	original := Book{
		Title:    "Zionomicon",
		Authors:  []string{"John A. De Goes", "Adam Fraser"},
		Pages:    632,
		Subtitle: "A field guide",
		HasIndex: true,
	}

	document, err := schema.EncodeJSON(bookSchema, original)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := schema.DecodeJSON(bookSchema, document)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, original) {
		t.Fatalf("round trip changed the value:\n  before %#v\n  after  %#v", original, decoded)
	}
}

func TestEncodingIsDeterministicAndInDeclaredOrder(t *testing.T) {
	// Field order follows the declaration, not Go's map iteration, so a
	// document can be compared in a test and cached by an intermediary.
	document, err := schema.EncodeJSON(bookSchema, Book{Title: "T", Authors: []string{}, Pages: 1})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"title":"T","authors":[],"pages":1,"hasIndex":false}`
	if got := strings.TrimSpace(string(document)); got != want {
		t.Fatalf("expected\n  %s\ngot\n  %s", want, got)
	}
}

func TestAnAbsentOptionalFieldIsOmittedRatherThanNull(t *testing.T) {
	document, err := schema.EncodeJSON(bookSchema, Book{Title: "T", Authors: []string{}, Pages: 1})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(document), "subtitle") {
		t.Fatalf("expected the absent field omitted, got %s", document)
	}

	// Both forms of absence decode: a missing field and an explicit null.
	for _, document := range []string{
		`{"title":"T","authors":[],"pages":1,"hasIndex":false}`,
		`{"title":"T","authors":[],"pages":1,"subtitle":null,"hasIndex":false}`,
	} {
		decoded, err := schema.DecodeJSON(bookSchema, []byte(document))
		if err != nil {
			t.Fatalf("%s: %v", document, err)
		}
		if decoded.Subtitle != "" {
			t.Fatalf("%s: expected no subtitle, got %q", document, decoded.Subtitle)
		}
	}
}

func TestAnUnknownFieldIsTolerated(t *testing.T) {
	// A decoder that rejected an unknown field could not read a document
	// written by a newer version of its producer.
	decoded, err := schema.DecodeJSON(bookSchema,
		[]byte(`{"title":"T","authors":[],"pages":1,"hasIndex":false,"isbn":"x","extra":{"a":[1]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Title != "T" {
		t.Fatalf("unexpected value: %#v", decoded)
	}
}

func TestScalarsRoundTripIncludingBytesAndTime(t *testing.T) {
	type payload struct {
		Blob []byte
		At   time.Time
		Rate float64
	}
	moment := time.Date(2026, 9, 7, 12, 30, 0, 0, time.UTC)
	payloadSchema := schema.Struct[payload]("Payload",
		schema.FieldOf("blob", schema.Bytes(),
			func(p payload) []byte { return p.Blob },
			func(p *payload, blob []byte) { p.Blob = blob }),
		schema.FieldOf("at", schema.Time(),
			func(p payload) time.Time { return p.At },
			func(p *payload, at time.Time) { p.At = at }),
		schema.FieldOf("rate", schema.Float64(),
			func(p payload) float64 { return p.Rate },
			func(p *payload, rate float64) { p.Rate = rate }),
	)

	original := payload{Blob: []byte{0x00, 0xff, 0x10}, At: moment, Rate: 1.5}
	document, err := schema.EncodeJSON(payloadSchema, original)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(document), `"2026-09-07T12:30:00Z"`) {
		t.Fatalf("expected an RFC 3339 timestamp, got %s", document)
	}

	decoded, err := schema.DecodeJSON(payloadSchema, document)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded.Blob, original.Blob) || !decoded.At.Equal(moment) || decoded.Rate != 1.5 {
		t.Fatalf("round trip changed the value: %#v", decoded)
	}
}

func TestMapEncodingSortsItsKeys(t *testing.T) {
	counts := schema.Map(schema.Int())
	document, err := schema.EncodeJSON(counts, map[string]int{"c": 3, "a": 1, "b": 2})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"a":1,"b":2,"c":3}`
	if got := strings.TrimSpace(string(document)); got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestNullableIsPresentAndNullRatherThanAbsent(t *testing.T) {
	nullable := schema.Nullable(schema.Text())

	document, err := schema.EncodeJSON(nullable, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(document)); got != "null" {
		t.Fatalf("expected null, got %s", got)
	}
	decoded, err := schema.DecodeJSON(nullable, []byte("null"))
	if err != nil || decoded != nil {
		t.Fatalf("unexpected decode: %v %v", decoded, err)
	}

	held := "present"
	document, err = schema.EncodeJSON(nullable, &held)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(document)); got != `"present"` {
		t.Fatalf("unexpected document: %s", got)
	}
}

func TestTransformSeparatesTheWireShapeFromTheDomainType(t *testing.T) {
	type Celsius float64
	// The wire carries a number; the program wants a distinct type, and a
	// negative reading below absolute zero is rejected rather than accepted.
	celsius := schema.TransformOrFail(schema.Float64(),
		func(value float64) (Celsius, error) {
			if value < -273.15 {
				return 0, errors.New("below absolute zero")
			}
			return Celsius(value), nil
		},
		func(value Celsius) (float64, error) { return float64(value), nil },
	)

	document, err := schema.EncodeJSON(celsius, Celsius(21.5))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(document)); got != "21.5" {
		t.Fatalf("expected a bare number, got %s", got)
	}
	if decoded, err := schema.DecodeJSON(celsius, []byte("21.5")); err != nil || decoded != 21.5 {
		t.Fatalf("unexpected decode: %v %v", decoded, err)
	}
	if _, err := schema.DecodeJSON(celsius, []byte("-300")); err == nil {
		t.Fatal("expected a refinement to reject an out-of-range value")
	}
}
