package schemagen

// Reading one field: what a document calls it, whether it may be absent, and
// which schema describes it.

import (
	"fmt"
	"go/ast"
	"go/token"
	"reflect"
	"strconv"
	"strings"
)

// describeField reads one field declaration, which may name several fields.
func describeField(field *ast.Field, files *token.FileSet) ([]structField, error) {
	if len(field.Names) == 0 {
		return nil, fmt.Errorf("%s: an embedded field has no shape of its own",
			where(files, field.Pos()))
	}

	tag := fieldTag(field)
	asked, err := readOptions(tag)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", where(files, field.Pos()), err)
	}
	if asked.skip {
		return nil, nil
	}

	read := make([]structField, 0, len(field.Names))
	for _, name := range field.Names {
		if !name.IsExported() {
			continue
		}
		described, err := fieldOf(name.Name, field, tag, asked, files)
		if err != nil {
			return nil, err
		}
		read = append(read, described)
	}
	return read, nil
}

func fieldOf(
	name string,
	field *ast.Field,
	tag reflect.StructTag,
	asked options,
	files *token.FileSet,
) (structField, error) {
	wire, optional := wireName(name, tag)
	described := structField{
		name:     name,
		wire:     wire,
		doc:      prose(field.Doc),
		optional: optional,
		goType:   source(files, field.Type),
	}

	described.element = described.goType
	if optional {
		pointed, isPointer := field.Type.(*ast.StarExpr)
		if !isPointer {
			return structField{}, fmt.Errorf(
				"%s: %s is optional and not a pointer; absent and empty are different, "+
					"so an optional field says which it is by being a pointer",
				where(files, field.Pos()), name)
		}
		described.element = source(files, pointed.X)
	}

	shape, err := shapeAsked(described.element, asked, field, files)
	if err != nil {
		return structField{}, err
	}
	described.shape = shape
	return described, nil
}

// shapeAsked builds the field's shape: what the tag names, or what the type
// implies, narrowed by whatever constraints the tag carries.
func shapeAsked(
	goType string,
	asked options,
	field *ast.Field,
	files *token.FileSet,
) (string, error) {
	shape := asked.use
	if shape == "" {
		derived, err := baseShape(goType, asked.format, field, files)
		if err != nil {
			return "", err
		}
		shape = derived
	}
	constrained, err := constrain(shape, goType, asked.constraints)
	if err != nil {
		return "", fmt.Errorf("%s: %w", where(files, field.Pos()), err)
	}
	return constrained, nil
}

// baseShape is the shape the type implies. A format applies to a string only:
// it is a refinement of what the text means, and the other kinds already carry
// the one their wire form has.
func baseShape(
	goType string,
	format string,
	field *ast.Field,
	files *token.FileSet,
) (string, error) {
	if format == "" {
		return shapeOf(goType, field, files)
	}
	if goType != "string" {
		return "", fmt.Errorf("%s: a format applies to a string, and this field is %s",
			where(files, field.Pos()), goType)
	}
	return "schema.Formatted(" + strconv.Quote(format) + ")", nil
}

// wireName is what a document calls the field, and whether it may be absent.
// The json tag is read because it is already the convention for exactly this,
// and a second tag saying the same thing would be one more place to disagree.
func wireName(name string, tag reflect.StructTag) (string, bool) {
	declared, _ := tag.Lookup("json")
	parts := strings.Split(declared, ",")
	wire := parts[0]
	if wire == "" {
		wire = strings.ToLower(name[:1]) + name[1:]
	}
	for _, option := range parts[1:] {
		if option == "omitempty" {
			return wire, true
		}
	}
	return wire, false
}

func fieldTag(field *ast.Field) reflect.StructTag {
	if field.Tag == nil {
		return ""
	}
	unquoted, err := strconv.Unquote(field.Tag.Value)
	if err != nil {
		return ""
	}
	return reflect.StructTag(unquoted)
}

// prose is the first paragraph of a doc comment, with directives removed.
//
// Only the first paragraph: it is the summary by Go's own convention, and the
// elaboration that follows is usually addressed to whoever maintains the type
// rather than to whoever reads the published contract. Directives are
// instructions to this generator and are documentation of nothing.
func prose(comment *ast.CommentGroup) string {
	if comment == nil {
		return ""
	}
	lines := []string{}
	for _, line := range comment.List {
		text := strings.TrimSpace(line.Text)
		if strings.HasPrefix(text, "//schema:") || strings.HasPrefix(text, "//go:") {
			continue
		}
		text = strings.TrimSpace(strings.TrimPrefix(text, "//"))
		if text == "" {
			if len(lines) > 0 {
				break
			}
			continue
		}
		lines = append(lines, text)
	}
	return strings.TrimSpace(strings.Join(lines, " "))
}

func where(files *token.FileSet, position token.Pos) string {
	return files.Position(position).String()
}
