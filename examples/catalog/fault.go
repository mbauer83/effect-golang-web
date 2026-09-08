package catalog

// Fault is what this program can fail with. It is one type rather than a
// structural union of an I/O failure and a schema failure because that is what
// an application boundary wants: a caller decides what to do about a stage, and
// the underlying error is still reachable with errors.As.
type Fault struct {
	// Stage names what the program was doing, which is what a caller acts on.
	Stage string
	Err   error
}

func (fault Fault) Error() string {
	return fault.Stage + ": " + fault.Err.Error()
}

// Unwrap keeps errors.Is and errors.As working through the boundary, so a
// caller can still ask whether a schema rejected a field.
func (fault Fault) Unwrap() error {
	return fault.Err
}
