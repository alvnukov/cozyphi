//go:build !darwin && !linux

package notify

import (
	"context"
	"errors"
)

// DefaultSound is empty here: there is no sender to play one.
const DefaultSound = ""

// platformSend always fails here; the notifier disables itself after the
// first send attempt instead of spawning doomed ones every turn.
func platformSend(context.Context, string, string, string) error {
	return errors.New("desktop notifications are not supported on this platform")
}
