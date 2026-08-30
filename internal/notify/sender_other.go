//go:build !darwin && !linux

package notify

import (
	"context"
	"errors"
)

// platformSender always fails here; the notifier disables itself after the
// first send attempt instead of spawning doomed ones every turn.
func platformSender() sendFunc {
	return func(context.Context, string, string) error {
		return errors.New("desktop notifications are not supported on this platform")
	}
}
