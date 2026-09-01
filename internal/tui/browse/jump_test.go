package browse_test

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/tui/browse"
)

// jumpList is the searchable fixture: a header the cursor may not rest
// on, then rows a query can tell apart.
var jumpList = []struct {
	text       string
	selectable bool
}{
	{"Steps · ordered", false},
	{"wire the pane", true},
	{"run test-pane", true},
	{"write the docs", true},
	{"+ Add step", true},
}

func jumpRow(i int) (string, bool) { return jumpList[i].text, jumpList[i].selectable }

func openJump(t *testing.T, cur *browse.Cursor) *browse.Jump {
	t.Helper()
	cur.SetRows(len(jumpList), func(i int) bool { return jumpList[i].selectable })
	cur.SetViewport(10)
	var j browse.Jump
	j.Open(cur.Selected(), xui.Style{}, xui.Style{})
	require.True(t, j.Active())
	return &j
}

func jumpType(j *browse.Jump, cur *browse.Cursor, text string) browse.JumpResult {
	var ctx components.EventContext
	return j.Handle(&ctx, xui.PasteEvent{Text: text}, cur, len(jumpList), jumpRow)
}

func jumpKey(j *browse.Jump, cur *browse.Cursor, code xui.KeyCode) browse.JumpResult {
	var ctx components.EventContext
	return j.Handle(&ctx, key(code, 0, 0), cur, len(jumpList), jumpRow)
}

func TestJumpSelectsTheTightestMatchLive(t *testing.T) {
	var cur browse.Cursor
	j := openJump(t, &cur)

	assert.Equal(t, browse.JumpOpen, jumpType(j, &cur, "test"))
	assert.Equal(t, 2, cur.Selected(), "run test-pane holds the tightest subsequence")
	label, warn := j.Label()
	assert.Equal(t, " Jump · 1 match ", label)
	assert.False(t, warn)

	assert.Equal(t, browse.JumpKept, jumpKey(j, &cur, xui.KeyEnter))
	assert.False(t, j.Active())
	assert.Equal(t, 2, cur.Selected(), "Enter keeps what the jump found")
}

func TestJumpEscRestoresTheOrigin(t *testing.T) {
	var cur browse.Cursor
	j := openJump(t, &cur)
	origin := cur.Selected()

	jumpType(j, &cur, "docs")
	require.NotEqual(t, origin, cur.Selected())
	assert.Equal(t, browse.JumpBack, jumpKey(j, &cur, xui.KeyEscape))
	assert.False(t, j.Active())
	assert.Equal(t, origin, cur.Selected())
}

func TestJumpCyclesMatchesAndNamesAMiss(t *testing.T) {
	var cur browse.Cursor
	j := openJump(t, &cur)

	jumpType(j, &cur, "pane")
	first := cur.Selected()
	label, _ := j.Label()
	assert.Equal(t, " Jump · match 1/2 ", label)

	jumpKey(j, &cur, xui.KeyDown)
	assert.NotEqual(t, first, cur.Selected(), "↓ moves to the next match")
	jumpKey(j, &cur, xui.KeyUp)
	assert.Equal(t, first, cur.Selected(), "↑ comes back")

	for range 4 {
		jumpKey(j, &cur, xui.KeyBackspace)
	}
	jumpType(j, &cur, "zzqq")
	label, warn := j.Label()
	assert.Equal(t, " Jump · no match ", label)
	assert.True(t, warn, "a miss asks for the warning style")
	assert.Equal(t, first, cur.Selected(), "a miss leaves the selection alone")
}

func TestJumpSkipsUnselectableRowsAndClosesOnClick(t *testing.T) {
	var cur browse.Cursor
	j := openJump(t, &cur)

	jumpType(j, &cur, "ordered")
	label, warn := j.Label()
	assert.Equal(t, " Jump · no match ", label, "a header is not a jump target")
	assert.True(t, warn)

	j.Open(cur.Selected(), xui.Style{}, xui.Style{})
	jumpType(j, &cur, "wire")
	require.Equal(t, 1, cur.Selected())
	var ctx components.EventContext
	res := j.Handle(&ctx, xui.MouseEvent{}, &cur, len(jumpList), jumpRow)
	assert.Equal(t, browse.JumpClick, res, "a click closes the strip and stays with the pane")
	assert.False(t, j.Active())
	assert.Equal(t, 1, cur.Selected())
}
