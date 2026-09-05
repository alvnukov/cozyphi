package editor

import (
	"context"
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/harnesssettings"
	"github.com/alvnukov/cozyphi/internal/tui/statuspane"
)

type statusReadOnlyStore struct {
	snapshot      harnesssettings.Snapshot
	reads, writes int
}

func (s *statusReadOnlyStore) Snapshot() harnesssettings.Snapshot {
	s.reads++
	return s.snapshot
}

func (s *statusReadOnlyStore) Apply(context.Context, harnesssettings.Draft) (harnesssettings.Snapshot, error) {
	s.writes++
	return s.snapshot, nil
}

func TestStatusSettingsSnapshotAllowlistAndNoPersistence(t *testing.T) {
	e := newEditorWithSettings(t)
	store := &statusReadOnlyStore{snapshot: harnesssettings.Snapshot{
		Token:           "SECRET-TOKEN",
		Path:            "/safe/settings.yaml",
		OpenCodeEnabled: true,
		AgentModels: map[string]string{
			"explore": "pinned-model",
			"worker":  "https://secret.example/token",
			"review":  "${SECRET_ENV}",
		},
		Notifications: harnesssettings.Notifications{Sound: "SECRET-SOUND"},
		Compaction:    harnesssettings.Compaction{ReminderTokens: 12345},
	}}
	e.statusStore = store
	e.ConfigureStatusTabs(func() string { return statuspane.Config }, nil)
	e.ShowStatus()
	require.Equal(t, 1, store.reads)
	store.snapshot.AgentModels["explore"] = "changed-after-open"
	for range 2 {
		s := e.status.Draw(
			components.DrawContext{Max: components.Size{Width: 120, Height: 40}, Method: xui.WidthUnicode},
		)
		text := components.SurfaceText(s)
		assert.Contains(t, text, "pinned-model")
		assert.Contains(t, text, "12345 tokens")
		assert.Contains(t, text, "Effective task access:")
		assert.Contains(t, text, "Configured agent model pins · effective unavailable")
		assert.Contains(t, text, "Winning source: unavailable")
		assert.Contains(t, strings.Join(statusSettingsRows(harnesssettings.Snapshot{}), "\n"),
			"Compaction reminder: effective unavailable; configured automatic default")
		assert.Contains(t, text, "/safe/settings.yaml")
		for _, secret := range []string{"SECRET", "secret.example", "changed-after-open"} {
			assert.NotContains(t, text, secret)
		}
		e.Handle(&components.EventContext{}, xui.PasteEvent{Text: "paste"})
		e.Handle(&components.EventContext{}, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 's', Mods: xui.ModCtrl})
	}
	e.status.Hide()
	assert.Equal(t, 1, store.reads, "Draw and close do not read the store")
	assert.Zero(t, store.writes)
	assert.False(t, e.settings.Visible())
	e.ShowStatus()
	assert.Equal(t, 2, store.reads, "reopening takes a fresh snapshot")
	assert.Contains(t, strings.Join(statusSettingsRows(store.snapshot), "\n"), "changed-after-open")
}

func TestStatusModelPinControlsAreSanitized(t *testing.T) {
	assert.Equal(t, "safe name [31m", statusModelPin("safe\nname\x1b[31m"))
	assert.Equal(t, "safe name", statusModelPin("safe\u202ename"))
}
