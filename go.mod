module github.com/mbauer83/effect-golang-web

go 1.27.0

require (
	github.com/mbauer83/effect-golang v0.0.0
	// A JSON Schema validator, used only by the tests: the projection is
	// checked against a parser that has never seen this module. Nothing in the
	// module itself needs it.
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3
)

require golang.org/x/text v0.14.0 // indirect

// effect-golang is not published yet, so it is resolved from the working copy
// next to this one. Replace this with a version requirement once it is tagged.
replace github.com/mbauer83/effect-golang => ../effect-golang
