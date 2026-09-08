package schemagen

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

// directive marks a struct the generator writes a schema for.
const directive = "//schema:generate"

// collected is one package's worth of work: the structs marked for generation,
// in the order the source declares them, so the output is stable.
type collected struct {
	packageName string
	types       []structType
	files       *token.FileSet
	// imports maps a qualifier to the path it stands for, taken from the
	// source files, so a generated accessor that names time.Time can import
	// what it needs without the generator knowing which packages exist.
	imports map[string]string
}

type structType struct {
	name   string
	doc    string
	fields []structField
}

type structField struct {
	// name is the Go field's name; wire is what it is called in a document.
	name string
	wire string
	doc  string
	// shape is the schema expression for the field, already rendered.
	shape string
	// optional fields are absent rather than null, and so must be pointers.
	optional bool
	// goType is the field's type as Go source, for the accessors.
	goType string
	// element is the pointed-to type of an optional field.
	element string
}

func collect(directory string) (collected, error) {
	files := token.NewFileSet()
	packages, err := parser.ParseDir(files, directory, sourceFile, parser.ParseComments)
	if err != nil {
		return collected{}, err
	}

	found := collected{files: files, imports: map[string]string{}}
	for name, parsed := range packages {
		if strings.HasSuffix(name, "_test") {
			continue
		}
		found.packageName = name
		found.readImports(parsed)
		types, err := markedTypes(parsed, files)
		if err != nil {
			return collected{}, err
		}
		found.types = append(found.types, types...)
	}
	return found, nil
}

// readImports records what each qualifier in the package's sources stands for.
func (found collected) readImports(parsed *ast.Package) {
	for _, file := range parsed.Files {
		for _, imported := range file.Imports {
			path, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				continue
			}
			qualifier := path[strings.LastIndex(path, "/")+1:]
			if imported.Name != nil {
				qualifier = imported.Name.Name
			}
			found.imports[qualifier] = path
		}
	}
}

// sourceFile excludes the generator's own output, so a rerun reads the structs
// and not what it wrote last time.
func sourceFile(info fs.FileInfo) bool {
	name := info.Name()
	return !strings.HasSuffix(name, "_test.go") && name != FileName
}

func markedTypes(parsed *ast.Package, files *token.FileSet) ([]structType, error) {
	names := make([]string, 0, len(parsed.Files))
	for name := range parsed.Files {
		names = append(names, name)
	}
	// Files in name order, declarations in source order: the output must not
	// depend on how the filesystem happened to list a directory.
	sort.Strings(names)

	types := []structType{}
	for _, name := range names {
		for _, declaration := range parsed.Files[name].Decls {
			marked, is := markedDeclaration(declaration)
			if !is {
				continue
			}
			described, err := describe(marked, files)
			if err != nil {
				return nil, err
			}
			types = append(types, described)
		}
	}
	return types, nil
}

func markedDeclaration(declaration ast.Decl) (*ast.GenDecl, bool) {
	general, is := declaration.(*ast.GenDecl)
	if !is || general.Tok != token.TYPE || general.Doc == nil {
		return nil, false
	}
	for _, line := range general.Doc.List {
		if strings.TrimSpace(line.Text) == directive {
			return general, true
		}
	}
	return nil, false
}

func describe(declaration *ast.GenDecl, files *token.FileSet) (structType, error) {
	specification, is := declaration.Specs[0].(*ast.TypeSpec)
	if !is {
		return structType{}, fmt.Errorf("%s is not a type", where(files, declaration.Pos()))
	}
	structure, is := specification.Type.(*ast.StructType)
	if !is {
		return structType{}, fmt.Errorf("%s: %s is not a struct; a union is written by hand",
			where(files, specification.Pos()), specification.Name.Name)
	}

	described := structType{
		name: specification.Name.Name,
		doc:  prose(declaration.Doc),
	}
	for _, field := range structure.Fields.List {
		read, err := describeField(field, files)
		if err != nil {
			return structType{}, err
		}
		described.fields = append(described.fields, read...)
	}
	return described, nil
}
