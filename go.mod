module github.com/mbauer83/effect-golang-web

go 1.27.0

require (
	connectrpc.com/connect v1.21.0
	github.com/Azure/go-amqp v1.7.0
	// Test-only. Each checks what a projection or a codec emits against
	// something that has never seen this module: a real OpenAPI parser, the
	// canonical protobuf implementation and its compiler, and h2c so that
	// "it really is gRPC on the wire" can be read off a request rather than
	// asserted.
	github.com/bufbuild/protocompile v0.14.1
	github.com/coder/websocket v1.8.15
	github.com/getkin/kin-openapi v0.149.0
	github.com/mbauer83/effect-golang v0.0.0
	github.com/mbauer83/effect-golang-schema v0.0.0
	github.com/rabbitmq/amqp091-go v1.14.0
	golang.org/x/net v0.58.0
	google.golang.org/protobuf v1.36.12
)

require (
	github.com/go-openapi/jsonpointer v0.22.5 // indirect
	github.com/go-openapi/swag/jsonname v0.25.5 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/oasdiff/yaml v0.1.1 // indirect
	github.com/oasdiff/yaml3 v0.0.14 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/text v0.41.0 // indirect
)

// None of the three is published yet, so all are resolved from sibling working
// copies. A replace is ignored by anything that depends on *this* module, so it
// is a development arrangement and not a distribution one: replace all three
// with version requirements once they are tagged.
replace (
	github.com/mbauer83/effect-golang => ../effect-golang
	github.com/mbauer83/effect-golang-schema => ../effect-golang-schema
	github.com/mbauer83/effect-golang-sql => ../effect-golang-sql
)
