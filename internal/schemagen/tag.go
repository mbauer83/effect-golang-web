package schemagen

// The schema tag. A struct field can carry a type and a name; everything else a
// schema says -- a bound, a length, a pattern, a format -- has to be written
// somewhere, and the tag is the only place next to the field.

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// options are what a field's schema tag asks for.
type options struct {
	skip bool
	// use names a schema written by hand, for anything the table cannot say.
	use string
	// format is a refinement a projection may carry, for strings.
	format string
	// constraints are applied in the order they were written, so the reader of
	// the tag and the reader of the generated code see the same order.
	constraints []constraint
}

type constraint struct {
	name  string
	value string
}

// readOptions parses the schema tag.
//
// Items are separated by commas, except inside braces, brackets or parentheses:
// a pattern like [0-9]{2,4} carries a comma of its own, and splitting on it
// would cut the pattern in half.
func readOptions(tag reflect.StructTag) (options, error) {
	declared, present := tag.Lookup("schema")
	if !present || strings.TrimSpace(declared) == "" {
		return options{}, nil
	}
	if strings.TrimSpace(declared) == "-" {
		return options{skip: true}, nil
	}

	read := options{}
	for _, item := range splitItems(declared) {
		name, value, hasValue := strings.Cut(item, "=")
		name = strings.TrimSpace(name)
		if !hasValue {
			return options{}, fmt.Errorf("the schema tag item %q has no value", item)
		}
		switch name {
		case "use":
			read.use = value
		case "format":
			read.format = value
		default:
			if _, known := combinators[name]; !known {
				return options{}, fmt.Errorf("the schema tag item %q is not one of %s",
					name, strings.Join(vocabulary(), ", "))
			}
			read.constraints = append(read.constraints, constraint{name: name, value: value})
		}
	}
	return read, nil
}

// splitItems splits on commas at nesting depth zero.
func splitItems(declared string) []string {
	items := []string{}
	depth := 0
	start := 0
	for index, character := range declared {
		switch character {
		case '{', '[', '(':
			depth++
		case '}', ']', ')':
			depth--
		case ',':
			if depth == 0 {
				items = append(items, declared[start:index])
				start = index + 1
			}
		}
	}
	return append(items, declared[start:])
}

// combinators names the schema function each tag item stands for, and what kind
// of Go type it applies to.
var combinators = map[string]struct {
	call    string
	applies appliesTo
}{
	"min":       {"schema.AtLeast", toNumber},
	"max":       {"schema.AtMost", toNumber},
	"above":     {"schema.Above", toNumber},
	"below":     {"schema.Below", toNumber},
	"minLength": {"schema.MinLength", toText},
	"maxLength": {"schema.MaxLength", toText},
	"pattern":   {"schema.Matching", toText},
	"minItems":  {"schema.MinItems", toList},
	"maxItems":  {"schema.MaxItems", toList},
}

type appliesTo uint8

const (
	toNumber appliesTo = iota
	toText
	toList
)

func (kind appliesTo) String() string {
	switch kind {
	case toText:
		return "a string"
	case toList:
		return "a list"
	default:
		return "a number"
	}
}

func vocabulary() []string {
	names := make([]string, 0, len(combinators)+2)
	names = append(names, "use", "format")
	for name := range combinators {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// formats names the constructor for each standard format.
//
// A known format both annotates and checks; one that is not here annotates
// only, because JSON Schema's format vocabulary is open and a projection should
// carry whatever the author wrote rather than refuse a name it has not been
// taught. The generated call says which of the two it is.
var formats = map[string]string{
	"uuid":          "schema.UUID()",
	"email":         "schema.Email()",
	"uri":           "schema.URI()",
	"url":           "schema.URL()",
	"uri-reference": "schema.URIReference()",
	"hostname":      "schema.Hostname()",
	"ipv4":          "schema.IPv4()",
	"ipv6":          "schema.IPv6()",
}

// formatShape is the shape a format asks for.
func formatShape(format string) string {
	if known, checked := formats[format]; checked {
		return known
	}
	return "schema.Formatted(" + strconv.Quote(format) + ")"
}

// constrain wraps a shape in the constraints the tag asked for, checking that
// each one applies to the type it is written on: a length on a number would
// compile into nothing sensible, and a generator that emitted it would be
// handing the reader a compile error instead of an explanation.
func constrain(shape string, goType string, constraints []constraint) (string, error) {
	for _, asked := range constraints {
		combinator := combinators[asked.name]
		if !appliesToType(combinator.applies, goType) {
			return "", fmt.Errorf("%s applies to %s, and this field is %s",
				asked.name, combinator.applies, goType)
		}
		rendered, err := literal(asked)
		if err != nil {
			return "", err
		}
		shape = combinator.call + "(" + shape + ", " + rendered + ")"
	}
	return shape, nil
}

func appliesToType(kind appliesTo, goType string) bool {
	switch kind {
	case toText:
		return goType == "string"
	case toList:
		return strings.HasPrefix(goType, "[]") && goType != "[]byte"
	default:
		_, isNumber := numbers[goType]
		return isNumber
	}
}

var numbers = map[string]bool{
	"int": true, "int8": true, "int16": true, "int32": true, "int64": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true,
	"float32": true, "float64": true,
}

// literal renders a tag value as the Go literal the combinator takes.
func literal(asked constraint) (string, error) {
	if asked.name == "pattern" {
		if asked.value == "" {
			return "", errors.New("a pattern is not empty")
		}
		return strconv.Quote(asked.value), nil
	}
	if _, err := strconv.ParseFloat(asked.value, 64); err != nil {
		return "", fmt.Errorf("%s takes a number, and %q is not one", asked.name, asked.value)
	}
	return asked.value, nil
}
