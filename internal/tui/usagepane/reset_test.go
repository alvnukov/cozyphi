package usagepane

import (
	"errors"
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/provider"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

type resetFixture struct {
	pane      *Pane
	stats     controller.SessionStats
	quota     controller.UsageQuotaMsg
	calls     []*provider.QuotaResetTarget
	refreshes int
}

func newResetFixture() *resetFixture {
	f := &resetFixture{stats: fixtureStats(), quota: largeQuota()}
	// The pane treats the target as opaque; only the provider validates it.
	f.quota.Snapshot.ResetTarget = &provider.QuotaResetTarget{}
	f.pane = New(components.DefaultTheme(), func() controller.SessionStats { return f.stats },
		func() { f.refreshes++ }, func(target *provider.QuotaResetTarget) { f.calls = append(f.calls, target) }, nil)
	f.pane.Show()
	f.pane.Apply(f.quota)
	return f
}

func clickLabel(t *testing.T, p *Pane, label string, action xui.MouseAction, button xui.MouseButton) xui.MouseEvent {
	t.Helper()
	for y, row := range strings.Split(draw80(p, 24), "\n") {
		if x := strings.Index(row, label); x >= 0 {
			e := xui.MouseEvent{X: x, Y: y, Action: action, Button: button}
			require.True(t, p.HandleEvent(&components.EventContext{}, e))
			return e
		}
	}
	t.Fatalf("button %q not found", label)
	return xui.MouseEvent{}
}

func TestResetKeyboardConfirmationAndDuplicateGuard(t *testing.T) {
	f := newResetFixture()
	p := f.pane
	assert.Contains(t, draw80(p, 24), "[Reset limit x]")
	press(t, p, xui.KeyRune, 'y')
	assert.Empty(t, f.calls, "bare y cannot mutate")
	press(t, p, xui.KeyRune, 'x')
	assert.Empty(t, f.calls, "request only opens confirmation")
	assert.Contains(t, draw80(p, 24), "Spend one reset credit to reset eligible usage limits?")
	assert.Contains(t, draw80(p, 24), "[Confirm y]")
	press(t, p, xui.KeyRune, 'x')
	press(t, p, xui.KeyEnter, 0)
	p.HandleEvent(&components.EventContext{}, xui.KeyEvent{Code: xui.KeyRune, Rune: 'y', Press: false})
	assert.Empty(t, f.calls)
	press(t, p, xui.KeyRune, 'y')
	require.Len(t, f.calls, 1)
	assert.Same(t, f.quota.Snapshot.ResetTarget, f.calls[0])
	assert.Contains(t, draw80(p, 24), "Reset in progress")
	assert.NotContains(t, draw80(p, 24), "Spend one reset credit")
	for range 3 {
		press(t, p, xui.KeyRune, 'x')
		press(t, p, xui.KeyRune, 'y')
	}
	assert.Len(t, f.calls, 1)
}

func TestResetDisarmsBeforeReentrantCallback(t *testing.T) {
	f := newResetFixture()
	calls := 0
	f.pane.onReset = func(*provider.QuotaResetTarget) {
		calls++
		require.LessOrEqual(t, calls, 1)
		assert.NotContains(t, draw80(f.pane, 24), "Spend one reset credit")
		assert.Contains(t, draw80(f.pane, 24), "Reset in progress")
		press(t, f.pane, xui.KeyRune, 'x')
		press(t, f.pane, xui.KeyRune, 'y')
	}
	press(t, f.pane, xui.KeyRune, 'x')
	draw80(f.pane, 24)
	press(t, f.pane, xui.KeyRune, 'y')
	assert.Equal(t, 1, calls)
}

func TestResetCancellationSendsNothing(t *testing.T) {
	for _, cancel := range []string{"n", "escape", "mouse", "reopen", "refresh", "quota", "invalidate"} {
		t.Run(cancel, func(t *testing.T) {
			f := newResetFixture()
			p := f.pane
			press(t, p, xui.KeyRune, 'x')
			draw80(p, 24)
			switch cancel {
			case "n":
				press(t, p, xui.KeyRune, 'n')
			case "escape":
				press(t, p, xui.KeyEscape, 0)
				assert.True(t, p.Visible(), "Esc cancels confirmation, not the pane")
			case "mouse":
				clickLabel(t, p, "[Cancel n/Esc]", xui.MousePress, xui.MouseLeft)
			case "reopen":
				p.Hide()
				p.Show()
			case "refresh":
				press(t, p, xui.KeyRune, 'r')
			case "quota":
				p.Apply(f.quota)
			case "invalidate":
				p.InvalidateReset()
			}
			press(t, p, xui.KeyRune, 'y')
			assert.Empty(t, f.calls)
			assert.NotContains(t, draw80(p, 24), "Spend one reset credit")
		})
	}
}

func TestResetMouseRequiresLeftPressAndSeparateConfirmCoordinates(t *testing.T) {
	f := newResetFixture()
	p := f.pane
	for _, action := range []xui.MouseAction{xui.MouseRelease, xui.MouseDrag, xui.MouseMotion} {
		clickLabel(t, p, "[Reset limit x]", action, xui.MouseLeft)
		assert.NotContains(t, draw80(p, 24), "Spend one reset credit")
	}
	clickLabel(t, p, "[Reset limit x]", xui.MousePress, xui.MouseRight)
	assert.NotContains(t, draw80(p, 24), "Spend one reset credit")
	original := clickLabel(t, p, "[Reset limit x]", xui.MousePress, xui.MouseLeft)
	// A terminal double click is two presses, possibly separated by redraw.
	p.HandleEvent(&components.EventContext{}, original)
	draw80(p, 24)
	p.HandleEvent(&components.EventContext{}, original)
	assert.Empty(t, f.calls)
	for _, action := range []xui.MouseAction{xui.MouseRelease, xui.MouseDrag, xui.MouseMotion} {
		clickLabel(t, p, "[Confirm y]", action, xui.MouseLeft)
		clickLabel(t, p, "[Cancel n/Esc]", action, xui.MouseLeft)
		assert.Empty(t, f.calls)
		assert.Contains(t, draw80(p, 24), "Spend one reset credit")
	}
	clickLabel(t, p, "[Confirm y]", xui.MousePress, xui.MouseRight)
	assert.Empty(t, f.calls)
	confirm := clickLabel(t, p, "[Confirm y]", xui.MousePress, xui.MouseLeft)
	assert.NotEqual(t, original.Y, confirm.Y)
	p.HandleEvent(&components.EventContext{}, confirm)
	draw80(p, 24)
	p.HandleEvent(&components.EventContext{}, confirm)
	assert.Len(t, f.calls, 1)
}

func TestResetDisabledExplanations(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		change     func(*resetFixture)
	}{
		{"unknown", "credit count unknown", func(f *resetFixture) { f.quota.Snapshot.Reset.Supported = false }},
		{"zero", "No reset credits", func(f *resetFixture) { f.quota.Snapshot.Reset.Available = 0 }},
		{"negative", "No reset credits", func(f *resetFixture) { f.quota.Snapshot.Reset.Available = -1 }},
		{"no target", "supported OAuth account", func(f *resetFixture) { f.quota.Snapshot.ResetTarget = nil }},
		{"unsupported", "unavailable for this provider", func(f *resetFixture) { f.quota.Unsupported = true }},
		{"disconnected", "check /connect", func(f *resetFixture) { f.quota.Err = errors.New("not connected") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newResetFixture()
			tc.change(f)
			f.pane.Apply(f.quota)
			assert.Contains(t, draw80(f.pane, 24), tc.want)
			press(t, f.pane, xui.KeyRune, 'x')
			press(t, f.pane, xui.KeyRune, 'y')
			assert.Empty(t, f.calls)
			assert.NotContains(t, draw80(f.pane, 24), "Spend one reset credit")
		})
	}
}

func TestResetSessionChangesInvalidateConsentAndOldQuota(t *testing.T) {
	for _, change := range []string{"provider", "model"} {
		for _, when := range []string{"before arm", "before confirm", "before apply", "explicit"} {
			t.Run(change+"/"+when, func(t *testing.T) {
				f := newResetFixture()
				p := f.pane
				if when != "before arm" {
					press(t, p, xui.KeyRune, 'x')
					draw80(p, 24)
				}
				if change == "provider" {
					f.stats.ProviderID = "other"
				} else {
					f.stats.Model = "other"
				}
				if when == "explicit" {
					p.InvalidateReset()
				}
				if when == "before apply" || when == "explicit" {
					p.Apply(f.quota)
				}
				if when == "before arm" {
					press(t, p, xui.KeyRune, 'x')
				}
				press(t, p, xui.KeyRune, 'y')
				assert.Empty(t, f.calls)
				assert.NotContains(t, draw80(p, 24), "Spend one reset credit")
				press(t, p, xui.KeyRune, 'x')
				press(t, p, xui.KeyRune, 'y')
				assert.Empty(t, f.calls, "old target must not be rearmed")
			})
		}
	}
	f := newResetFixture()
	wrong := f.quota
	wrong.ProviderID = "other"
	wrong.Snapshot.PlanName = "WRONG PLAN"
	f.pane.Apply(wrong)
	assert.NotContains(t, draw80(f.pane, 24), "WRONG PLAN")
}

func TestResetBusySurvivesReopenAndHiddenLifecycleMessages(t *testing.T) {
	f := newResetFixture()
	p := f.pane
	press(t, p, xui.KeyRune, 'x')
	draw80(p, 24)
	press(t, p, xui.KeyRune, 'y')
	p.Hide()
	p.Show()
	p.Apply(f.quota)
	press(t, p, xui.KeyRune, 'x')
	press(t, p, xui.KeyRune, 'y')
	assert.Len(t, f.calls, 1, "local busy slot survives reopening before the controller acknowledges")
	p.Hide()
	p.ApplyReset(controller.UsageResetMsg{InFlight: true})
	p.Show()
	p.Apply(f.quota)
	assert.Contains(t, draw80(p, 24), "Reset in progress")
	press(t, p, xui.KeyRune, 'x')
	press(t, p, xui.KeyRune, 'y')
	assert.Len(t, f.calls, 1)
	p.Hide()
	p.ApplyReset(
		controller.UsageResetMsg{Result: provider.QuotaResetResult{Code: provider.QuotaResetDone, WindowsReset: 2}},
	)
	p.Show()
	p.Apply(f.quota)
	assert.Contains(t, draw80(p, 24), "Reset complete: 2 windows reset")
	assert.NotContains(t, draw80(p, 24), "Reset in progress")
}

func TestResetResultStatusesNeverRetryAndSurviveQuotaFailure(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		msg        controller.UsageResetMsg
	}{
		{"done", "Reset complete: 2 windows reset", controller.UsageResetMsg{Result: provider.QuotaResetResult{Code: provider.QuotaResetDone, WindowsReset: 2}}},
		{"nothing", "No limits needed resetting", controller.UsageResetMsg{Result: provider.QuotaResetResult{Code: provider.QuotaResetNothing}}},
		{"no credit", "No reset credits available", controller.UsageResetMsg{Result: provider.QuotaResetResult{Code: provider.QuotaResetNoCredit}}},
		{"redeemed", "Reset credit already redeemed", controller.UsageResetMsg{Result: provider.QuotaResetResult{Code: provider.QuotaResetAlreadyRedeemed}}},
		{"unknown", "Reset outcome unknown; check Codex", controller.UsageResetMsg{Err: provider.ErrQuotaResetUnknown}},
		{"stale", "Reset target changed", controller.UsageResetMsg{Err: provider.ErrQuotaResetStale}},
		{"error", "Reset failed", controller.UsageResetMsg{Err: errors.New("secret-transport-detail")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newResetFixture()
			p := f.pane
			press(t, p, xui.KeyRune, 'x')
			draw80(p, 24)
			press(t, p, xui.KeyRune, 'y')
			p.ApplyReset(tc.msg)
			assert.Contains(t, draw80(p, 24), tc.want)
			assert.NotContains(t, draw80(p, 24), "secret-transport-detail")
			press(t, p, xui.KeyRune, 'x')
			press(t, p, xui.KeyRune, 'y')
			assert.Len(t, f.calls, 1)
			assert.Equal(t, 1, f.refreshes, "results never perform I/O")
			p.Apply(controller.UsageQuotaMsg{ProviderID: "openai", Err: errors.New("quota refresh failed")})
			text := draw80(p, 24)
			assert.Contains(t, text, tc.want, "mutation outcome is independent of later refresh failure")
			assert.Contains(t, text, "quota refresh failed")
			assert.Contains(t, text, "Session")
			assert.Len(t, f.calls, 1)
		})
	}
}

func TestSharedReportNeverRendersResetActions(t *testing.T) {
	f := newResetFixture()
	press(t, f.pane, xui.KeyRune, 'x')
	s := f.pane.Report(components.DrawContext{Max: components.Size{Width: 80}})
	text := components.SurfaceText(s)
	assert.NotContains(t, text, "Reset limit")
	assert.NotContains(t, text, "Spend one reset credit")
	assert.NotContains(t, text, "Confirm")
	assert.Contains(t, text, "Session")
	assert.LessOrEqual(t, s.Size.Height, 14)
}

func TestResetNeedsFreshQuotaAfterInvalidation(t *testing.T) {
	f := newResetFixture()
	p := f.pane
	p.InvalidateReset()
	p.Apply(f.quota)
	press(t, p, xui.KeyRune, 'x')
	press(t, p, xui.KeyRune, 'y')
	assert.Empty(t, f.calls)
	press(t, p, xui.KeyRune, 'r')
	p.Apply(f.quota)
	press(t, p, xui.KeyRune, 'x')
	draw80(p, 24)
	press(t, p, xui.KeyRune, 'y')
	assert.Len(t, f.calls, 1)
}

func TestResetWithoutSessionConnectionAndDrawWithoutIO(t *testing.T) {
	calls := 0
	p := New(components.DefaultTheme(), func() controller.SessionStats {
		return controller.SessionStats{}
	}, nil, func(*provider.QuotaResetTarget) { calls++ }, nil)
	p.Show()
	assert.Contains(t, draw80(p, 24), "Not connected; open /connect")
	press(t, p, xui.KeyRune, 'x')
	press(t, p, xui.KeyRune, 'y')
	assert.Zero(t, calls)

	f := newResetFixture()
	for _, height := range []int{1, 2, 3, 4, 5, 6, 24} {
		for _, width := range []int{1, 12, 80} {
			s := f.pane.Draw(components.DrawContext{Max: components.Size{Width: width, Height: height}})
			assert.Equal(t, height, s.Size.Height)
			assert.Equal(t, width, s.Size.Width)
		}
	}
	assert.Empty(t, f.calls)
	assert.Equal(t, 1, f.refreshes)
}

func TestResetRequiresVisibleCurrentConfirmation(t *testing.T) {
	for _, size := range []components.Size{{Width: 80, Height: 5}, {Width: 20, Height: 24}} {
		f := newResetFixture()
		press(t, f.pane, xui.KeyRune, 'x')
		f.pane.Draw(components.DrawContext{Max: size, Method: xui.WidthUnicode})
		press(t, f.pane, xui.KeyRune, 'y')
		require.Empty(t, f.calls, "incomplete warning cannot authorize a credit spend")
	}
	f := newResetFixture()
	press(t, f.pane, xui.KeyRune, 'x')
	press(t, f.pane, xui.KeyRune, 'y')
	require.Empty(t, f.calls, "queued input before a frame cannot confirm")
	press(t, f.pane, xui.KeyRune, 'x')
	draw80(f.pane, 24)
	f.pane.Draw(components.DrawContext{Max: components.Size{Width: 80, Height: 5}})
	press(t, f.pane, xui.KeyRune, 'y')
	require.Empty(t, f.calls, "shrinking withdraws visible consent")
	press(t, f.pane, xui.KeyRune, 'x')
	draw80(f.pane, 24)
	f.pane.HandleEvent(&components.EventContext{}, xui.ResizeEvent{})
	press(t, f.pane, xui.KeyRune, 'y')
	require.Empty(t, f.calls, "resize invalidates consent before the next draw")
}

func TestResetFreshGenerationRestoresAction(t *testing.T) {
	f := newResetFixture()
	f.quota.Generation = 1
	f.pane.Apply(f.quota)
	press(t, f.pane, xui.KeyRune, 'x')
	draw80(f.pane, 24)
	press(t, f.pane, xui.KeyRune, 'y')
	f.pane.ApplyReset(controller.UsageResetMsg{Result: provider.QuotaResetResult{Code: provider.QuotaResetDone}})
	f.quota.Generation = 2
	f.quota.Snapshot.ResetTarget = &provider.QuotaResetTarget{}
	f.pane.Apply(f.quota)
	require.Contains(t, draw80(f.pane, 24), "[Reset limit x]")
	require.Len(t, f.calls, 1, "fresh read must not itself retry the mutation")
	press(t, f.pane, xui.KeyRune, 'x')
	draw80(f.pane, 24)
	press(t, f.pane, xui.KeyRune, 'y')
	require.Len(t, f.calls, 2, "a fresh separately confirmed intent is available")
}
