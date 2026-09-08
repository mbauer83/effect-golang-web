package variant

// The walk that leaves fields out.
//
// It rebuilds the tree rather than mutating it, because a description is shared:
// two endpoints may hold the same one, and a derivation that edited it in place
// would change what the other publishes.

import (
	"fmt"

	"github.com/mbauer83/effect-golang-web/schema/structure"
)

func derived(node structure.Node, keeping supplied) (structure.Node, error) {
	if reference, named := node.(structure.Reference); named && reference.Resolve == nil {
		// A name with nothing behind it cannot be derived from, and passing it
		// through would publish a shape whose contents nobody can see.
		return nil, fmt.Errorf("%q: %w", reference.Name, ErrUnresolved)
	}
	object, isObject := resolved(node)
	if !isObject {
		return nil, ErrNotAnObject
	}

	fields := make([]structure.Field, 0, len(object.Fields))
	for _, field := range object.Fields {
		kept, keep, err := keptField(field, keeping)
		if err != nil {
			return nil, fmt.Errorf("field %q of %s: %w", field.Name, object.Name, err)
		}
		if keep {
			fields = append(fields, kept)
		}
	}
	if len(fields) == 0 {
		return nil, fmt.Errorf("%s: %w", object.Name, ErrNothingLeft)
	}

	return structure.Object{Name: object.Name, Doc: object.Doc, Fields: fields}, nil
}

// keptField decides one field's fate, and what it looks like if it survives.
func keptField(
	field structure.Field,
	keeping supplied,
) (structure.Field, bool, error) {
	switch {
	case field.Computed:
		return structure.Field{}, false, nil
	case field.Identity && !keeping.keepIdentity:
		return structure.Field{}, false, nil
	}

	entity, nested := entityCollected(field.Node)
	if nested && !keeping.reachIntoEntities {
		return structure.Field{}, false, nil
	}
	if nested {
		// The entity's own fields are derived the same way, so a computed
		// column on a child is left out of the child's shape too.
		inner, err := derivedWithin(field.Node, entity, keeping)
		if err != nil {
			return structure.Field{}, false, err
		}
		field.Node = inner
	}

	// The marks are the description's, not the derived shape's: a shape a
	// caller supplies has no identity to declare and nothing computed left in
	// it, so carrying the marks through would say something untrue about it.
	field.Identity = false
	field.Computed = false
	if keeping.partial {
		field.Optional = true
	}
	return field, true, nil
}

// derivedWithin derives a nested entity, keeping whatever wraps it.
//
// A list of order lines is a list of a derived order line, and a nullable one is
// a nullable derived one. The wrapper survives because it says how many there
// are and whether there is one, which a derivation has no business changing.
func derivedWithin(
	node structure.Node,
	entity structure.Object,
	keeping supplied,
) (structure.Node, error) {
	// An entity nested inside another is created with its parent, so its own
	// identity is its to supply -- but it is never *updated* through the
	// parent, because it has an identity of its own to be selected by. Keeping
	// the outer decision is what makes that fall out.
	inner, err := derived(entity, keeping)
	if err != nil {
		return nil, err
	}
	return rewrapped(node, inner)
}

// rewrapped puts a derived element back inside whatever held the original.
func rewrapped(node structure.Node, inner structure.Node) (structure.Node, error) {
	switch held := node.(type) {
	case structure.Object:
		return inner, nil
	case structure.Reference:
		return inner, nil
	case structure.Sequence:
		return structure.Sequence{Element: inner, Constraints: held.Constraints}, nil
	case structure.Nullable:
		return structure.Nullable{Inner: inner}, nil
	case structure.Mapping:
		return structure.Mapping{Key: held.Key, Value: inner}, nil
	default:
		return nil, fmt.Errorf("%T holds an entity and this walk cannot rebuild it", node)
	}
}

// entityCollected is the entity a field carries, through whatever wraps it,
// including a map.
//
// Deliberately not structure.EntityBehind, and the difference is the map. That
// one answers the storage question -- does this field get a table -- and a map
// of entities does not, because its key would need somewhere of its own to live
// and the description does not name it. This answers a different question:
// whether the caller creates these separately. A map of entities is still a map
// of things with identities, so a create shape leaves them out for the same
// reason a list of them is left out.
//
// Two questions that agree about everything except a map, so they are two
// functions rather than one with a flag.
func entityCollected(node structure.Node) (structure.Object, bool) {
	switch held := node.(type) {
	case structure.Object:
		return held, held.IsEntity()
	case structure.Reference:
		object, isObject := resolved(held)
		return object, isObject && object.IsEntity()
	case structure.Sequence:
		return entityCollected(held.Element)
	case structure.Nullable:
		return entityCollected(held.Inner)
	case structure.Mapping:
		return entityCollected(held.Value)
	default:
		return structure.Object{}, false
	}
}

// resolved is the object a node is, following one reference.
func resolved(node structure.Node) (structure.Object, bool) {
	switch held := node.(type) {
	case structure.Object:
		return held, true
	case structure.Reference:
		if held.Resolve == nil {
			return structure.Object{}, false
		}
		return resolved(held.Resolve())
	default:
		return structure.Object{}, false
	}
}
