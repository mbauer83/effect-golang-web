package web

// How a client asks for one page of a list.

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/mbauer83/effect-golang-schema/schema"
	"github.com/mbauer83/effect-golang/effect"
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

var errTwoPositions = errors.New("a page is after a cursor, before one, or numbered, and not two of those")

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
	return Convert(
		Zip(
			Zip(
				OptionalQueryParam("sort", schema.Text()).WithDescription("the order to read in: "+offered),
				OptionalQueryParam("after", schema.Text()).WithDescription("the cursor a page gave as its next"),
			),
			Zip(
				OptionalQueryParam("before", schema.Text()).WithDescription("the cursor a page gave as its previous"),
				Zip(
					OptionalQueryParam("page", positive).WithDescription("a numbered page, counted from 1"),
					OptionalQueryParam("size", positive).WithDescription("how many rows a page holds"),
				),
			),
		),
		func(params effect.Product[effect.Product[*string, *string], effect.Product[*string, effect.Product[*int, *int]]]) (PageRequest, error) {
			request := PageRequest{
				Sort:   given(params.First.First),
				After:  given(params.First.Second),
				Before: given(params.Second.First),
				Number: givenNumber(params.Second.Second.First),
				Size:   givenNumber(params.Second.Second.Second),
			}
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
		},
	)
}

func given(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func givenNumber(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}
