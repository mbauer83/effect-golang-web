package schemagen

// The Go type a described shape becomes.
//
// The mapping is a convention and not a choice the description makes, because
// a description says what is on the wire and Go has more than one type for
// most of it. A wire integer becomes int64: it is the width the wire can
// carry, and narrowing it silently would be the generator deciding something
// the description did not say. A program that wants an int says so with a
// Transform, where the narrowing is visible.

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mbauer83/effect-golang-web/schema/structure"
)

func goTypeOf(node structure.Node) (string, error) {
	switch shape := node.(type) {
	case structure.Scalar:
		return goScalar(shape), nil
	case structure.Sequence:
		return wrappedGoType("[]", shape.Element)
	case structure.Mapping:
		return wrappedGoType("map[string]", shape.Value)
	case structure.Nullable:
		return wrappedGoType("*", shape.Inner)
	case structure.Object:
		return namedGoType(shape.Name, "an object")
	case structure.Union:
		return namedGoType(shape.Name, "a union")
	case structure.Reference:
		return namedGoType(shape.Name, "a reference")
	default:
		return "", errors.New("this shape has no Go type")
	}
}

// goScalar is the Go type the description names, or the default for its wire
// kind where it names none. It is not a guess: a description that pinned a
// width down says so, and one that did not gets the widest thing the wire can
// carry.
func goScalar(shape structure.Scalar) string {
	if named := shape.Precision.String(); named != "" {
		return named
	}
	switch shape.Kind {
	case structure.Integer:
		return "int64"
	case structure.Number:
		return "float64"
	case structure.Boolean:
		return "bool"
	case structure.Bytes:
		return "[]byte"
	case structure.Timestamp:
		return "time.Time"
	default:
		return "string"
	}
}

func wrappedGoType(prefix string, inner structure.Node) (string, error) {
	within, err := goTypeOf(inner)
	if err != nil {
		return "", err
	}
	return prefix + within, nil
}

// namedGoType refuses an anonymous shape. A name is what a Go type is, and a
// generator that invented one would be naming something the author did not.
func namedGoType(name string, what string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("%s has no name, and a Go type is a name", what)
	}
	return name, nil
}

// initialisms are the words Go capitalises whole, so a generated field reads
// as a Go programmer would have written it: ID rather than Id.
//
// It is the conventional set and not a domain glossary. A domain's own acronym
// is not a Go initialism, and guessing which ones are would be this generator
// having opinions about someone else's vocabulary.
var initialisms = map[string]string{
	"api": "API", "dns": "DNS", "html": "HTML", "http": "HTTP", "https": "HTTPS",
	"id": "ID", "ip": "IP", "json": "JSON", "sql": "SQL", "tcp": "TCP",
	"udp": "UDP", "uri": "URI", "url": "URL", "utf8": "UTF8", "uuid": "UUID",
	"xml": "XML",
}

// goFieldName is the exported Go name for a wire name: each part capitalised,
// with the separators a wire name uses dropped.
func goFieldName(wire string) (string, error) {
	parts := strings.FieldsFunc(wire, func(character rune) bool {
		return character == '_' || character == '-' || character == '.'
	})
	name := ""
	for _, part := range parts {
		if initialism, known := initialisms[strings.ToLower(part)]; known {
			name += initialism
			continue
		}
		name += strings.ToUpper(part[:1]) + part[1:]
	}
	if name == "" {
		return "", errors.New("a member has no name")
	}
	if first := name[:1]; strings.ToUpper(first) != first {
		return "", fmt.Errorf("the member %q cannot become an exported Go name", wire)
	}
	return name, nil
}
