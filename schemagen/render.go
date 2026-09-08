package schemagen

// Rendering a schema: the field declarations, the accessors and the combinator
// calls. One renderer, so a generated schema always looks the same.

import (
	"bytes"
	"fmt"
	"strconv"
)

// schemaPackage is what a generated file imports.
const schemaPackage = "github.com/mbauer83/effect-golang-web/schema"

// structType is one Go struct to write: its name, its prose, and its fields in
// the order the description declares them.
type structType struct {
	name   string
	doc    string
	fields []structField
}

// structField is one field: the Go name and type, the wire name, the schema
// expression that describes it, and whether it may be absent.
type structField struct {
	name string
	wire string
	doc  string
	// shape is the schema expression, already rendered.
	shape string
	// optional fields are absent rather than null, and so are pointers.
	optional bool
	// goType is the field's Go type; element is what it is a pointer to, where
	// it is one.
	goType  string
	element string
}

// renderType writes one schema. A struct with no fields is allowed: an empty
// variant of a union is a real shape, and refusing it would make one
// unexpressible.
func renderType(written *bytes.Buffer, described structType, origin string) error {
	fmt.Fprintf(written, "\n// %sSchema describes %s. It is generated from %s.\n",
		described.name, described.name, origin)
	fmt.Fprintf(written, "var %sSchema = ", described.name)
	fmt.Fprintf(written, "schema.Struct[%s](%s,\n", described.name, strconv.Quote(described.name))
	for _, field := range described.fields {
		renderField(written, described.name, field)
	}
	fmt.Fprintf(written, ")")
	if described.doc != "" {
		fmt.Fprintf(written, ".Documented(%s)", strconv.Quote(described.doc))
	}
	fmt.Fprintf(written, "\n")
	return nil
}

func renderField(written *bytes.Buffer, owner string, field structField) {
	if field.optional {
		renderOptional(written, owner, field)
	} else {
		renderRequired(written, owner, field)
	}
	if field.doc != "" {
		fmt.Fprintf(written, ".Documented(%s)", strconv.Quote(field.doc))
	}
	fmt.Fprintf(written, ",\n")
}

func renderRequired(written *bytes.Buffer, owner string, field structField) {
	fmt.Fprintf(written, "schema.FieldOf(%s, %s,\n", strconv.Quote(field.wire), field.shape)
	fmt.Fprintf(written, "func(value %s) %s { return value.%s },\n",
		owner, field.goType, field.name)
	fmt.Fprintf(written, "func(value *%s, field %s) { value.%s = field })",
		owner, field.goType, field.name)
}

// renderOptional reads presence from the pointer, which is the only honest
// answer: a zero value is not absence, and the schema deliberately refuses to
// guess that it is.
func renderOptional(written *bytes.Buffer, owner string, field structField) {
	fmt.Fprintf(written, "schema.OptionalFieldOf(%s, %s,\n", strconv.Quote(field.wire), field.shape)
	fmt.Fprintf(written, "func(value %s) (%s, bool) {\n", owner, field.element)
	fmt.Fprintf(written, "var absent %s\n", field.element)
	fmt.Fprintf(written, "if value.%s == nil { return absent, false }\n", field.name)
	fmt.Fprintf(written, "return *value.%s, true\n},\n", field.name)
	fmt.Fprintf(written, "func(value *%s, field %s) { value.%s = &field })",
		owner, field.element, field.name)
}
