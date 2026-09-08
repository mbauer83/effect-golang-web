module github.com/mbauer83/effect-golang-web

go 1.27.0

require (
	connectrpc.com/connect v1.21.0
	github.com/Azure/go-amqp v1.7.0
	// A protobuf compiler and the reference implementation, used only by the
	// tests: the proto3 projection is compiled by a real compiler and the wire
	// codec is checked against the canonical encoder in both directions.
	// Nothing in the module itself needs either, which is what keeps the
	// schema layer free of a dependency every user would acquire.
	github.com/bufbuild/protocompile v0.14.1
	github.com/coder/websocket v1.8.15
	github.com/getkin/kin-openapi v0.149.0
	// The two drivers the generated DDL is checked against, used only by the
	// tests: the derivation is established with sqlite, and these are what say
	// the statements are ones Postgres and MySQL accept. Nothing in the module
	// imports either, because sql depends on a port and not on a driver.
	github.com/go-sql-driver/mysql v1.10.1
	github.com/jackc/pgx/v5 v5.11.0
	github.com/mbauer83/effect-golang v0.0.0
	github.com/rabbitmq/amqp091-go v1.14.0
	// A JSON Schema validator, used only by the tests: the projection is
	// checked against a parser that has never seen this module. Nothing in the
	// module itself needs it.
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3
	// h2c, used only by the tests: gRPC proper needs HTTP/2, and over plain TCP
	// that means h2c -- so this is what lets the claim "it really is gRPC on the
	// wire" be checked rather than asserted. A deployment with TLS needs none of
	// it, and nothing in the module imports it.
	golang.org/x/net v0.58.0
	google.golang.org/protobuf v1.36.12
	modernc.org/sqlite v1.58.0
)

require (
	filippo.io/edwards25519 v1.2.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/go-openapi/jsonpointer v0.22.5 // indirect
	github.com/go-openapi/swag/jsonname v0.25.5 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/oasdiff/yaml v0.1.1 // indirect
	github.com/oasdiff/yaml3 v0.0.14 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/rogpeppe/go-internal v1.16.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	modernc.org/libc v1.75.6 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
)

// effect-golang is not published yet, so it is resolved from the working copy
// next to this one. Replace this with a version requirement once it is tagged.
replace github.com/mbauer83/effect-golang => ../effect-golang
