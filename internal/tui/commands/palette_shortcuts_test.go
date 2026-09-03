package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components/palette"
	"github.com/alvnukov/cozyphi/internal/tui/keys"
)

// Every palette shortcut must be a chord the keys catalog documents: the
// palette teaches the fast path, so a spelling the help screen does not
// know is a lie in one place or the other.
func TestPaletteShortcutsComeFromTheKeysCatalog(t *testing.T) {
	labels := map[string]bool{}
	for _, g := range keys.Groups() {
		for _, b := range g.Bindings {
			for _, k := range b.Keys {
				labels[k] = true
			}
		}
	}

	cmds := NewBuiltinRegistry().BuildPalette(CommandContext{})
	shortcuts := map[string]string{}
	for _, c := range cmds {
		if c.Shortcut != "" {
			assert.True(t, labels[c.Shortcut],
				"%s: shortcut %q is not in the keys catalog", c.ID, c.Shortcut)
		}
		shortcuts[c.ID] = c.Shortcut
	}

	for id, chord := range map[string]string{
		"help":                keys.Label(keys.CmdHelp),
		"harness-settings":    keys.Label(keys.CmdSettings),
		"plan-editor":         keys.Label(keys.CmdPlanEditor),
		"clipboard-copy-last": keys.Label(keys.CmdCopyLast),
	} {
		assert.Equal(t, chord, shortcuts[id], "command %q must name its chord", id)
	}
}

func runByID(t *testing.T, cmds []palette.PaletteCommand, id string) {
	t.Helper()
	for _, c := range cmds {
		if c.ID == id {
			require.NotNil(t, c.Run, "command %q has no Run", id)
			c.Run()
			return
		}
	}
	t.Fatalf("command %q is not in the palette", id)
}

// The slash-only commands ride the palette too: Ctrl+K is the one place a
// user can search every command, so /help, /context, /connect, /compact
// and /sessions must be reachable there.
func TestPaletteCarriesTheSlashOnlyCommands(t *testing.T) {
	host := &fakeHost{}
	cmds := NewBuiltinRegistry().BuildPalette(CommandContext{Host: host})

	runByID(t, cmds, "help")
	assert.Equal(t, 1, host.helpOpens)
	runByID(t, cmds, "context")
	assert.Equal(t, 1, host.contexts)
	runByID(t, cmds, "connect")
	assert.Equal(t, 1, host.connected)
	runByID(t, cmds, "compact")
	assert.Equal(t, 1, host.compacted)
	runByID(t, cmds, "sessions")
	assert.Equal(t, 1, host.sessions)
}
