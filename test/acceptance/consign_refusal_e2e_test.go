package acceptance

// What the consign program does with a shipment no carrier will take.
//
// Three finalities, each producing the disposition that says it. This is what
// the package exists for: AMQP 0-9-1 would have put all three back the same
// way, saying nothing.

import (
	"testing"

	"github.com/mbauer83/effect-golang-schema/schema/dynamic"
	"github.com/mbauer83/effect-golang-web/amqp10"
	"github.com/mbauer83/effect-golang-web/examples/consign"
	"github.com/mbauer83/effect-golang/effect"
)

// refusing is a carrier that refuses everything, the same way each time.
func refusing(how consign.Finality, why string) func(consign.Shipment) consigning[consign.Outcome] {
	return func(consign.Shipment) consigning[consign.Outcome] {
		return effect.For[effect.Unit, amqp10.Fault]().
			Succeed[consign.Outcome](consign.Refused{Finality: how, Reason: why})
	}
}

func TestARefusalThatWillNeverSucceedIsRejectedRatherThanGivenBack(t *testing.T) {
	// Never means retrying is pointless, so the message does not come back and
	// the node empties -- which is why this stream ends on its own where the
	// other two would not.
	broker, exit := consigned(t, func(
		sender amqp10.Sending,
		receiver amqp10.Receiving,
	) consigning[[]consign.Shipment] {
		return consign.Hand(sender, consignment).
			FlatMap(func(effect.Unit) consigning[[]consign.Shipment] {
				return effect.RunCollect(
					consign.Collect(receiver, refusing(consign.Never, "no room")))
			})
	})

	collected, ok := exit.Value()
	if !ok {
		t.Fatalf("unexpected exit: %+v", exit)
	}
	if len(collected) != 0 {
		t.Fatalf("expected nothing collected, got %#v", collected)
	}
	rejected := broker.Rejected()
	if len(rejected) != 1 || rejected[0].Reason != "no room" {
		t.Fatalf("expected one rejection carrying the reason, got %v", rejected)
	}
	if modified := broker.Modified(); len(modified) != 0 {
		t.Fatalf("expected nothing given back, got %v", modified)
	}
}

func TestARefusalThatMightSucceedLaterIsGivenBackSayingWhy(t *testing.T) {
	// NotNow and NotMe both put the message back, so a carrier that always
	// refused would loop -- which is what the disposition means and what the
	// reference says. Each carrier here refuses once and then takes it, so the
	// stream ends and the modification is there to read.
	for named, expected := range map[string]struct {
		how       consign.Finality
		tried     bool
		elsewhere bool
	}{
		"not me":  {how: consign.NotMe, elsewhere: true},
		"not now": {how: consign.NotNow, tried: true},
	} {
		broker, exit := consigned(t, func(
			sender amqp10.Sending,
			receiver amqp10.Receiving,
		) consigning[[]consign.Shipment] {
			return consign.Hand(sender, consignment).
				FlatMap(func(effect.Unit) consigning[[]consign.Shipment] {
					return effect.RunCollect(
						consign.Collect(receiver, refusingOnce(expected.how, "no room")).
							TakeStream(1))
				})
		})

		collected, ok := exit.Value()
		if !ok {
			t.Errorf("%s: unexpected exit: %+v", named, exit)
			continue
		}
		// Taken on the second offer, which is the witness that it came back.
		if len(collected) != 1 || collected[0] != consignment {
			t.Errorf("%s: expected the shipment collected in the end, got %#v", named, collected)
		}
		modified := broker.Modified()
		if len(modified) != 1 {
			t.Errorf("%s: expected one modification, got %d", named, len(modified))
			continue
		}
		if modified[0].Change.Tried != expected.tried ||
			modified[0].Change.Elsewhere != expected.elsewhere {
			t.Errorf("%s: unexpected change: %#v", named, modified[0].Change)
		}
		// And what the receiver recorded, which is the other half of what
		// modifying says: whoever gets it next knows why it came back.
		held, present := modified[0].Change.Annotations.Member("refused-because")
		if !present {
			t.Errorf("%s: expected the reason recorded, got %#v", named, modified[0].Change)
		} else if held != dynamic.OfText("no room") {
			t.Errorf("%s: unexpected reason: %#v", named, held)
		}
		// Nothing was released: a release says the message is back and nothing
		// else, which is the disposition this program never wants.
		if released := broker.Released(); len(released) != 0 {
			t.Errorf("%s: expected nothing released, got %v", named, released)
		}
		if accepted := broker.Accepted(); len(accepted) != 1 {
			t.Errorf("%s: expected one acceptance, got %v", named, accepted)
		}
	}
}

// refusingOnce refuses the first shipment it is offered and takes the rest, so
// a stream over a disposition that puts the message back still ends.
func refusingOnce(
	how consign.Finality,
	why string,
) func(consign.Shipment) consigning[consign.Outcome] {
	offered := 0
	return func(consign.Shipment) consigning[consign.Outcome] {
		return effect.For[effect.Unit, amqp10.Fault]().
			Suspend(func() consigning[consign.Outcome] {
				offered++
				if offered > 1 {
					return effect.For[effect.Unit, amqp10.Fault]().
						Succeed[consign.Outcome](consign.Collected{})
				}
				return effect.For[effect.Unit, amqp10.Fault]().
					Succeed[consign.Outcome](consign.Refused{Finality: how, Reason: why})
			})
	}
}
