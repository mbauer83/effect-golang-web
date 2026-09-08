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
	if tag.Get("schema") == "-" {
		return nil, nil
	}

	read := make([]structField, 0, len(field.Names))
	for _, name := range field.Names {
		if !name.IsExported() {
			continue
		}
		described, err := fieldOf(name.Name, field, tag, files)
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

	if used := tag.Get("schema"); strings.HasPrefix(used, "use=") {
		described.shape = strings.TrimPrefix(used, "use=")
		return described, nil
	}
	shape, err := shapeOf(described.element, field, files)
	if err != nil {
		return structField{}, err
	}
	described.shape = shape
	return described, nil
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
