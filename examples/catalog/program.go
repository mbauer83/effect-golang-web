package catalog

import (
	"context"
	"io/fs"

	"github.com/mbauer83/effect-golang-web/schema"
	"github.com/mbauer83/effect-golang-web/schema/jsonschema"
	"github.com/mbauer83/effect-golang/effect"
)

type catalogEffect[A any] = effect.Effect[effect.Unit, Fault, A]

// loading is the workflow's explicit state. Naming it keeps the sequence flat
// instead of nesting one FlatMap per stage.
type loading struct {
	document   []byte
	catalog    Catalog
	normalised []byte
	contract   published
}

// published is the pair a projection yields: the document to serve and the
// names of the shapes it declares once and refers to thereafter.
type published struct {
	document   []byte
	components []string
}

// Program loads a catalogue, writes it back normalised, and publishes the
// contract that describes what it accepts.
//
// The schema is validated first, so a declaration mistake fails the program at
// its start rather than on the first document that happens to reach it.
func Program(inputPath string, normalisedPath string, contractPath string) catalogEffect[Report] {
	io := effect.IOFor[effect.Unit]()
	return effect.Do[effect.Unit, Fault](func() loading { return loading{} }).
		Bind(ignoring(validated()), keepState).
		Bind(ignoring(read(io, inputPath)), func(state loading, document []byte) loading {
			state.document = document
			return state
		}).
		Bind(func(state loading) catalogEffect[Catalog] { return decoded(state.document) },
			func(state loading, catalog Catalog) loading {
				state.catalog = catalog
				return state
			}).
		Bind(func(state loading) catalogEffect[[]byte] { return normalised(state.catalog) },
			func(state loading, document []byte) loading {
				state.normalised = document
				return state
			}).
		Bind(func(state loading) catalogEffect[effect.Unit] {
			return write(io, normalisedPath, state.normalised)
		}, keepState).
		Bind(ignoring(contract()), func(state loading, contract published) loading {
			state.contract = contract
			return state
		}).
		Bind(func(state loading) catalogEffect[effect.Unit] {
			return write(io, contractPath, state.contract.document)
		}, keepState).
		Yield(report).
		Named("catalog")
}

// ignoring adapts a step that does not consult the state, and keepState adapts
// one whose result the state does not need. Together they keep every Bind in
// the sequence reading the same way.
func ignoring[A any](step catalogEffect[A]) func(loading) catalogEffect[A] {
	return func(loading) catalogEffect[A] { return step }
}

func keepState[A any](state loading, _ A) loading { return state }

// validated refuses to start on a schema that could never work.
func validated() catalogEffect[effect.Unit] {
	return effect.Try(
		func(context.Context, effect.Unit) (effect.Unit, error) {
			return effect.Unit{}, schema.Validate(Schema)
		},
		faulting[error]("validating the catalogue schema"),
	).Named("validate-schema")
}

func read(io effect.IOOperations[effect.Unit], path string) catalogEffect[[]byte] {
	return io.ReadFile(path).
		MapError(faulting[effect.IOError]("reading the catalogue")).
		Named("read-catalogue")
}

func write(io effect.IOOperations[effect.Unit], path string, document []byte) catalogEffect[effect.Unit] {
	return io.WriteFile(path, document, fs.FileMode(0o600)).
		MapError(faulting[effect.IOError]("writing " + path)).
		Named("write-document")
}

// decoded is where a document becomes a value. It is fallible and belongs in
// the same failure channel as the file it came from, which is why it is a stage
// rather than a call.
func decoded(document []byte) catalogEffect[Catalog] {
	return effect.Try(
		func(context.Context, effect.Unit) (Catalog, error) {
			return schema.DecodeJSON(Schema, document)
		},
		faulting[error]("decoding the catalogue"),
	).Named("decode-catalogue")
}

// normalised writes the decoded value back out. Encoding is deterministic, so
// this is the document a cache or a diff can rely on.
func normalised(catalog Catalog) catalogEffect[[]byte] {
	return effect.Try(
		func(context.Context, effect.Unit) ([]byte, error) {
			return schema.EncodeJSON(Schema, catalog)
		},
		faulting[error]("normalising the catalogue"),
	).Named("normalise-catalogue")
}

// contract projects the one description into the published one. No shape is
// declared twice: this is the same Schema the decoder used.
func contract() catalogEffect[published] {
	return effect.Try(
		func(context.Context, effect.Unit) (published, error) {
			projected := jsonschema.Project(Schema.Structure())
			document, err := projected.Render()
			if err != nil {
				return published{}, err
			}
			return published{document: document, components: projected.ComponentNames()}, nil
		},
		faulting[error]("publishing the contract"),
	).Named("publish-contract")
}

func report(state loading) Report {
	shelved := 0
	for _, book := range state.catalog.Books {
		if _, onTheShelf := book.Availability.(InStock); onTheShelf {
			shelved++
		}
	}
	return Report{
		Books:      len(state.catalog.Books),
		Shelved:    shelved,
		Components: state.contract.components,
	}
}

// faulting names the stage a failure happened in, so a caller reads one type
// and still reaches the underlying error with errors.As. It is instantiated
// explicitly because the two failure channels it adapts -- a filesystem error
// and a schema error -- are different types at the call site.
func faulting[E error](stage string) func(E) Fault {
	return func(err E) Fault { return Fault{Stage: stage, Err: err} }
}
