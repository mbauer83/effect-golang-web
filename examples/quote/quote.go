// Package quote prices a shipment, over gRPC.
//
// One description does three jobs here: it decodes the request, it encodes the
// response, and it becomes the .proto file another language generates its
// client from. So a field number that changed would change that client's wire
// format, which is why the numbers are declared and not derived.
//
// The service depends on the port and never on Connect. Nothing in this package
// imports it.
package quote

import (
	"errors"

	"github.com/mbauer83/effect-golang-schema/schema"
	"github.com/mbauer83/effect-golang-web/grpc"
	"github.com/mbauer83/effect-golang/effect"
)

// Enquiry is what a caller asks about.
type Enquiry struct {
	Origin      string
	Destination string
	Kilos       float64
}

// Rate is the answer.
type Rate struct {
	Carrier  string
	Currency string
	Cents    int64
}

// EnquirySchema describes an enquiry.
var EnquirySchema = schema.Struct[Enquiry]("Enquiry",
	schema.FieldAt("origin", schema.Text().Check(schema.MinLength(1)),
		func(enquiry *Enquiry) *string { return &enquiry.Origin }).
		WithNumber(1).
		WithDescription("Origin is where the shipment starts."),
	schema.FieldAt("destination", schema.Text().Check(schema.MinLength(1)),
		func(enquiry *Enquiry) *string { return &enquiry.Destination }).
		WithNumber(2).
		WithDescription("Destination is where it is going."),
	schema.FieldAt("kilos", schema.Float64().Check(schema.Above[float64](0)),
		func(enquiry *Enquiry) *float64 { return &enquiry.Kilos }).
		WithNumber(3).
		WithDescription("Kilos is what it weighs, and it weighs something."),
).WithDescription("Enquiry asks what a shipment would cost.")

// RateSchema describes a rate.
var RateSchema = schema.Struct[Rate]("Rate",
	schema.FieldAt("carrier", schema.Text().Check(schema.MinLength(1)),
		func(rate *Rate) *string { return &rate.Carrier }).WithNumber(1),
	schema.FieldAt("currency", schema.Text().Check(schema.Pattern(`^[A-Z]{3}$`)),
		func(rate *Rate) *string { return &rate.Currency }).WithNumber(2),
	schema.FieldAt("cents", schema.Int64().Check(schema.AtLeast[int64](1)),
		func(rate *Rate) *int64 { return &rate.Cents }).WithNumber(3),
).WithDescription("Rate is what a carrier would charge.")

// Service is the fully-qualified proto service name, which is what forms the
// path a gRPC client calls.
const Service = "logistics.v1.Rates"

// Quote is the procedure.
var Quote = grpc.Unary(Service, "Quote", EnquirySchema, RateSchema).
	WithDescription("Quote prices one shipment, or says why it cannot be priced.")

// Refusal is why this service would not answer.
//
// The application's own failures, in its own words. What turns them into gRPC
// codes is one function, in one place -- so a handler never mentions a code and
// the mapping can be read on its own.
type Refusal struct {
	Reason  Reason
	Details string
}

// Reason is the closed set of things that go wrong here.
type Reason uint8

const (
	// NoRoute is nobody carrying between those two places.
	NoRoute Reason = iota
	// TooHeavy is a shipment above what any carrier will take.
	TooHeavy
	// NoCapacity is every carrier being full today, which is worth retrying.
	NoCapacity
)

func (refusal Refusal) Error() string {
	return "quote: " + refusal.Details
}

// FailureFor is what each refusal answers with.
//
// NoRoute is NotFound because the route does not exist; TooHeavy is
// FailedPrecondition because the system could not do it in any state a retry
// would reach; NoCapacity is Unavailable because tomorrow it might.
func FailureFor(refusal Refusal) grpc.Failure {
	switch refusal.Reason {
	case TooHeavy:
		return grpc.Failure{Code: grpc.FailedPrecondition, Message: refusal.Details}
	case NoCapacity:
		return grpc.Failure{Code: grpc.Unavailable, Message: refusal.Details}
	default:
		return grpc.Failure{Code: grpc.NotFound, Message: refusal.Details}
	}
}

type quoteEffect[A any] = effect.Effect[effect.Unit, Refusal, A]

// maxKilos is what any carrier here will take.
const maxKilos = 24000.0

// Price answers an enquiry from a table of routes.
//
// A function of the rates rather than a method on a service object, because
// what it needs is the table and nothing else -- and a handler that takes what
// it needs is a handler a test can call.
func Price(rates map[string]Rate) func(Enquiry) quoteEffect[Rate] {
	return func(enquiry Enquiry) quoteEffect[Rate] {
		return effect.For[effect.Unit, Refusal]().
			Suspend(func() quoteEffect[Rate] {
				if enquiry.Kilos > maxKilos {
					return quoteRefusal(TooHeavy, "no carrier takes more than 24 tonnes")
				}
				rate, carried := rates[enquiry.Origin+"-"+enquiry.Destination]
				if !carried {
					return quoteRefusal(NoRoute,
						"nobody carries "+enquiry.Origin+" to "+enquiry.Destination)
				}
				// Priced by weight, rounded up to the cent.
				rate.Cents += int64(enquiry.Kilos * 10)
				return effect.For[effect.Unit, Refusal]().Succeed(rate)
			})
	}
}

func quoteRefusal(reason Reason, details string) quoteEffect[Rate] {
	return effect.For[effect.Unit, Refusal]().
		Fail[Rate](Refusal{Reason: reason, Details: details})
}

// Answer wires the procedure to a handler at a boundary.
func Answer(
	boundary *grpc.Boundary[effect.Unit, Refusal],
	rates map[string]Rate,
) error {
	if boundary == nil {
		return errors.New("quote: a service needs a boundary")
	}
	return grpc.Answer(boundary, Quote, Price(rates))
}
