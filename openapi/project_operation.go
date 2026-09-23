package openapi

// One operation: what it reads, what it accepts, and what it can answer with.

import (
	"net/http"
	"slices"
	"strings"

	"github.com/mbauer83/effect-golang-schema/schema/jsonschema"
	"github.com/mbauer83/effect-golang-web/web"
)

func operation(endpoint entry, shapes *cursor) Operation {
	return Operation{
		Method:      endpoint.declaration.Method,
		ID:          identify(endpoint.declaration),
		Summary:     endpoint.declaration.Summary,
		Description: endpoint.declaration.Doc,
		Parameters:  withPathTemplate(endpoint.declaration, parameters(endpoint, shapes)),
		RequestBody: requestBody(endpoint, shapes),
		Responses:   responses(endpoint, shapes),
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
func withPathTemplate(declaration web.Declaration, list []Parameter) []Parameter {
	inPath := make(map[string]bool, len(list))
	for _, parameter := range list {
		if parameter.In == string(web.InPath) {
			inPath[parameter.Name] = true
		}
	}
	for _, name := range templateParameters(declaration.Path) {
		if !inPath[name] {
			list = append(list, Parameter{
				Name: name, In: string(web.InPath), Required: true,
				Schema: jsonschema.Node{Type: "string"},
			})
		}
	}
	return list
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

func parameters(endpoint entry, shapes *cursor) []Parameter {
	list := make([]Parameter, 0, endpoint.parameters)
	for index := 0; index < endpoint.parameters; index++ {
		parameter := endpoint.declaration.Parameters[index]
		list = append(list, Parameter{
			Name:        parameter.Name,
			In:          string(parameter.In),
			Required:    parameter.Required,
			Description: parameter.Doc,
			Schema:      shapes.next(),
		})
	}
	return list
}

func requestBody(endpoint entry, shapes *cursor) *RequestBody {
	if !endpoint.hasEntity {
		return nil
	}
	// A declared entity is required. A codec that read an optional body would
	// be describing two shapes, and would say so itself.
	return &RequestBody{
		Required:  true,
		MediaType: endpoint.declaration.Entity.MediaType,
		Schema:    shapes.next(),
	}
}

// responses lists the success the endpoint declared and the failures it
// documented, ordered by status so the same routes render identically.
func responses(endpoint entry, shapes *cursor) []Response {
	list := []Response{success(endpoint, shapes)}
	for _, failure := range endpoint.declaration.Failures {
		list = append(list, Response{
			Status:      failure.Status,
			Description: responseDescription(failure.Status, failure.Doc),
		})
	}
	slices.SortStableFunc(list, func(first Response, second Response) int {
		return first.Status - second.Status
	})
	return list
}

func success(endpoint entry, shapes *cursor) Response {
	response := Response{
		Status:      endpoint.declaration.Status,
		Description: responseDescription(endpoint.declaration.Status, ""),
	}
	if endpoint.declaration.Content != nil {
		response.MediaType = endpoint.declaration.Content.MediaType
	}
	if endpoint.hasContent {
		shape := shapes.next()
		response.Schema = &shape
	}
	return response
}

// responseDescription supplies the description the specification requires, from the
// status itself when the endpoint said nothing.
func responseDescription(status int, doc string) string {
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
func identify(declaration web.Declaration) string {
	parts := []string{strings.ToLower(declaration.Method)}
	for _, part := range strings.Split(strings.TrimPrefix(declaration.Path, "/"), "/") {
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
