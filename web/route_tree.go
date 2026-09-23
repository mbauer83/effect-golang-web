package web

// Matching is a tree over path segments. A literal is tried before a capture
// and a capture before a wildcard, so the most specific pattern that can match
// does -- and because the tree is walked with backtracking, a literal that
// matches one segment but leads nowhere does not shadow a capture that would
// have matched the whole path.

import (
	"errors"
	"strings"
)

type treeNode[R, E any] struct {
	literals map[string]*treeNode[R, E]
	capture  *treeNode[R, E]
	wildcard *treeNode[R, E]
	// name is what a capture or wildcard node binds its segment to.
	name string
	// handlers and patterns are keyed by method. patterns is kept so an
	// ambiguity can name both routes that caused it.
	handlers map[string]Handler[R, E]
	patterns map[string]string
}

// binding is one captured segment. It is a slice rather than a map while
// matching, because a branch that fails must leave nothing behind.
type binding struct {
	name  string
	value string
}

// resolution is what a walk found: a handler, or the methods the matched path
// does allow, which is the difference between 405 and 404.
type resolution[R, E any] struct {
	handler  Handler[R, E]
	found    bool
	captures []binding
	methods  []string
}

func newTreeNode[R, E any]() *treeNode[R, E] {
	return &treeNode[R, E]{
		literals: map[string]*treeNode[R, E]{},
		handlers: map[string]Handler[R, E]{},
		patterns: map[string]string{},
	}
}

// insert adds one route, or reports the route already there that could match
// the same request. Ambiguity is a construction mistake rather than a silent
// precedence rule discovered at run time.
func (node *treeNode[R, E]) insert(
	segments []segment,
	method string,
	handler Handler[R, E],
	pattern string,
) error {
	if len(segments) == 0 {
		if prior, taken := node.patterns[method]; taken {
			return errors.New(method + " " + prior + " and " + method + " " + pattern +
				" could match the same request")
		}
		node.handlers[method] = handler
		node.patterns[method] = pattern
		return nil
	}

	child, err := node.childFor(segments[0], pattern)
	if err != nil {
		return err
	}
	return child.insert(segments[1:], method, handler, pattern)
}

// childFor finds or creates the node one segment leads to. Two routes that
// capture the same position under different names are refused: they would share
// this node, and one of the two names would silently never be bound.
func (node *treeNode[R, E]) childFor(part segment, pattern string) (*treeNode[R, E], error) {
	switch part.kind {
	case literalSegment:
		if _, present := node.literals[part.text]; !present {
			node.literals[part.text] = newTreeNode[R, E]()
		}
		return node.literals[part.text], nil
	case captureSegment:
		return claimSlot(&node.capture, part.text, pattern)
	default:
		return claimSlot(&node.wildcard, part.text, pattern)
	}
}

func claimSlot[R, E any](slot **treeNode[R, E], name string, pattern string) (*treeNode[R, E], error) {
	if *slot == nil {
		*slot = newTreeNode[R, E]()
		(*slot).name = name
		return *slot, nil
	}
	if (*slot).name != name {
		return nil, errors.New(pattern + " captures a segment as " + name +
			" where another route captures it as " + (*slot).name)
	}
	return *slot, nil
}

// resolve walks the remaining path. It tries the branches in order of
// specificity and keeps looking after a branch that matched the path but not
// the method, because a less specific branch may serve the method.
func (node *treeNode[R, E]) resolve(path []string, method string, bindings []binding) resolution[R, E] {
	if len(path) == 0 {
		return node.here(method, bindings)
	}

	methods := []string{}
	if literal, present := node.literals[path[0]]; present {
		match := literal.resolve(path[1:], method, bindings)
		if match.found {
			return match
		}
		methods = append(methods, match.methods...)
	}
	if node.capture != nil {
		match := node.capture.resolve(path[1:], method, bind(bindings, node.capture.name, path[0]))
		if match.found {
			return match
		}
		methods = append(methods, match.methods...)
	}
	if node.wildcard != nil {
		match := node.wildcard.here(method, bind(bindings, node.wildcard.name, strings.Join(path, "/")))
		if match.found {
			return match
		}
		methods = append(methods, match.methods...)
	}
	return resolution[R, E]{methods: methods}
}

// here answers for a path that ends at this node.
func (node *treeNode[R, E]) here(method string, bindings []binding) resolution[R, E] {
	if handler, served := node.handlers[method]; served {
		return resolution[R, E]{handler: handler, found: true, captures: bindings}
	}
	methods := make([]string, 0, len(node.handlers))
	for key := range node.handlers {
		methods = append(methods, key)
	}
	return resolution[R, E]{methods: methods}
}

// bind appends a capture.
//
// Sibling branches may share a backing array and a branch that fails may have
// written into it, which is harmless: a slice's length governs what it can see,
// every branch appends at its own level's length, and the walk returns the
// moment one branch succeeds. Copying here would be defensive against something
// that cannot happen, so it does not.
func bind(bindings []binding, name string, value string) []binding {
	return append(bindings, binding{name: name, value: value})
}

// captureValues turns the bindings into what a request carries.
func captureValues(bindings []binding) map[string]string {
	if len(bindings) == 0 {
		return nil
	}
	values := make(map[string]string, len(bindings))
	for _, one := range bindings {
		values[one.name] = one.value
	}
	return values
}
