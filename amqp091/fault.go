package amqp091

import "errors"

// Fault is what went wrong, and where.
type Fault struct {
	// Doing names the stage, so a report says which of the several things a
	// message passes through refused it.
	Doing string
	// Queue or Target is the subject, whichever the stage had one of. A broker
	// error without the queue or exchange it was about is nearly useless, and
	// both are the program's own text rather than a user's.
	Subject string
	Err     error
}

func (fault Fault) Error() string {
	rendered := "amqp: " + fault.Doing
	if fault.Subject != "" {
		rendered += " [" + fault.Subject + "]"
	}
	if fault.Err != nil {
		rendered += ": " + fault.Err.Error()
	}
	return rendered
}

// Unwrap keeps errors.Is and errors.As working through the boundary, so a
// caller can still ask the broker's error whether the channel was closed.
func (fault Fault) Unwrap() error {
	return fault.Err
}

func faulted(doing string, subject string, err error) Fault {
	return Fault{Doing: doing, Subject: subject, Err: err}
}

var (
	errNotAnObject  = errors.New("a header table is a set of named values, and this is something else")
	errConsumerGone = errors.New("the consumer ended without saying why")
)
