package web

import (
	"errors"
	"strings"
)

// A route's path is a pattern over segments. Three kinds, and the order they
// are tried in is the order they are listed: a literal is more specific than a
// capture, and a capture is more specific than a wildcard.
type segmentKind uint8

const (
	literalSegment segmentKind = iota
	captureSegment
	wildcardSegment
)

// segment is one part of a pattern. text is the literal for a literal segment
// and the capture name for the other two.
type segment struct {
	kind segmentKind
	text string
}

// parsePattern reads a path pattern.
//
//	/books                a literal
//	/books/{title}        one captured segment
//	/files/{path...}      a wildcard capturing the rest, only at the end
//
// A trailing slash is part of the pattern rather than something to normalise
// away: /books and /books/ are different paths, and a router that quietly
// redirected between them would be guessing.
func parsePattern(path string) ([]segment, error) {
	if !strings.HasPrefix(path, "/") {
		return nil, faulted("reading the path "+path, errUnrootedPath)
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	segments := make([]segment, 0, len(parts))
	captured := make(map[string]bool, len(parts))

	for index, part := range parts {
		parsed, err := parseSegment(part, index == len(parts)-1)
		if err != nil {
			return nil, faulted("reading the path "+path, err)
		}
		if parsed.kind != literalSegment {
			if captured[parsed.text] {
				return nil, faulted("reading the path "+path,
					errors.New("two segments are captured as "+parsed.text))
			}
			captured[parsed.text] = true
		}
		segments = append(segments, parsed)
	}
	return segments, nil
}

func parseSegment(part string, last bool) (segment, error) {
	if !strings.HasPrefix(part, "{") {
		if strings.ContainsAny(part, "{}") {
			return segment{}, errors.New("a capture takes the whole segment: " + part)
		}
		return segment{kind: literalSegment, text: part}, nil
	}
	if !strings.HasSuffix(part, "}") {
		return segment{}, errors.New("a capture is not closed: " + part)
	}

	name := part[1 : len(part)-1]
	if rest, wildcard := strings.CutSuffix(name, "..."); wildcard {
		if !last {
			return segment{}, errors.New("a wildcard captures the rest and so comes last: " + part)
		}
		return namedSegment(wildcardSegment, rest)
	}
	return namedSegment(captureSegment, name)
}

func namedSegment(kind segmentKind, name string) (segment, error) {
	if strings.TrimSpace(name) == "" {
		return segment{}, errNamelessCapture
	}
	return segment{kind: kind, text: name}, nil
}

// pathSegments splits a request path the same way a pattern is split, so the
// two are compared on the same terms.
func pathSegments(path string) []string {
	return strings.Split(strings.TrimPrefix(path, "/"), "/")
}

// renderPattern writes a pattern back out, for the message a construction
// mistake reports and for a published document.
func renderPattern(segments []segment) string {
	rendered := make([]string, 0, len(segments))
	for _, part := range segments {
		switch part.kind {
		case captureSegment:
			rendered = append(rendered, "{"+part.text+"}")
		case wildcardSegment:
			rendered = append(rendered, "{"+part.text+"...}")
		default:
			rendered = append(rendered, part.text)
		}
	}
	return "/" + strings.Join(rendered, "/")
}

var (
	errUnrootedPath    = errors.New("a path begins with /")
	errNamelessCapture = errors.New("a capture has no name")
)
