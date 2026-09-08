package protobuf

// The document a description implies: a proto3 file, as data before it is text.

import (
	"strconv"
	"strings"
)

// Document is a proto3 file.
type Document struct {
	// Package is the proto package. It is the caller's, because a proto
	// package is a namespace shared with every other language reading these
	// messages and nothing in a Go description knows what it should be.
	Package string
	// Imports are the well-known types the messages use.
	Imports []string
	// Messages are every named shape reachable from the root, each declared
	// once, in the order they were first reached.
	Messages []Message
	// Root is the name of the message the projected description became.
	Root string
}

// Message is one proto3 message.
type Message struct {
	Name string
	Doc  string
	// Fields are its members. A message projected from a union has one field
	// per variant, all inside a oneof.
	Fields []Field
	// OneOf is the name of the oneof its fields belong to, or empty when they
	// are ordinary fields. A message holds at most one, because a description's
	// union is the whole of the shape it describes.
	OneOf string
}

// Field is one member of a message.
type Field struct {
	Name   string
	Doc    string
	Number int
	// Type is the proto type: a scalar keyword, a message name, or a map type.
	Type string
	// Repeated says the field carries many of Type.
	Repeated bool
	// Optional says the field has explicit presence, which proto3 spells with
	// the keyword and which a oneof member may not have.
	Optional bool
	// Notes are what the description says and proto3 has no way to state --
	// the constraints, principally. They are emitted as comments, because a
	// comment is honest about not being enforced where an invented option
	// would not be.
	Notes []string
}

// Render writes the document as a .proto file.
func (document Document) Render() string {
	written := &strings.Builder{}
	written.WriteString("syntax = \"proto3\";\n")
	if document.Package != "" {
		written.WriteString("\npackage " + document.Package + ";\n")
	}
	if len(document.Imports) > 0 {
		written.WriteString("\n")
		for _, imported := range document.Imports {
			written.WriteString("import \"" + imported + "\";\n")
		}
	}
	for _, message := range document.Messages {
		written.WriteString("\n")
		message.render(written)
	}
	return written.String()
}

func (message Message) render(written *strings.Builder) {
	writeComment(written, "", message.Doc)
	written.WriteString("message " + message.Name + " {\n")

	indent := "  "
	if message.OneOf != "" {
		written.WriteString("  oneof " + message.OneOf + " {\n")
		indent = "    "
	}
	for _, field := range message.Fields {
		field.render(written, indent)
	}
	if message.OneOf != "" {
		written.WriteString("  }\n")
	}
	written.WriteString("}\n")
}

func (field Field) render(written *strings.Builder, indent string) {
	writeComment(written, indent, field.Doc)
	for _, note := range field.Notes {
		written.WriteString(indent + "// " + note + "\n")
	}

	written.WriteString(indent)
	if field.Repeated {
		written.WriteString("repeated ")
	}
	if field.Optional {
		written.WriteString("optional ")
	}
	written.WriteString(field.Type + " " + field.Name + " = " +
		strconv.Itoa(field.Number) + ";\n")
}

// writeComment writes prose as a leading comment, one line per line of it, so a
// multi-paragraph doc comment does not become one unreadable line.
func writeComment(written *strings.Builder, indent string, doc string) {
	if doc == "" {
		return
	}
	for _, line := range strings.Split(strings.TrimSpace(doc), "\n") {
		written.WriteString(indent + "// " + strings.TrimSpace(line) + "\n")
	}
}
