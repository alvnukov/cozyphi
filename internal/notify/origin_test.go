package notify_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/notify"
)

func TestOriginPreservesNotificationModeAndFocus(t *testing.T) {
	for _, tc := range []struct {
		name          string
		mode          notify.Mode
		focused, want bool
	}{
		{"off", notify.ModeOff, false, false},
		{"always", notify.ModeAlways, true, true},
		{"unfocused", notify.ModeUnfocused, false, true},
		{"focused", notify.ModeUnfocused, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sent := make(chan string, 1)
			n := notify.New(
				tc.mode,
				notify.WithSender(
					func(_ context.Context, title, body string) error { sent <- title + "|" + body; return nil },
				),
			)
			n.SetFocused(false)
			n.SetFocused(tc.focused)
			n.SetOrigin("#2 review")
			n.NeedsAttention("bash")
			if tc.want {
				select {
				case msg := <-sent:
					require.Equal(t, "cozyphi · #2 review|bash", msg)
				case <-time.After(time.Second):
					t.Fatal("notification missing")
				}
			} else {
				select {
				case msg := <-sent:
					t.Fatalf("unexpected notification: %s", msg)
				case <-time.After(30 * time.Millisecond):
				}
			}
		})
	}
}
