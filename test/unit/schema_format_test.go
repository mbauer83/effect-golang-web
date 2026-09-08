package unit

// The standard string formats. A format is two claims -- an annotation a reader
// acts on and a rule a server enforces -- and these check both are made.

import (
	"strings"
	"testing"

	"github.com/mbauer83/effect-golang-web/schema"
	"github.com/mbauer83/effect-golang-web/schema/structure"
)

// formats pairs each constructor with what it must admit and refuse.
var formats = []struct {
	// label identifies the case; format is the annotation it must carry, and
	// the two differ where more than one constructor annotates the same way.
	label  string
	format string
	// expressed says whether the rule is a regular expression the document can
	// carry as well as annotate.
	expressed bool
	shape     schema.Schema[string]
	admitted  []string
	refused   []string
}{
	{
		label: "uuid", format: "uuid", expressed: true,
		shape: schema.UUID(),
		admitted: []string{
			"123e4567-e89b-12d3-a456-426614174000",
			"123E4567-E89B-12D3-A456-426614174000",
		},
		refused: []string{
			"", "123e4567e89b12d3a456426614174000", "not-a-uuid",
			// Near misses: a group of the wrong length, and a trailing extra.
			"123e456-e89b-12d3-a456-426614174000",
			"123e4567-e89b-12d3-a456-4266141740000",
		},
	},
	{
		label: "email", format: "email",
		shape:    schema.Email(),
		admitted: []string{"ada@example.test", "ada+notes@sub.example.test"},
		// A mailbox is not an address: a field asking for an address means the
		// address, and every regular expression claiming to be this grammar is
		// wrong about something, so it is parsed instead.
		refused: []string{"", "ada", "ada@", "Ada <ada@example.test>"},
	},
	{
		label: "uri", format: "uri",
		shape:    schema.URI(),
		admitted: []string{"https://example.test/books", "mailto:ada@example.test"},
		refused:  []string{"", "/books", "example.test/books"},
	},
	{
		// A URL annotates as "uri" -- the registered name -- and enforces
		// more: a locator has a host.
		label: "url", format: "uri", expressed: true,
		shape:    schema.URL(),
		admitted: []string{"https://example.test/books", "http://example.test", "ftp://h/p"},
		refused:  []string{"", "/books", "mailto:ada@example.test", "https://"},
	},
	{
		label: "uri-reference", format: "uri-reference",
		shape:    schema.URIReference(),
		admitted: []string{"/books", "https://example.test/books"},
		refused:  []string{"https://example.test/\x7f"},
	},
	{
		label: "hostname", format: "hostname", expressed: true,
		shape:    schema.Hostname(),
		admitted: []string{"example.test", "sub.example.test", "a"},
		refused:  []string{"", "-example.test", "example-.test", "exa mple.test"},
	},
	{
		label: "ipv4", format: "ipv4",
		shape:    schema.IPv4(),
		admitted: []string{"127.0.0.1", "192.168.0.255"},
		// A dotted quad is also a valid IPv6 address written the short way, so
		// the family is told from the parsed form rather than from the text.
		refused: []string{"", "::1", "256.0.0.1", "127.0.0"},
	},
	{
		label: "ipv6", format: "ipv6",
		shape:    schema.IPv6(),
		admitted: []string{"::1", "2001:db8::1"},
		refused:  []string{"", "127.0.0.1", "not-an-address"},
	},
}

func TestEachFormatAdmitsWhatItSaysAndRefusesTheRest(t *testing.T) {
	for _, format := range formats {
		for _, admitted := range format.admitted {
			if _, err := schema.DecodeJSON(format.shape, quoted(admitted)); err != nil {
				t.Errorf("%s: expected %q to be admitted, got %v", format.label, admitted, err)
			}
		}
		for _, refused := range format.refused {
			if _, err := schema.DecodeJSON(format.shape, quoted(refused)); err == nil {
				t.Errorf("%s: expected %q to be refused", format.label, refused)
			}
		}
	}
}

func TestEachFormatSaysWhichFormatItIs(t *testing.T) {
	// The annotation is half the point: a reader of the contract acts on it,
	// and JSON Schema's own format keyword asserts nothing by design.
	for _, format := range formats {
		shape, isScalar := format.shape.Structure().(structure.Scalar)
		if !isScalar {
			t.Errorf("%s: expected a scalar, got %#v", format.label, format.shape.Structure())
			continue
		}
		if shape.Format != format.format {
			t.Errorf("%s: expected %q, got %q", format.label, format.format, shape.Format)
		}
	}
}

func TestAFormatWhoseRuleIsAnExpressionRecordsIt(t *testing.T) {
	// Where the rule is a regular expression it is recorded as well, so a
	// consumer whose validator ignores format still gets the check from
	// pattern. Where the rule is grammar there is nothing to record, and
	// pretending otherwise would say something no projection could read.
	for _, format := range formats {
		shape := format.shape.Structure().(structure.Scalar)
		recorded := false
		for _, constraint := range shape.Constraints {
			if _, isPattern := constraint.(structure.Pattern); isPattern {
				recorded = true
			}
		}
		if recorded != format.expressed {
			t.Errorf("%s: pattern recorded = %v, expected %v",
				format.label, recorded, format.expressed)
		}
	}
}

func TestAFormatIsCheckedOnEncodingToo(t *testing.T) {
	if _, err := schema.EncodeJSON(schema.Email(), "not-an-address"); err == nil {
		t.Fatal("expected an address this program built to be checked before it was sent")
	}
}

func TestAFormatRefusalSaysWhatItWanted(t *testing.T) {
	_, err := schema.DecodeJSON(schema.Email(), quoted("ada"))
	if err == nil {
		t.Fatal("expected the address to be refused")
	}
	if !strings.Contains(err.Error(), "is not an email address") {
		t.Fatalf("expected the reason to say so, got %v", err)
	}
}

func TestAnUnknownFormatAnnotatesWithoutChecking(t *testing.T) {
	// The format vocabulary is open. A projection carries whatever the author
	// wrote, and a name this package has not been taught claims nothing.
	shape := schema.Formatted("isbn")
	if _, err := schema.DecodeJSON(shape, quoted("anything at all")); err != nil {
		t.Fatalf("expected an unchecked format to admit anything, got %v", err)
	}
	if scalar := shape.Structure().(structure.Scalar); scalar.Format != "isbn" {
		t.Fatalf("expected the name carried, got %q", scalar.Format)
	}
}

func quoted(value string) []byte {
	document, err := schema.EncodeJSON(schema.Text(), value)
	if err != nil {
		panic(err)
	}
	return document
}
