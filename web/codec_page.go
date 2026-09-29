package web

// How a client asks for one page of a list.

import (
	"fmt"
	"slices"
	"strings"

	"github.com/mbauer83/effect-golang-schema/schema"
)

// PageRequest is which page of a list a client asks for: in one of the sorts
// the list offers, after or before a cursor a page gave it or by number, and of
// a size. A zero field is the list's default; the list decides the rest.
type PageRequest struct {
	Sort   string
	After  string
	Before string
	Number int
	Size   int
}

var errTwoPositions = requestMistake("a page is after a cursor, before one, or numbered, and not two of those")

// PageParams reads a page request from the query -- sort, after, before, page
// and size -- offering the sorts named, the first of which a client gets
// without asking. A sort the list does not offer, or two positions at once, is
// refused as the client's mistake, and the parameters say so in the document.
func PageParams(sorts ...string) Codec[PageRequest] {
	positive := schema.Int().Check(schema.AtLeast(1))
	offered := "one of " + strings.Join(sorts, ", ")
	if len(sorts) > 0 {
		offered += "; " + sorts[0] + " unless asked"
	}
	read := Struct(
		FieldOf(OptionalQueryParam("sort", schema.Text()).WithDescription("the order to read in: "+offered),
			func(page *PageRequest, sort *string) { page.Sort = given(sort) }),
		FieldOf(OptionalQueryParam("after", schema.Text()).WithDescription("the cursor a page gave as its next"),
			func(page *PageRequest, after *string) { page.After = given(after) }),
		FieldOf(OptionalQueryParam("before", schema.Text()).WithDescription("the cursor a page gave as its previous"),
			func(page *PageRequest, before *string) { page.Before = given(before) }),
		FieldOf(OptionalQueryParam("page", positive).WithDescription("a numbered page, counted from 1"),
			func(page *PageRequest, number *int) { page.Number = given(number) }),
		FieldOf(OptionalQueryParam("size", positive).WithDescription("how many rows a page holds"),
			func(page *PageRequest, size *int) { page.Size = given(size) }),
	)
	return Convert(read, func(request PageRequest) (PageRequest, error) {
		if request.Sort != "" && !slices.Contains(sorts, request.Sort) {
			return PageRequest{}, fmt.Errorf("no sort is called %q; the list offers %s", request.Sort, offered)
		}
		positions := 0
		for _, set := range []bool{request.After != "", request.Before != "", request.Number != 0} {
			if set {
				positions++
			}
		}
		if positions > 1 {
			return PageRequest{}, errTwoPositions
		}
		return request, nil
	})
}

// given is an optional parameter's value, or the zero value for "the list's
// default" when it was not given.
func given[A any](value *A) A {
	if value == nil {
		var zero A
		return zero
	}
	return *value
}
