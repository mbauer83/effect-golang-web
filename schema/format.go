package schema

// The standard string formats.
//
// A format is two different claims and this package makes both. It is an
// annotation a reader of the contract acts on -- JSON Schema's own format
// keyword asserts nothing, by design -- and it is a rule a server has to
// enforce, because a request is refused here or it is not refused at all.
//
// Where the rule is a regular expression, the expression is recorded as well as
// the format name, so a consumer whose validator ignores format still gets the
// check from pattern. Where the rule is arithmetic or grammar -- an address, a
// URI -- there is no expression to record, and the document can only annotate.
// That asymmetry is real and is not hidden.

import (
	"net/mail"
	"net/netip"
	"net/url"
)

// UUID admits the textual form of a UUID, in any case.
func UUID() Schema[string] {
	return Matching(Formatted("uuid"),
		`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
}

// Email admits one addr-spec, which is what a form field carries.
//
// It is parsed rather than matched, because the grammar is not a regular
// language and every regular expression that claims to be it is wrong about
// something. A display name is refused: "Ada <ada@example.test>" is a mailbox,
// not an address, and a field asking for an address means the address.
func Email() Schema[string] {
	return checked(Formatted("email"), func(value string) error {
		parsed, err := mail.ParseAddress(value)
		if err != nil {
			return fail("is not an email address", err)
		}
		if parsed.Address != value {
			return fail("is a mailbox rather than an address", nil)
		}
		return nil
	})
}

// URI admits an absolute URI: one that says what scheme it is.
//
// A relative reference is a legitimate thing and a different one, so it has its
// own constructor rather than being quietly admitted here.
func URI() Schema[string] {
	return checked(Formatted("uri"), func(value string) error {
		parsed, err := url.Parse(value)
		if err != nil {
			return fail("is not a URI", err)
		}
		if !parsed.IsAbs() {
			return fail("is a relative reference rather than a URI", nil)
		}
		return nil
	})
}

// URL admits an absolute URI that locates something: a scheme, "://", and a
// host.
//
// mailto:ada@example.test is a URI and is not a URL, which is the whole
// difference between naming a thing and saying where it is. The rule is an
// expression, so the document carries it and a consumer's validator enforces
// the same thing this one does.
//
// It annotates as "uri", because that is the registered format name and there
// is no registered one for a locator.
func URL() Schema[string] {
	return Matching(Formatted("uri"), `^[A-Za-z][A-Za-z0-9+.\-]*://[^/?#]+`)
}

// URIReference admits a URI or a relative reference.
func URIReference() Schema[string] {
	return checked(Formatted("uri-reference"), func(value string) error {
		if _, err := url.Parse(value); err != nil {
			return fail("is not a URI reference", err)
		}
		return nil
	})
}

// Hostname admits a host name by the RFC 1123 rules: labels of letters, digits
// and hyphens, each at most 63 characters, and 253 in total.
func Hostname() Schema[string] {
	// The expression already refuses an empty name and a label that starts or
	// ends with a hyphen; the length is the one rule it cannot state.
	return MaxLength(
		Matching(Formatted("hostname"),
			`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$`),
		253)
}

// IPv4 admits a dotted-quad address.
func IPv4() Schema[string] {
	return checked(Formatted("ipv4"), func(value string) error {
		return addressOf(value, 4, "is not an IPv4 address")
	})
}

// IPv6 admits an IPv6 address, in any of its written forms.
func IPv6() Schema[string] {
	return checked(Formatted("ipv6"), func(value string) error {
		return addressOf(value, 16, "is not an IPv6 address")
	})
}

// addressOf parses an address and checks its family. The family is told from the
// parsed form rather than from the text, because a dotted quad is also a valid
// IPv6 address written the short way, and net.ParseIP admits both.
func addressOf(value string, width int, reason string) error {
	parsed, err := netip.ParseAddr(value)
	if err != nil {
		return fail(reason, err)
	}
	if len(parsed.AsSlice()) != width {
		return fail(reason, nil)
	}
	return nil
}

// checked narrows a schema with a rule the description cannot state.
//
// The value is checked in both directions, as a constraint is, but nothing is
// added to the structure: there is no keyword for "parses as an address", and
// inventing one would say something no other projection could read.
func checked[A any](inner Schema[A], check func(A) error) Schema[A] {
	if fault := Validate(inner); fault != nil {
		return faulted[A](inner.node, fault)
	}
	return of(
		inner.node,
		func(value A, into Sink) error {
			if err := check(value); err != nil {
				return err
			}
			return Encode(inner, value, into)
		},
		func(from Source) (A, error) {
			decoded, err := Decode(inner, from)
			if err != nil {
				return decoded, err
			}
			if err := check(decoded); err != nil {
				var missing A
				return missing, err
			}
			return decoded, nil
		},
	)
}
