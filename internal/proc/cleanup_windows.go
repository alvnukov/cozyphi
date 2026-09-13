//go:build windows

package proc

// Windows has no process-group containment after the leader exits. Existing
// taskkill cancellation remains best effort; a Job Object would be needed here.
func cleanupProcessGroup(int) error { return nil }
