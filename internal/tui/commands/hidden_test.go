package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components/palette"
)

// paletteHas reports whether an id is present; the shared findPaletteCommand
// fails the test on absence, which is exactly the case under test here.
func paletteHas(commands []palette.PaletteCommand, id string) bool {
	for _, command := range commands {
		if command.ID == id {
			return true
		}
	}
	return false
}

// TestSetHiddenWithdrawsAndRestoresCommands: a feature switched off at runtime
// keeps its registration but stops being listed, dispatched or offered in the
// palette; restoring it brings every surface back.
func TestSetHiddenWithdrawsAndRestoresCommands(t *testing.T) {
	r := NewBuiltinRegistry()
	host := &fakeHost{}
	ctx := CommandContext{Host: host}

	require.True(t, r.DispatchSlash("/plan", ctx), "/plan dispatches while offered")
	require.NotEmpty(t, r.FilterSlash("plan"))
	require.True(t, paletteHas(r.BuildPalette(ctx), "plan-editor"))
	require.Equal(t, 1, host.planOpens)

	r.SetHidden("plan", true)
	r.SetHidden("plan-editor", true)

	assert.Empty(t, r.FilterSlash("plan"), "hidden commands leave the slash picker")
	assert.Empty(t, r.LookupInsert("plan"), "hidden commands lose their insert")
	assert.False(t, r.DispatchSlash("/plan", ctx), "hidden commands refuse dispatch")
	assert.False(t, paletteHas(r.BuildPalette(ctx), "plan-editor"), "hidden commands leave the palette")
	assert.Equal(t, 1, host.planOpens, "a hidden command's Run never fires")

	r.SetHidden("plan", false)
	r.SetHidden("plan-editor", false)
	require.True(t, r.DispatchSlash("/plan", ctx))
	assert.Equal(t, 2, host.planOpens, "restoring reopens dispatch")
	assert.True(t, paletteHas(r.BuildPalette(ctx), "plan-editor"))
}
