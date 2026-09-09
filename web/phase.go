package web

import (
	"context"

	"github.com/mbauer83/effect-golang/effect"
)

// The parts of a route's own work: how they are named, and how they are
// measured.

// phases are the names a route's own parts are spanned under, and the sampler
// they are measured with.
//
// Decoding and encoding are the route's work as much as the handler is, and a
// trace that showed one bar for all three could not say which of them a slow
// request spent its time in. A large document to unmarshal is real time, and
// so is a large one to write back.
//
// Empty names and no sampler mean no spans and no measurement, which is the
// default: a surface nobody is watching should not pay for three spans per
// request instead of none.
type phases struct {
	decoding string
	handling string
	encoding string
	sample   Sampling
}

// quiet is a surface that asked for neither names nor measurements.
func (named phases) quiet() bool {
	return named.sample == nil &&
		named.decoding == "" && named.handling == "" && named.encoding == ""
}

// Sampling measures one phase of a route. It is called when the phase begins,
// and the function it returns is called when the phase ends -- however it
// ended, including a failure or an interruption.
//
// A pair of callbacks rather than an effect wrapper, and the reason is Go: the
// three phases carry three different value types, so a single value that
// wrapped "an effect of any type" would need a method with type parameters of
// its own, which the language does not have.
//
// A pair of callbacks also keeps the direction right. Measuring the process is
// not this module's business -- it depends on the runtime and on schemas, not
// on anything that reads counters -- so the measuring belongs to whoever is
// watching, and this is the seam they reach through.
type Sampling func(phase string) func()

// The names a detailing surface spans its phases under.
//
// Exported because anything keyed by operation needs them: a metric
// vocabulary that does not declare them measures three spans per request as
// "an operation nobody declared", which is how the largest thing in an
// aggregate came to be a bucket with no name on it.
const (
	PhaseDecoding = "decoding"
	PhaseHandling = "handling"
	PhaseEncoding = "encoding"
)

// PhaseNames are the three, for a caller assembling a vocabulary.
//
//	metrics.Naming(append(inspect.Names(surface.Declarations()), web.PhaseNames()...)...)
func PhaseNames() []string {
	return []string{PhaseDecoding, PhaseHandling, PhaseEncoding}
}

// detailed is the naming a surface uses when it details its phases.
func detailed() phases {
	return phases{
		decoding: PhaseDecoding,
		handling: PhaseHandling,
		encoding: PhaseEncoding,
	}
}

// within names and measures one phase, and returns it untouched where neither
// was asked for.
//
// One shape for all of it, so the composition in route.go is written once: a
// surface that is not detailing pays for nothing, and no path is a second copy
// of another that could drift from it.
func within[R, E, A any](
	fx effect.Effect[R, E, A],
	name string,
	sample Sampling,
) effect.Effect[R, E, A] {
	return spanned(measuring(fx, name, sample), name)
}

// spanned names an effect when a name is given.
func spanned[R, E, A any](fx effect.Effect[R, E, A], name string) effect.Effect[R, E, A] {
	if name == "" {
		return fx
	}
	return fx.WithSpan(name)
}

// measuring hands the phase to the sampler around the effect's run.
//
// Suspended, so the sampler is called when the phase is interpreted and not
// when the route was described -- one description run twice is two phases. The
// second call is a finalizer, so a phase that failed or was interrupted is
// reported too: a decoding that allocated a great deal and then refused the
// request is exactly the one worth seeing.
func measuring[R, E, A any](
	fx effect.Effect[R, E, A],
	name string,
	sample Sampling,
) effect.Effect[R, E, A] {
	if sample == nil || name == "" {
		return fx
	}
	operations := effect.For[R, E]()
	return operations.Suspend(func() effect.Effect[R, E, A] {
		ended := sample(name)
		if ended == nil {
			return fx
		}
		return fx.Ensuring(effect.Release[R](func(context.Context) error {
			ended()
			return nil
		}))
	})
}
