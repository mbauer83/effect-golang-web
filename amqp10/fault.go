package amqp10

import "errors"

// Fault is what went wrong, and where.
type Fault struct {
	// Doing names the stage, so a report says which of the several things a
	// message passes through refused it.
	Doing string
	// Address is the node the link was attached to, where the stage had one. A
	// broker error without it is nearly useless, and it is the program's own
	// text rather than a user's.
	Address string
	Err     error
}

func (fault Fault) Error() string {
	rendered := "amqp10: " + fault.Doing
	if fault.Address != "" {
		rendered += " [" + fault.Address + "]"
	}
	if fault.Err != nil {
		rendered += ": " + fault.Err.Error()
	}
	return rendered
}

// Unwrap keeps errors.Is and errors.As working through the boundary, so a
// caller can still ask the broker's error whether the link was detached.
func (fault Fault) Unwrap() error {
	return fault.Err
}

func faulted(doing string, address string, err error) Fault {
	return Fault{Doing: doing, Address: address, Err: err}
}

var (
	errNoTag      = errors.New("the broker sent a message with no delivery tag, so it cannot be settled")
	errUnknownTag = errors.New("no unsettled delivery has that tag")
)
