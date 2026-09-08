package evolve

// An aggregate's versions, and the steps between them.
//
// Version one is declared and every later one is derived, which is the whole
// reason a step and a declaration cannot disagree about what the declaration
// became: there is nothing to disagree with.

import (
	"fmt"
	"strings"

	"github.com/mbauer83/effect-golang-web/schema/structure"
)

// History is what an aggregate has been.
//
// Identified by a fully-qualified name rather than by a Go package path or a
// struct name, because that is what makes a declaration a contract between
// services rather than a detail of one program -- the same reason a protobuf
// service carries its own.
type History struct {
	name     string
	versions []structure.Object
	steps    [][]Change
	fault    error
}

// From starts a history at version one.
func From(name string, node structure.Node) History {
	object, isObject := node.(structure.Object)
	if !isObject {
		return History{name: name, fault: errNotAnObject}
	}
	if strings.TrimSpace(name) == "" {
		return History{fault: errNoName}
	}
	return History{name: name, versions: []structure.Object{object}}
}

// Then declares the next version as a list of changes to the one before it.
//
// The version it makes is derived, so the numbers are the positions in this
// chain and are monotonic by construction. A step with no changes is refused:
// a version that is the same as the one before it is not a version, and
// numbering one would make every consumer look for a difference there is none
// of.
func (history History) Then(changes ...Change) History {
	if history.fault != nil {
		return history
	}
	if len(changes) == 0 {
		history.fault = fmt.Errorf("version %d: %w", len(history.versions)+1, errNoChanges)
		return history
	}

	before := history.versions[len(history.versions)-1]
	after := before
	for at, change := range changes {
		applied, err := change.applied(after)
		if err != nil {
			history.fault = fmt.Errorf("%s in version %d of %s: %w",
				change.describe(), len(history.versions)+1, history.name, err)
			return history
		}
		after = applied
		_ = at
	}

	history.versions = append(history.versions, after)
	history.steps = append(history.steps, changes)
	return history
}

// Fault is why the history cannot be used, if it cannot.
//
// A declaration mistake is reported here rather than when something tries to
// migrate, which is the same rule the routing tree and an endpoint follow. Go
// cannot check a step's totality at compile time -- a version is a value and
// not a type -- so this is where it is checked, and a test that asks is a test
// that has checked it.
func (history History) Fault() error {
	return history.fault
}

// Name is the fully-qualified name this history is of.
func (history History) Name() string { return history.name }

// Latest is the most recent version's number.
func (history History) Latest() int { return len(history.versions) }

// At is the description as of one version.
func (history History) At(version int) (structure.Node, error) {
	object, err := history.objectAt(version)
	if err != nil {
		return nil, err
	}
	return object, nil
}

func (history History) objectAt(version int) (structure.Object, error) {
	if history.fault != nil {
		return structure.Object{}, history.fault
	}
	if version < 1 || version > len(history.versions) {
		return structure.Object{}, fmt.Errorf("%s has versions 1 to %d, and %d: %w",
			history.name, len(history.versions), version, errNoSuchVersion)
	}
	return history.versions[version-1], nil
}

// Between is the changes that take one version to another, in the order they
// have to be applied.
//
// Either direction: going back inverts each change and reverses the order,
// which is what makes N-1 declared steps answer every one of the N-squared
// pairs. Nothing is precomputed, because there is nothing to precompute --
// folding a handful of changes costs less than remembering the answer.
func (history History) Between(from int, to int) ([]Change, error) {
	if _, err := history.objectAt(from); err != nil {
		return nil, err
	}
	if _, err := history.objectAt(to); err != nil {
		return nil, err
	}
	if from == to {
		return nil, nil
	}
	if from < to {
		return history.forward(from, to), nil
	}
	return history.backward(from, to)
}

func (history History) forward(from int, to int) []Change {
	changes := []Change{}
	for at := from; at < to; at++ {
		changes = append(changes, history.steps[at-1]...)
	}
	return changes
}

// backward inverts each change and reverses the order twice over: the steps run
// in reverse, and the changes within each step do too.
//
// Both are necessary. A step that renamed a field and then removed another has
// to put the other back before undoing the rename, or the inverse would be
// looking for a field under a name it no longer has.
func (history History) backward(from int, to int) ([]Change, error) {
	changes := []Change{}
	for at := from; at > to; at-- {
		before := history.versions[at-2]
		step := history.steps[at-2]
		// Within the step, each change's inverse needs the version as it was
		// just before that change was applied.
		states := make([]structure.Object, len(step))
		state := before
		for index, change := range step {
			states[index] = state
			applied, err := change.applied(state)
			if err != nil {
				return nil, err
			}
			state = applied
		}
		for index := len(step) - 1; index >= 0; index-- {
			inverted, err := step[index].inverse(states[index])
			if err != nil {
				return nil, fmt.Errorf("undoing %s in version %d of %s: %w",
					step[index].describe(), at, history.name, err)
			}
			changes = append(changes, inverted)
		}
	}
	return changes, nil
}
