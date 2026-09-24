package web

// How a surface spells the members of the documents it reads and writes: one
// naming strategy for the whole surface, applied to its codecs and to what it
// declares, so a published document says what goes over the wire.

import (
	"github.com/mbauer83/effect-golang-schema/schema/naming"
	"github.com/mbauer83/effect-golang-schema/schema/structure"
)

// WithNaming returns the surface spelling the members of every document it
// reads and writes by strategy -- camelCase for a JSON API, most often -- and
// declaring them spelled that way, so a published document says what goes over
// the wire.
//
// It is the surface's, not a route's, because a client meets one API: an API
// that spelled a member one way in one response and another way in the next
// would be one nobody could write a client for.
func (routes Routes[R, E]) WithNaming(strategy naming.Strategy) Routes[R, E] {
	if len(routes.routes) == 0 {
		return routes
	}
	copies := make([]Route[R, E], 0, len(routes.routes))
	for _, route := range routes.routes {
		copies = append(copies, route.withNaming(strategy))
	}
	surface, err := NewRoutesWithRejection(routes.reject, copies...)
	if err != nil {
		return routes
	}
	return surface
}

// withNaming returns the route reading and writing its documents spelled by
// strategy, and declaring them so.
func (route Route[R, E]) withNaming(strategy naming.Strategy) Route[R, E] {
	if route.fault != nil {
		return route
	}
	route.strategy = strategy
	route.declaration = route.declaration.withNaming(strategy)
	return route
}

// spelled is the declaration with the documents it reads and writes described
// as a surface with strategy spells them.
func (declaration Declaration) withNaming(strategy naming.Strategy) Declaration {
	declaration.Entity = declaration.Entity.withNaming(strategy)
	declaration.Content = declaration.Content.withNaming(strategy)
	return declaration
}

func (content *Content) withNaming(strategy naming.Strategy) *Content {
	if content == nil || content.Node == nil {
		return content
	}
	named := *content
	named.Node = structure.Spell(content.Node, strategy)
	return &named
}
