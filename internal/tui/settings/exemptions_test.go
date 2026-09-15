package settings_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/tui/settings"
)

// allRows scrolls the plan tab from the cursor to the end and returns every
// row it rendered on the way. The panel clamps its body to a couple of dozen
// lines whatever height it is drawn at, so a single Draw shows a window, not
// the list — an assertion about "no such row exists" has to walk it.
func allRows(t *testing.T, pane *settings.Pane) []string {
	t.Helper()
	var (
		rows  []string
		seen  = map[string]struct{}{}
		quiet int
	)
	for range 400 {
		fresh := false
		for line := range strings.SplitSeq(drawText(pane), "\n") {
			line = strings.TrimRight(strings.TrimPrefix(strings.TrimSpace(line), "› "), " ")
			if line == "" {
				continue
			}
			if _, ok := seen[line]; ok {
				continue
			}
			seen[line] = struct{}{}
			rows = append(rows, line)
			fresh = true
		}
		if fresh {
			quiet = 0
		} else {
			quiet++
			if quiet > 2 {
				break
			}
		}
		require.True(t, key(pane, xui.KeyDown, 0, 0))
	}
	return rows
}

// Every per-type permission row is a tool the draft may assign to that step
// type. An exemption-only tool has no capability rank, so the row was a trap:
// ticking it moved the name out of the exemptions and into a step type, and
// the next Ctrl+S failed to compile — the pane looked like it had forgotten
// the checkbox when it had actually wedged the draft.
func TestPlanTabOffersNoStepRowForExemptionOnlyTools(t *testing.T) {
	pane := settings.New(components.DefaultTheme(), fixtureStore(), nil)
	pane.Show()
	rows := allRows(t, pane)
	require.NotEmpty(t, rows)

	var offered, exemptionOnly int
	for _, tool := range plangate.KnownTools() {
		perType := fmt.Sprintf("] %s · for ", tool.Name)
		outside := fmt.Sprintf("] %s · allowed outside plan", tool.Name)
		hasPerType := slices.ContainsFunc(rows, func(row string) bool { return strings.Contains(row, perType) })
		hasOutside := slices.ContainsFunc(rows, func(row string) bool { return strings.Contains(row, outside) })

		switch {
		case tool.MandatoryExemption:
			assert.False(t, hasPerType, "%s is a mandatory exemption, not a step-type tool", tool.Name)
			assert.False(t, hasOutside, "%s is listed as mandatory, not as an editable exemption", tool.Name)
		case tool.ExemptionOnly:
			exemptionOnly++
			assert.False(t, hasPerType, "%s has no capability rank — no step type may hold it", tool.Name)
			assert.True(t, hasOutside, "%s stays editable as an exemption", tool.Name)
		default:
			offered++
			assert.True(t, hasPerType, "%s is assignable and must have a step-type row", tool.Name)
		}
	}
	assert.Positive(t, exemptionOnly, "the fixture still exercises exemption-only tools")
	assert.Positive(t, offered, "the fixture still exercises assignable tools")

	assert.True(t, slices.ContainsFunc(rows, func(row string) bool {
		return strings.Contains(row, "] watch · for run ·")
	}), "watch is rankable now, so the run step offers it")
}

// The exemption checkboxes are the whole editable set: a box the user clears
// has to stay cleared through Apply and the next Show. The pane seeds its
// draft only in Show, so a snapshot that does not carry the edit re-ticks it.
func TestPlanTabExemptionCheckboxesSurviveApplyAndReopen(t *testing.T) {
	store := fixtureStore()
	pane := settings.New(components.DefaultTheme(), store, nil)
	pane.Show()

	// task collides with shell_task as a substring; the checkbox and the
	// separator pin the row to the short name.
	clickRow(t, pane, "] task · allowed outside plan")
	require.True(t, key(pane, xui.KeyRune, 's', xui.ModCtrl))

	require.Len(t, store.applied, 1)
	assert.NotContains(t, store.applied[0].Plan.Exemptions, "task", "the cleared box reaches the store")
	assert.Contains(t, store.applied[0].Plan.Exemptions, "shell_task", "its neighbor is untouched")

	pane.Show()
	rows := allRows(t, pane)
	assert.True(t, slices.ContainsFunc(rows, func(row string) bool {
		return strings.Contains(row, "[ ] task · allowed outside plan")
	}), "the box is still clear after reopening")
	assert.True(t, slices.ContainsFunc(rows, func(row string) bool {
		return strings.Contains(row, "[x] shell_task · allowed outside plan")
	}), "the others are still ticked")
}
