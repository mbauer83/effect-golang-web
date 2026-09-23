package openapi

import (
	"github.com/mbauer83/effect-golang-schema/schema/jsonschema"
	"github.com/mbauer83/effect-golang-schema/schema/structure"
	"github.com/mbauer83/effect-golang-web/web"
)

// Describe projects route declarations into a document.
//
// Every shape goes through one projection, so a type used by ten operations
// becomes one component that all ten refer to -- which is the reason the schema
// layer exposes its structure at all.
func Describe(info Info, declarations []web.Declaration, servers ...Server) Document {
	inventory := gather(declarations)
	schemas, components := jsonschema.ProjectAllWithPointer(ComponentPointer, inventory.nodes...)
	shapes := &cursor{schemas: schemas}

	document := Document{Info: info, Servers: servers, Components: components}
	for _, endpoint := range inventory.declarations {
		document.add(templatePath(endpoint.declaration.Path), operation(endpoint, shapes))
	}
	return document
}

// gathered holds every declaration together with the shapes it contributes, in
// the one order both the projection and the assembly walk them in.
type collection struct {
	declarations []entry
	nodes        []structure.Node
}

type entry struct {
	declaration web.Declaration
	parameters  int
	hasEntity   bool
	hasContent  bool
}

// cursor hands out the projected shapes in the order they were collected. The
// two walks are kept in step by construction rather than by counting twice.
type cursor struct {
	schemas []jsonschema.Node
	at      int
}

func (shapes *cursor) next() jsonschema.Node {
	node := shapes.schemas[shapes.at]
	shapes.at++
	return node
}

func gather(declarations []web.Declaration) collection {
	inventory := collection{}
	for _, declaration := range declarations {
		contribution := entry{
			declaration: declaration,
			parameters:  len(declaration.Parameters),
			hasEntity:   declaration.Entity != nil && declaration.Entity.Node != nil,
			// A content entry may name a media type and describe no shape, for
			// an entity this program did not build from a value.
			hasContent: declaration.Content != nil && declaration.Content.Node != nil,
		}
		for _, parameter := range declaration.Parameters {
			inventory.nodes = append(inventory.nodes, parameter.Node)
		}
		if contribution.hasEntity {
			inventory.nodes = append(inventory.nodes, declaration.Entity.Node)
		}
		if contribution.hasContent {
			inventory.nodes = append(inventory.nodes, declaration.Content.Node)
		}
		inventory.declarations = append(inventory.declarations, contribution)
	}
	return inventory
}

// add puts an operation under its path, merging with a path already present,
// because a document has one entry per path however the routes were written.
func (document *Document) add(path string, operation Operation) {
	for index := range document.Paths {
		if document.Paths[index].Path == path {
			document.Paths[index].Operations = append(document.Paths[index].Operations, operation)
			return
		}
	}
	document.Paths = append(document.Paths, Path{Path: path, Operations: []Operation{operation}})
}
