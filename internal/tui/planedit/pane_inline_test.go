package planedit_test

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPaneInlineEditorKeepsTheListVisible(t *testing.T) {
	pane := newPane(&fakeStore{snapshot: fixturePlan()})
	require.True(t, key(pane, xui.KeyEnter, 0, 0)) // Goal is initially selected.

	screen := renderText(t, pane, 84, 30)
	assert.Contains(t, screen, "Edit goal · 20/512", "the rule row names the field and its budget")
	assert.Contains(t, screen, "├", "the editor is a strip inside the panel, not a popup over it")
	assert.Contains(t, screen, "+ Add step", "the list stays visible above the editor")
	assert.Equal(t, 1, strings.Count(screen, "›"), "the edited row keeps a passive marker")

	require.True(t, key(pane, xui.KeyEscape, 0, 0))
	screen = renderText(t, pane, 84, 30)
	assert.NotContains(t, screen, "Edit goal", "Esc closes the editor without saving")
	assert.NotContains(t, screen, "├")
}

func TestPaneInlineEditorGrowsToItsCapAndFollowsTheCursor(t *testing.T) {
	pane := newPane(&fakeStore{snapshot: fixturePlan()})
	down(t, pane, 2) // Context.
	require.True(t, key(pane, xui.KeyEnter, 0, 0))
	paste(pane, "\nalpha0\nalpha1\nalpha2\nalpha3\nalpha4\nalpha5\nalpha6")

	screen := renderText(t, pane, 84, 30)
	assert.Contains(t, screen, "alpha6", "the cursor line is always visible")
	assert.NotContains(t, screen, "alpha0",
		"the editor caps at six lines and scrolls, instead of eating the list")
}

func TestPaneInlineEditorSpansBothPanesOnWideScreens(t *testing.T) {
	pane := newPane(&fakeStore{snapshot: fixturePlan()})
	require.True(t, key(pane, xui.KeyEnter, 0, 0)) // Goal.

	screen := renderText(t, pane, 100, 30)
	assert.Contains(t, screen, "Goal · 20/512", "the preview column stays up while editing")
	assert.Contains(t, screen, "Edit goal · 20/512", "the editor strip runs along the bottom")
}
