package transcript_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/block"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/transcript"
)

func asideSnap(answer string, state session.State) session.Snapshot {
	return session.Apply(session.Snapshot{}, session.AsideUpdate{
		ID: "x1", Question: "side?", AnchorPreview: "answer earlier", Answer: answer, State: state,
	})
}

func requireAsideBlock(t *testing.T, w components.Widget) *block.AsideBlock {
	t.Helper()
	a, ok := w.(*block.AsideBlock)
	require.Truef(t, ok, "widget %T, want *block.AsideBlock", w)
	return a
}

// A side question draws as its own block, open by default, with the
// question, what it was about and the answer so far.
func TestMapperDrawsAnAsideOpen(t *testing.T) {
	th := components.VSLightTheme()
	m := transcript.NewMapper(th, nil, nil)
	entries, ids, _ := m.Sync(nil, nil, asideSnap("part", session.StateStreaming))
	require.Equal(t, []string{"x1"}, ids)
	a := requireAsideBlock(t, entries[0])
	assert.True(t, a.Expanded)
	assert.Equal(t, "side?", a.Question)
	assert.Equal(t, "answer earlier", a.AnchorPreview)
	assert.Equal(t, "part", a.Answer)
	assert.Equal(t, session.StateStreaming, a.State)
	assert.Equal(t, th.Aside, a.Theme.Aside, "the block paints with the theme it was handed")
}

// A folded aside stays folded as its answer keeps arriving and after it ends,
// and the same widget is patched all along.
func TestMapperKeepsAFoldedAsideFoldedThroughSync(t *testing.T) {
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	entries, ids, _ := m.Sync(nil, nil, asideSnap("part", session.StateStreaming))
	a := requireAsideBlock(t, entries[0])
	require.True(t, a.CollapseOnClick())

	entries, ids, dirty := m.Sync(entries, ids, asideSnap("part and more", session.StateStreaming))
	assert.Same(t, a, entries[0], "the row is patched, not rebuilt")
	assert.Equal(t, []int{0}, dirty)
	assert.False(t, a.Expanded)
	assert.Equal(t, "part and more", a.Answer)

	entries, _, _ = m.Sync(entries, ids, asideSnap("part and more", session.StateComplete))
	assert.False(t, requireAsideBlock(t, entries[0]).Expanded, "the end of the answer does not reopen it")

	// A fold made through the widget alone, with no toggle callback, is
	// still read back by the next full pass.
	fresh := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	entries, ids, _ = fresh.Sync(nil, nil, asideSnap("part", session.StateComplete))
	requireAsideBlock(t, entries[0]).Expanded = false
	entries, _, _ = fresh.Sync(entries, ids, asideSnap("part", session.StateComplete))
	assert.False(t, requireAsideBlock(t, entries[0]).Expanded)
}

// An aside asked after a finished turn does not pull that turn's answer into
// the fold of a condensed turn, and never folds away itself.
func TestACondensedTurnKeepsItsAnswerAndItsAside(t *testing.T) {
	m := transcript.NewMapper(components.DefaultTheme(), nil, nil)
	var snap session.Snapshot
	for _, turn := range []string{"t1", "t2", "t3", "t4"} {
		snap = session.Apply(snap, session.UserAppend{ID: "u-" + turn, Text: "prompt " + turn})
		snap = session.Apply(snap, session.AssistantMessageUpdate{Message: session.Message{
			ID: "c-" + turn, State: session.StateComplete,
			Content: []session.ContentBlock{{Type: session.BlockToolUse, ID: "tool-" + turn, Name: "read"}},
		}})
		snap = session.Apply(snap, session.ToolData{Run: session.ToolRun{
			ToolUseID: "tool-" + turn, Name: "read", Status: session.ToolDone,
		}})
		snap = session.Apply(snap, session.AssistantMessageUpdate{Message: session.Message{
			ID: "a-" + turn, State: session.StateComplete, Text: "answer " + turn,
			Content: []session.ContentBlock{{Type: session.BlockText, Text: "answer " + turn}},
		}})
		switch turn {
		case "t1":
			snap = session.Apply(snap, session.AsideUpdate{
				ID: "x1", Question: "side?", Answer: "yes", State: session.StateComplete,
			})
		case "t2":
			// A local command after the aside ends the turn with a row the
			// fold takes, and the aside must not go with it.
			snap = session.Apply(snap, session.AsideUpdate{
				ID: "x2", Question: "again?", Answer: "no", State: session.StateComplete,
			})
			snap = session.Apply(snap, session.LocalBashStart{ID: "b2", Command: "ls"})
			snap = session.Apply(snap, session.ToolData{Run: session.ToolRun{
				ToolUseID: "b2", Name: "bash", Status: session.ToolDone, Local: true,
			}})
		}
	}
	_, ids, _ := m.Sync(nil, nil, snap)
	require.Contains(t, ids, "turnsum-u-t1", "the first turn is condensed")
	require.Contains(t, ids, "turnsum-u-t2", "the second turn is condensed")
	assert.Contains(t, ids, "x1")
	assert.Contains(t, ids, "x2", "an aside inside the fold stays in view")
	answer := -1
	for i, id := range ids {
		if id == "x1" {
			require.GreaterOrEqual(t, answer, 0, "the answer of the turn stays out of the fold, above the aside")
		}
		if len(id) >= len("a-t1") && id[:len("a-t1")] == "a-t1" {
			answer = i
		}
	}
}
