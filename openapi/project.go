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
	gathered := gather(declarations)
	projected, components := jsonschema.ProjectAllReferencing(ComponentPointer, gathered.nodes...)
	shapes := &cursor{projected: projected}

	document := Document{Info: info, Servers: servers, Components: components}
	for _, declared := range gathered.declarations {
		document.add(templatePath(declared.declaration.Path), operation(declared, shapes))
	}
	return document
}

// gathered holds every declaration together with the shapes it contributes, in
// the one order both the projection and the assembly walk them in.
type gathering struct {
	declarations []contributed
	nodes        []structure.Node
}

type contributed struct {
	declaration web.Declaration
	parameters  int
	hasEntity   bool
	hasContent  bool
}

// cursor hands out the projected shapes in the order they were collected. The
// two walks are kept in step by construction rather than by counting twice.
type cursor struct {
	projected []jsonschema.Node
	at        int
}

func (shapes *cursor) next() jsonschema.Node {
	node := shapes.projected[shapes.at]
	shapes.at++
	return node
}

func gather(declarations []web.Declaration) gathering {
	collected := gathering{}
	for _, declared := range declarations {
		contribution := contributed{
			declaration: declared,
			parameters:  len(declared.Parameters),
			hasEntity:   declared.Entity != nil && declared.Entity.Node != nil,
			// A content entry may name a media type and describe no shape, for
			// an entity this program did not build from a value.
			hasContent: declared.Content != nil && declared.Content.Node != nil,
		}
		for _, parameter := range declared.Parameters {
			collected.nodes = append(collected.nodes, parameter.Node)
		}
		if contribution.hasEntity {
			collected.nodes = append(collected.nodes, declared.Entity.Node)
		}
		if contribution.hasContent {
			collected.nodes = append(collected.nodes, declared.Content.Node)
		}
		collected.declarations = append(collected.declarations, contribution)
	}
	return collected
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
