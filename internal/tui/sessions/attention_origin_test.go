package sessions

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The stop notification speaks about the work, not the slot: an explicit
// session title (the model's set_title or the user's /rename) becomes the
// notifier origin, while the input line keeps the stable registry label.
func TestNotifierOriginUsesSessionTitle(t *testing.T) {
	e, n := newNotifyTestEditor(t)

	require.NoError(t, e.ctrl.SetSessionTitle("Имя вкладки"))
	e.SetIdentity(1, "main")

	require.NotEmpty(t, n.origins)
	assert.Equal(t, "#1 Имя вкладки", n.origins[len(n.origins)-1])
	assert.Equal(t, "#1 main", e.composer.Chat.SessionLabel,
		"the input line keeps the stable slot label")
}

// Without an explicit title the origin stays the stable slot label — the
// first-prompt and short-ID display fallbacks do not leak into notifications.
func TestNotifierOriginFallsBackToSlotName(t *testing.T) {
	e, n := newNotifyTestEditor(t)

	e.SetIdentity(2, "review")

	require.NotEmpty(t, n.origins)
	assert.Equal(t, "#2 review", n.origins[len(n.origins)-1])
}
