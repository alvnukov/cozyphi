package tooldef

// CodeControl identifies a cell entry point. The cell transport, fixture-only
// tester and session-local wait handle have no ambient host authority.
func CodeControl(name string) bool {
	switch name {
	case "code", "code_wait", "code_test":
		return true
	default:
		return false
	}
}
