//go:build !darwin && !linux

package notify

import (
	"context"
	"errors"
)

// DefaultSound is empty here: there is no sender to play one.
const DefaultSound = ""

// platformSender always fails here; the notifier disables itself after the
// first send attempt instead of spawning doomed ones every turn.
func platformSender(string) sendFunc {
	return func(context.Context, string, string) error {
		return errors.New("desktop notifications are not supported on this platform")
	}
}
