package variant

// The three shapes, derived from one description.

import (
	"errors"

	"github.com/mbauer83/effect-golang-web/schema/structure"
)

// Select is the whole thing, which is the description unchanged.
//
// It exists so that the three shapes are named in one place and a reader is not
// left wondering whether "the schema" means the stored shape or the supplied
// one. A caller reading rows wants this.
func Select(node structure.Node) structure.Node {
	return node
}

// Create is what a caller supplies to make one.
//
// Every field except the computed ones: a value the database generates or a
// trigger overwrites is not the caller's to give, and asking for it would be
// asking a question with no answer. An identity the application generates stays,
// because it is the caller's to give -- which is why Identity and Computed are
// separate marks that compose.
//
// Nested entities are left out. They have identities of their own, so they are
// things rather than parts, and creating one is its own act. CreateWithEntities
// is for the caller that means to create the whole aggregate at once.
func Create(node structure.Node) (structure.Node, error) {
	return derived(node, supplied{keepIdentity: true, reachIntoEntities: false})
}

// CreateWithEntities is Create for an aggregate created in one act: the root and
// the entities beneath it together.
//
// It is a separate function rather than a flag because the two are different
// requests with different shapes, and a boolean at the call site would not say
// which of them was meant.
func CreateWithEntities(node structure.Node) (structure.Node, error) {
	return derived(node, supplied{keepIdentity: true, reachIntoEntities: true})
}

// Update is what a caller supplies to change one.
//
// Every field except the computed ones and the identity. The identity is left
// out because it selects which thing is being changed rather than being part of
// what it is being changed to -- a caller that could send a new identity would
// be able to ask for something no store can do.
//
// Every remaining field becomes optional, because a change describes what is
// changing and a field nobody mentioned is a field nobody is changing. That is
// the difference between an update shape and a create shape with the identity
// removed, and it is why this is not that.
func Update(node structure.Node) (structure.Node, error) {
	return derived(node, supplied{keepIdentity: false, reachIntoEntities: false, partial: true})
}

// UpdateWithEntities is Update reaching into the entities beneath the root.
func UpdateWithEntities(node structure.Node) (structure.Node, error) {
	return derived(node, supplied{keepIdentity: false, reachIntoEntities: true, partial: true})
}

// supplied says which fields a derived shape keeps.
type supplied struct {
	keepIdentity      bool
	reachIntoEntities bool
	partial           bool
}

var (
	// ErrNotAnObject is returned for a description that has no fields to
	// derive from. A scalar has one shape in every role.
	ErrNotAnObject = errors.New(
		"a derived shape is a shape with fields left out, and this description has none")
	// ErrNothingLeft is returned when every field was left out. A shape a
	// caller cannot put anything into is not a shape worth publishing, and it
	// usually means every field was marked computed by mistake.
	ErrNothingLeft = errors.New(
		"every field was left out, so the derived shape would hold nothing")
	// ErrUnresolved is returned for a reference the walker cannot follow. A
	// derived shape has to be built, so a name with nothing behind it is not
	// something to pass through.
	ErrUnresolved = errors.New("a reference has nothing behind it to derive from")
)
