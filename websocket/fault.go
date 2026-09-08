package websocket

// Fault is what this transport can fail with: a connection that could not be
// made, a message that could not be sent or read, or a value the schema
// refused.
//
// It is not an application failure. A conversation's own failures have the
// conversation's own type, and MapError adapts this into them.
type Fault struct {
	// Doing names what was being attempted, which is what a caller acts on.
	Doing string
	Err   error
}

func (fault Fault) Error() string {
	if fault.Err == nil {
		return "websocket: " + fault.Doing
	}
	return "websocket: " + fault.Doing + ": " + fault.Err.Error()
}

// Unwrap keeps errors.Is and errors.As working through the boundary.
func (fault Fault) Unwrap() error {
	return fault.Err
}

func faulted(doing string, err error) Fault {
	return Fault{Doing: doing, Err: err}
}
