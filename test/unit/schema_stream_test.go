package unit

// The streaming forms. They are the ones a request body and a response writer
// will use, so a document that never becomes a []byte has to work as well as
// one that does.

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/mbauer83/effect-golang-web/schema"
)

func TestAValueRoundTripsThroughAStreamWithoutBecomingBytes(t *testing.T) {
	original := Book{Title: "Zionomicon", Authors: []string{"A"}, Pages: 632, HasIndex: true}

	var written bytes.Buffer
	if err := schema.EncodeJSONTo(bookSchema, original, &written); err != nil {
		t.Fatal(err)
	}
	decoded, err := schema.DecodeJSONFrom(bookSchema, bytes.NewReader(written.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, original) {
		t.Fatalf("round trip changed the value:\n  before %#v\n  after  %#v", original, decoded)
	}
}

func TestAStreamThatEndsEarlyIsReportedAsTruncation(t *testing.T) {
	// A body that stops mid-document has a different cause and a different fix
	// from one that is the wrong shape, so it must not be reported as one.
	_, err := schema.DecodeJSONFrom(bookSchema, strings.NewReader(`{"title":"T","auth`))
	if err == nil {
		t.Fatal("expected a truncated stream to be reported")
	}
	if !strings.Contains(err.Error(), "the document ended") {
		t.Fatalf("expected the reason to say the document ended early, got %v", err)
	}
}

func TestAWriterThatFailsIsReportedRatherThanIgnored(t *testing.T) {
	err := schema.EncodeJSONTo(bookSchema, Book{Authors: []string{}}, refusingWriter{})
	if !errors.Is(err, errRefused) {
		t.Fatalf("expected the writer's failure to surface, got %v", err)
	}
}

var errRefused = errors.New("the writer refused")

type refusingWriter struct{}

func (refusingWriter) Write([]byte) (int, error) { return 0, errRefused }
