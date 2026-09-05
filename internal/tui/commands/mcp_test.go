package commands

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components/palette"
	"github.com/alvnukov/cozyphi/internal/components/toast"
	"github.com/alvnukov/cozyphi/internal/mcp"
)

// slashMCP pushes the dialog through the registry, the way the composer does.
func slashMCP(t *testing.T, host *fakeHost) {
	t.Helper()
	reg := NewBuiltinRegistry()
	require.True(t, reg.DispatchSlash("/mcp", CommandContext{Host: host}))
}

func TestMCPCommandDialogTogglesAndRefreshes(t *testing.T) {
	host := &fakeHost{mcpStatuses: []mcp.ServerStatus{
		{Name: "alpha", State: mcp.StateConnected},
		{Name: "beta", State: mcp.StateDisabled},
	}}
	slashMCP(t, host)

	require.True(t, host.pushed)
	require.Equal(t, mcpDialogTitle, host.pushTitle)
	require.Len(t, host.pushCmds, 2)
	assert.Contains(t, host.pushCmds[0].Verb, "alpha")
	assert.Contains(t, host.pushCmds[0].Verb, "disable", "an on server offers disable")
	assert.Contains(t, host.pushCmds[1].Verb, "beta")
	assert.Contains(t, host.pushCmds[1].Verb, "enable", "an off server offers enable")

	// Accepting the alpha row disables it and pushes a page with fresh state.
	host.pushed = false
	host.pushCmds[0].Run()
	require.Equal(t, []mcpToggleCall{{Name: "alpha", Enabled: false}}, host.mcpToggles)
	require.True(t, host.pushed, "successful toggle reopens the dialog")
	assert.Len(t, host.pushCmds, 2)

	// The refreshed page reflects the toggle: alpha now offers enable, and
	// accepting it toggles back on.
	host.pushed = false
	host.pushCmds[0].Run()
	require.Equal(t, []mcpToggleCall{
		{Name: "alpha", Enabled: false},
		{Name: "alpha", Enabled: true},
	}, host.mcpToggles, "a disabled server toggles back on")
	assert.Len(t, host.pushCmds, 2)
}

func TestMCPCommandFailedToggleStaysQuiet(t *testing.T) {
	host := &fakeHost{
		mcpStatuses:  []mcp.ServerStatus{{Name: "alpha", State: mcp.StateConnected}},
		mcpToggleErr: errors.New("boom"),
	}
	slashMCP(t, host)

	host.pushed = false
	host.pushCmds[0].Run()

	require.Len(t, host.mcpToggles, 1, "the toggle was attempted")
	assert.False(t, host.pushed, "a failed toggle does not reopen the dialog")
	assert.Equal(t, "mcp: boom", host.toastMsg)
	assert.Equal(t, toast.ToastError, host.toastKind)
}

func TestMCPCommandEmptyState(t *testing.T) {
	host := &fakeHost{}
	slashMCP(t, host)

	require.Len(t, host.pushCmds, 1)
	assert.True(t, host.pushCmds[0].Disabled)
	assert.Contains(t, host.pushCmds[0].Verb, "No MCP servers configured")
}

func TestMCPCommandPaletteRoot(t *testing.T) {
	host := &fakeHost{mcpStatuses: []mcp.ServerStatus{
		{Name: "alpha", State: mcp.StateConnected},
	}}
	reg := NewBuiltinRegistry()
	rows := reg.BuildPalette(CommandContext{Host: host})

	var page palette.PaletteCommand
	for _, row := range rows {
		if row.ID == "mcp" {
			page = row
		}
	}
	require.NotNil(t, page.Submenu, "palette root opens the same dialog as /mcp")
	assert.Equal(t, mcpDialogTitle, page.SubmenuTitle)
	require.Len(t, page.Submenu, 1)
	assert.Contains(t, page.Submenu[0].Verb, "alpha")
}
