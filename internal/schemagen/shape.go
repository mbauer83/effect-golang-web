package schemagen

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"strings"
)

// shapeOf is the schema expression for a Go type.
//
// The set it covers is deliberately small. A shape the table cannot name is
// refused with the position of the field, because a generator that guessed
// would produce a schema that compiles and describes the wrong thing.
func shapeOf(goType string, field *ast.Field, files *token.FileSet) (string, error) {
	if named, ok := scalars[goType]; ok {
		return named, nil
	}
	switch {
	case goType == "[]byte":
		return "schema.Bytes()", nil
	case strings.HasPrefix(goType, "[]"):
		return wrapped("schema.List", strings.TrimPrefix(goType, "[]"), field, files)
	case strings.HasPrefix(goType, "map[string]"):
		return wrapped("schema.Map", strings.TrimPrefix(goType, "map[string]"), field, files)
	case strings.HasPrefix(goType, "*"):
		// A pointer that is not an optional field is present-and-null, which is
		// a different thing and has its own combinator.
		return wrapped("schema.Nullable", strings.TrimPrefix(goType, "*"), field, files)
	case local(goType):
		// A type in this package has a schema of its own: generated if it is
		// marked, hand-written if it is a union or carries a refinement.
		return goType + "Schema", nil
	default:
		return "", fmt.Errorf(
			"%s: no schema for %s; write one by hand and point at it with a use= tag",
			where(files, field.Pos()), goType)
	}
}

// scalars are the shapes named directly. time.Time is here because a timestamp
// is a scalar on the wire whatever Go calls it.
var scalars = map[string]string{
	"string":    "schema.Text()",
	"bool":      "schema.Bool()",
	"time.Time": "schema.Time()",
	"int":       "schema.Int()",
	"int8":      "schema.Int8()",
	"int16":     "schema.Int16()",
	"int32":     "schema.Int32()",
	"int64":     "schema.Int64()",
	"uint":      "schema.Uint()",
	"uint16":    "schema.Uint16()",
	"uint32":    "schema.Uint32()",
	"uint64":    "schema.Uint64()",
	"float32":   "schema.Float32()",
	"float64":   "schema.Float64()",
}

func wrapped(combinator string, inner string, field *ast.Field, files *token.FileSet) (string, error) {
	shape, err := shapeOf(inner, field, files)
	if err != nil {
		return "", err
	}
	return combinator + "(" + shape + ")", nil
}

// local reports a name declared in this package: one identifier, capitalised,
// with no qualifier. Anything else needs a schema someone else wrote.
func local(goType string) bool {
	if goType == "" || strings.ContainsAny(goType, ".[]*{}") {
		return false
	}
	first := goType[:1]
	return strings.ToUpper(first) == first
}

// source renders a type expression as the Go text it was written as, which is
// what the accessors need to name.
func source(files *token.FileSet, expression ast.Expr) string {
	var written bytes.Buffer
	if err := printer.Fprint(&written, files, expression); err != nil {
		return ""
	}
	return written.String()
}
