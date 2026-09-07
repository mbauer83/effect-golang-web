module github.com/mbauer83/effect-golang-web

go 1.27.0

require (
	github.com/getkin/kin-openapi v0.149.0
	github.com/mbauer83/effect-golang v0.0.0
	// A JSON Schema validator, used only by the tests: the projection is
	// checked against a parser that has never seen this module. Nothing in the
	// module itself needs it.
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3
)

require (
	github.com/go-openapi/jsonpointer v0.22.5 // indirect
	github.com/go-openapi/swag/jsonname v0.25.5 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/oasdiff/yaml v0.1.1 // indirect
	github.com/oasdiff/yaml3 v0.0.14 // indirect
	github.com/rogpeppe/go-internal v1.16.0 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	golang.org/x/text v0.14.0 // indirect
)

// effect-golang is not published yet, so it is resolved from the working copy
// next to this one. Replace this with a version requirement once it is tagged.
replace github.com/mbauer83/effect-golang => ../effect-golang
