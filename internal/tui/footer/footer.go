package footer

import (
	"fmt"
	"strings"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/layout"
	"github.com/pulseaiclub/phi/internal/components/status"
	"github.com/pulseaiclub/phi/internal/session"
	"github.com/pulseaiclub/phi/internal/tui/controller"
	"github.com/pulseaiclub/phi/internal/tui/tokens"
)

type labelComposer interface {
	SetUsageHints([]components.Span)
	ClearUsageHints()
}

// FooterChrome owns activity status, spinner, token label, and footer hints.
type FooterChrome struct {
	theme         components.Theme
	spin          *status.Spinner
	activity      *controller.ActivityHandler
	contextWindow int
	lastUsage     session.TokenUsage
	updateHint    string
	hookStatus    string

	composer     labelComposer
	labelContext func() session.Snapshot
	liveJobs     func() int
	sessionID    func() string
}

// NewFooterChrome builds footer chrome with a fresh spinner and activity handler.
func NewFooterChrome(theme components.Theme, contextWindow int) *FooterChrome {
	spin := status.NewSpinner(theme.ToolName)
	return &FooterChrome{
		theme:         theme,
		spin:          spin,
		activity:      controller.NewActivityHandler(spin),
		contextWindow: contextWindow,
	}
}

// Spinner returns the shared spinner (e.g. for TranscriptPane mapper).
func (f *FooterChrome) Spinner() *status.Spinner {
	if f == nil {
		return nil
	}
	return f.spin
}

// Activity returns the activity handler.
func (f *FooterChrome) Activity() *controller.ActivityHandler {
	if f == nil {
		return nil
	}
	return f.activity
}

// BindComposer wires the composer for token display updates.
func (f *FooterChrome) BindComposer(c labelComposer) {
	if f != nil {
		f.composer = c
	}
}

// SetLabelContext supplies snap for activity footer labels.
func (f *FooterChrome) SetLabelContext(fn func() session.Snapshot) {
	if f != nil {
		f.labelContext = fn
	}
}

// SetLiveJobs supplies live sub-agent job count for the footer.
func (f *FooterChrome) SetLiveJobs(fn func() int) {
	if f != nil {
		f.liveJobs = fn
	}
}

// SetSessionID supplies the current session id; the footer shows its short
// form so a resumed session is identifiable from the first frame on.
func (f *FooterChrome) SetSessionID(fn func() string) {
	if f != nil {
		f.sessionID = fn
	}
}

// AdvanceTick drives spinner animation during active work. The frame rate
// equals the spinner glyph rate: the app loop only draws while the spinner
// is active (Editor.Draw asks for the wake), so no decimation is needed.
func (f *FooterChrome) AdvanceTick() {
	if f == nil {
		return
	}
	if f.activity.ShowSpinner() && f.spin != nil {
		f.spin.Tick()
	}
}

// SetTheme updates footer chrome styling.
func (f *FooterChrome) SetTheme(th components.Theme) {
	if f == nil {
		return
	}
	f.theme = th
	if f.spin != nil {
		f.spin.Style = th.ToolName
	}
	if f.lastUsage.Reported() {
		f.UpdateTokenDisplay(f.lastUsage)
	}
}

// UpdateTokenDisplay refreshes composer token/context labels from usage.
func (f *FooterChrome) UpdateTokenDisplay(usage session.TokenUsage) {
	if f == nil || !usage.Reported() {
		return
	}
	f.lastUsage = usage
	if f.composer == nil {
		return
	}
	combined := joinBorderParts(tokens.FormatUsageStats(usage), tokens.FormatContextLabel(usage, f.contextWindow))
	if combined == "" {
		f.composer.ClearUsageHints()
		return
	}
	f.composer.SetUsageHints([]components.Span{{
		Text: combined,
		Style: tokens.FillStyle(f.theme, tokens.ContextFillLevelFor(
			tokens.ContextFillRatio(usage.ContextTokens(), f.contextWindow), f.contextWindow)),
	}})
}

// ClearTokenDisplay clears composer token stats (e.g. after /clear).
func (f *FooterChrome) ClearTokenDisplay() {
	if f != nil {
		f.lastUsage = session.TokenUsage{}
		if f.composer != nil {
			f.composer.ClearUsageHints()
		}
	}
}

// SetHookStatus overrides the footer activity label prefix.
func (f *FooterChrome) SetHookStatus(status string) {
	if f != nil {
		f.hookStatus = status
	}
}

// Apply handles footer-related bus messages.
func (f *FooterChrome) Apply(m controller.Msg) {
	if f == nil {
		return
	}
	switch msg := m.(type) {
	case controller.SetActivityMsg:
		f.activity.Apply(msg.Activity)
	case controller.ClearIfActivityMsg:
		if f.activity.Current == msg.If {
			f.activity.Apply(controller.ActivityIdle)
		}
	case controller.RunEndedMsg:
		switch f.activity.Current {
		case controller.ActivitySubmitting, controller.ActivityWaiting, controller.ActivityStreaming,
			controller.ActivityTools, controller.ActivityCompacting:
			f.activity.Apply(controller.ActivityIdle)
		}
	case controller.UpdateAvailableMsg:
		latest := strings.TrimPrefix(msg.Latest, "v")
		f.updateHint = latest + " available · phi update"
	case controller.HookSessionEffectsMsg:
		if msg.StatusSet {
			f.hookStatus = msg.Status
		}
	}
}

// Draw renders the one-row footer surface.
func (f *FooterChrome) Draw(ctx components.DrawContext, width int) components.Surface {
	if f == nil {
		return components.NewSurface(width, 1, nil)
	}
	footer := components.NewSurface(width, 1, nil)
	dim := f.theme.Muted
	var snap session.Snapshot
	if f.labelContext != nil {
		snap = f.labelContext()
	}
	msg := f.activity.Label(snap)
	if hs := strings.TrimSpace(f.hookStatus); hs != "" {
		if msg == "" {
			msg = hs
		} else {
			msg = hs + " · " + msg
		}
	}
	if f.liveJobs != nil {
		if n := f.liveJobs(); n > 0 {
			jobBit := fmt.Sprintf("%d job", n)
			if n != 1 {
				jobBit += "s"
			}
			if msg == "" {
				msg = jobBit
			} else {
				msg = msg + " · " + jobBit
			}
		}
	}
	if f.sessionID != nil {
		if sid := strings.TrimSpace(f.sessionID()); sid != "" {
			if msg == "" {
				msg = session.ShortID(sid)
			} else {
				msg = msg + " · " + session.ShortID(sid)
			}
		}
	}

	hint := strings.TrimSpace(f.updateHint)
	hintW := 0
	if hint != "" {
		hintW = xui.StringWidth(hint, ctx.Method)
	}

	x := 1
	if msg != "" {
		if f.activity.ShowSpinner() && f.spin != nil {
			x += f.spin.PaintScan(&footer, x, 0, f.theme.ToolName, dim, ctx.Method)
			footer.Print(x, 0, " ", dim, ctx.Method)
			x += xui.StringWidth(" ", ctx.Method)
		}
		// The status yields to the right-aligned hint: stop a column short of
		// its gap, or short of the row edge when there is no hint.
		budget := width - 1 - x
		if hintW > 0 {
			budget = min(budget, width-hintW-2-x)
		}
		msg = layout.EllipsizeToWidth(msg, budget, ctx.Method)
		footer.Print(x, 0, msg, dim, ctx.Method)
		x += xui.StringWidth(msg, ctx.Method)
	}

	if hint != "" {
		hw := hintW
		hx := width - hw - 1
		hx = max(hx, x+2)
		if hx+hw <= width {
			st := f.theme.Warning
			st.Bold = false
			footer.Print(hx, 0, hint, st, ctx.Method)
		}
	}
	return footer
}

// joinBorderParts concatenates non-empty label fragments with a single space.
func joinBorderParts(parts ...string) string {
	out := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		if out != "" {
			out += " "
		}
		out += p
	}
	return out
}
