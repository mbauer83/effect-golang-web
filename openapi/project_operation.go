package openapi

// One operation: what it reads, what it accepts, and what it can answer with.

import (
	"net/http"
	"slices"
	"strings"

	"github.com/mbauer83/effect-golang-schema/schema/jsonschema"
	"github.com/mbauer83/effect-golang-web/web"
)

func operation(declared contributed, shapes *cursor) Operation {
	return Operation{
		Method:      declared.declaration.Method,
		ID:          identify(declared.declaration),
		Summary:     declared.declaration.Summary,
		Description: declared.declaration.Doc,
		Parameters:  withPathTemplate(declared.declaration, parameters(declared, shapes)),
		RequestBody: requestBody(declared, shapes),
		Responses:   responses(declared, shapes),
	}
}

// withPathTemplate adds a parameter for every captured segment the codecs do
// not read.
//
// The specification requires an operation to declare all of its path
// template's parameters, and a route may legitimately capture a segment whose
// value its handler has no use for. Such a segment is still part of the path, so
// it is described as the string it is rather than left out to make the document
// invalid.
func withPathTemplate(declared web.Declaration, described []Parameter) []Parameter {
	read := make(map[string]bool, len(described))
	for _, parameter := range described {
		if parameter.In == string(web.InPath) {
			read[parameter.Name] = true
		}
	}
	for _, name := range templateParameters(declared.Path) {
		if !read[name] {
			described = append(described, Parameter{
				Name: name, In: string(web.InPath), Required: true,
				Schema: jsonschema.Node{Type: "string"},
			})
		}
	}
	return described
}

// templateParameters names the captured segments of a path template.
func templateParameters(path string) []string {
	names := []string{}
	for _, part := range strings.Split(path, "/") {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			names = append(names, strings.TrimSuffix(strings.Trim(part, "{}"), "..."))
		}
	}
	return names
}

// templatePath writes the path the way OpenAPI spells one.
//
// A wildcard has no counterpart in the specification: {path...} matches the
// rest, and a document can only say that one parameter is there. Describing it
// as an ordinary parameter is the closest honest thing, and it is why a
// wildcard route's document says less than the route knows.
func templatePath(path string) string {
	parts := strings.Split(path, "/")
	for index, part := range parts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			parts[index] = "{" + strings.TrimSuffix(strings.Trim(part, "{}"), "...") + "}"
		}
	}
	return strings.Join(parts, "/")
}

func parameters(declared contributed, shapes *cursor) []Parameter {
	described := make([]Parameter, 0, declared.parameters)
	for index := 0; index < declared.parameters; index++ {
		parameter := declared.declaration.Parameters[index]
		described = append(described, Parameter{
			Name:        parameter.Name,
			In:          string(parameter.In),
			Required:    parameter.Required,
			Description: parameter.Doc,
			Schema:      shapes.next(),
		})
	}
	return described
}

func requestBody(declared contributed, shapes *cursor) *RequestBody {
	if !declared.hasEntity {
		return nil
	}
	// A declared entity is required. A codec that read an optional body would
	// be describing two shapes, and would say so itself.
	return &RequestBody{
		Required:  true,
		MediaType: declared.declaration.Entity.MediaType,
		Schema:    shapes.next(),
	}
}

// responses lists the success the endpoint declared and the failures it
// documented, ordered by status so the same routes render identically.
func responses(declared contributed, shapes *cursor) []Response {
	described := []Response{success(declared, shapes)}
	for _, failure := range declared.declaration.Failures {
		described = append(described, Response{
			Status:      failure.Status,
			Description: describing(failure.Status, failure.Doc),
		})
	}
	slices.SortStableFunc(described, func(first Response, second Response) int {
		return first.Status - second.Status
	})
	return described
}

func success(declared contributed, shapes *cursor) Response {
	answered := Response{
		Status:      declared.declaration.Status,
		Description: describing(declared.declaration.Status, ""),
	}
	if declared.declaration.Content != nil {
		answered.MediaType = declared.declaration.Content.MediaType
	}
	if declared.hasContent {
		shape := shapes.next()
		answered.Schema = &shape
	}
	return answered
}

// describing supplies the description the specification requires, from the
// status itself when the endpoint said nothing.
func describing(status int, doc string) string {
	if doc != "" {
		return doc
	}
	if text := http.StatusText(status); text != "" {
		return text
	}
	return "Response"
}

// identify derives an operation identifier from the method and the path.
//
// It is derived rather than declared so that it exists at all and is stable:
// GET /books/{title} is getBooksByTitle every time, whoever generates the
// document.
func identify(declared web.Declaration) string {
	parts := []string{strings.ToLower(declared.Method)}
	for _, part := range strings.Split(strings.TrimPrefix(declared.Path, "/"), "/") {
		switch {
		case part == "":
			continue
		case strings.HasPrefix(part, "{"):
			parts = append(parts, "By"+capitalise(strings.Trim(part, "{}.")))
		default:
			parts = append(parts, capitalise(part))
		}
	}
	return strings.Join(parts, "")
}

func capitalise(word string) string {
	if word == "" {
		return word
	}
	return strings.ToUpper(word[:1]) + word[1:]
}
