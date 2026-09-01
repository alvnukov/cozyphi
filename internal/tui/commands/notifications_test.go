package commands

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components/palette"
	"github.com/alvnukov/cozyphi/internal/components/toast"
)

func TestNotificationsCommandPushesTheHistory(t *testing.T) {
	rows := []palette.PaletteCommand{{ID: "toast-0", Verb: "✕ 12:00:00  disk full", Disabled: true}}
	host := &fakeHost{listToasts: rows}
	cmds := NewBuiltinRegistry().BuildPalette(CommandContext{Host: host})

	runByID(t, cmds, "toasts")
	require.True(t, host.pushed)
	assert.Equal(t, "Recent notifications", host.pushTitle)
	assert.Equal(t, rows, host.pushCmds)
}

func TestNotificationsCommandNamesAnEmptyHistory(t *testing.T) {
	host := &fakeHost{}
	cmds := NewBuiltinRegistry().BuildPalette(CommandContext{Host: host})

	runByID(t, cmds, "toasts")
	require.True(t, host.pushed)
	require.Len(t, host.pushCmds, 1)
	assert.Equal(t, "No notifications yet", host.pushCmds[0].Verb)
	assert.True(t, host.pushCmds[0].Disabled)
}

func TestToastListEntriesRenderTheKindAndTime(t *testing.T) {
	at := time.Date(2026, 9, 1, 12, 30, 5, 0, time.UTC)
	rows := ToastListEntries([]toast.Entry{
		{Message: "disk full", Kind: toast.ToastError, At: at},
		{Message: "copied", Kind: toast.ToastSuccess, At: at},
	})
	require.Len(t, rows, 2)
	assert.Equal(t, "✕ 12:30:05  disk full", rows[0].Verb)
	assert.Equal(t, "✓ 12:30:05  copied", rows[1].Verb)
	for _, row := range rows {
		assert.True(t, row.Disabled, "history rows are read-only")
	}
}
