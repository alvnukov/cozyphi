package job

// StoppedError is an assignment stop with a durable reason. It differs from a
// turn interruption, which must not return from an interactive Runner at all.
type StoppedError struct{ Reason string }

func (e *StoppedError) Error() string { return "assignment stopped: " + e.Reason }
