package footer

import (
	"fmt"
	"strings"
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/layout"
	"github.com/alvnukov/cozyphi/internal/components/status"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/tokens"
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
	modelSource  func() string
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

// SetModelSource supplies the working model's name — the engine's live
// model — so a spinning footer says who is running, in every run phase.
func (f *FooterChrome) SetModelSource(fn func() string) {
	if f != nil {
		f.modelSource = fn
	}
}

// AdvanceTick drives spinner animation during active work. It is time-based:
// the spinner only advances when the wall clock has passed its interval, so
// extra redraws (e.g. mouse movement) do not speed the animation up.
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
		f.updateHint = latest + " available · cozyphi update"
	case controller.HookSessionEffectsMsg:
		if msg.StatusSet {
			f.hookStatus = msg.Status
		}
	}
}

// Draw renders the one-row footer surface: the consolidated live-activity
// line while the run spins, the quiet status line otherwise.
func (f *FooterChrome) Draw(ctx components.DrawContext, width int) components.Surface {
	if f == nil {
		return components.NewSurface(width, 1, nil)
	}
	var snap session.Snapshot
	if f.labelContext != nil {
		snap = f.labelContext()
	}
	if f.activity.ShowSpinner() {
		return f.drawLive(ctx, width, snap)
	}
	footer := components.NewSurface(width, 1, nil)
	dim := f.theme.Muted
	msg := f.activity.Label(snap)
	if hs := strings.TrimSpace(f.hookStatus); hs != "" {
		msg = dotJoin(hs, msg)
	}
	if f.liveJobs != nil {
		if n := f.liveJobs(); n > 0 {
			msg = dotJoin(msg, jobLabel(n))
		}
	}
	if f.sessionID != nil {
		if sid := strings.TrimSpace(f.sessionID()); sid != "" {
			msg = dotJoin(msg, session.ShortID(sid))
		}
	}

	hint := strings.TrimSpace(f.updateHint)
	hintW := 0
	if hint != "" {
		hintW = xui.StringWidth(hint, ctx.Method)
	}

	x := 1
	if msg != "" {
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

// drawLive renders the streaming turn's activity line — the one place that
// answers "what is the model doing right now, for how long, at what cost":
// a breathing glyph, the working model, the phase verb under a soft letter
// shimmer, the turn's elapsed time and token stream, and the interrupt hint.
// The scan-bar spinner is gone from here: the only spinner glyph left in
// view is the active transcript row's.
func (f *FooterChrome) drawLive(ctx components.DrawContext, width int, snap session.Snapshot) components.Surface {
	footer := components.NewSurface(width, 1, nil)
	dim := f.theme.Muted
	label := f.activity.Label(snap)
	verb := strings.TrimSuffix(label, "…")

	spans := []components.Span{{Text: "✻ ", Style: components.PulseStyle(f.theme.ToolName, dim)}}
	if hs := strings.TrimSpace(f.hookStatus); hs != "" {
		spans = append(spans, components.Span{Text: hs + " · ", Style: dim})
	}
	// The loader names the engine's live model — the who behind the phase
	// label — so it never drops to generic mid-run (waiting, tools, and
	// compaction included).
	if f.modelSource != nil {
		if model := strings.TrimSpace(f.modelSource()); model != "" {
			spans = append(spans, components.Span{Text: model + " · ", Style: dim})
		}
	}
	spans = append(spans, components.WaveLabel(verb, f.theme.ToolName, dim)...)
	if verb != label {
		spans = append(spans, components.Span{Text: "…", Style: dim})
	}
	start, turnTokens := liveTurn(snap)
	if !start.IsZero() {
		if d := time.Since(start); d >= time.Second {
			spans = append(spans, components.Span{Text: " · " + components.FormatDuration(d), Style: dim})
		}
	}
	if turnTokens > 0 {
		spans = append(spans, components.Span{Text: " · ↓" + tokens.FormatTokens(turnTokens), Style: dim})
	}
	if f.liveJobs != nil {
		if n := f.liveJobs(); n > 0 {
			spans = append(spans, components.Span{Text: " · " + jobLabel(n), Style: dim})
		}
	}
	if f.sessionID != nil {
		if sid := strings.TrimSpace(f.sessionID()); sid != "" {
			spans = append(spans, components.Span{Text: " · " + session.ShortID(sid), Style: dim})
		}
	}

	// The interrupt hint holds the right edge; a pending update outranks it.
	hint := "Esc interrupts"
	hintSt := dim
	if uh := strings.TrimSpace(f.updateHint); uh != "" {
		hint = uh
		hintSt = f.theme.Warning
		hintSt.Bold = false
	}
	hintW := xui.StringWidth(hint, ctx.Method)

	budget := width - 2
	if hintW > 0 {
		budget = width - hintW - 4
	}
	x := 1
	for _, sp := range clipSpans(spans, budget, ctx.Method) {
		footer.Print(x, 0, sp.Text, sp.Style, ctx.Method)
		x += xui.StringWidth(sp.Text, ctx.Method)
	}
	if hx := width - hintW - 1; hx >= x+2 {
		footer.Print(hx, 0, hint, hintSt, ctx.Method)
	}
	return footer
}

// liveTurn finds the running turn — everything after the last sent user
// message — and reports the wall-clock start of its first timed assistant
// round plus the completion tokens its rounds have streamed so far.
func liveTurn(snap session.Snapshot) (start time.Time, completion int) {
	msgs := snap.Messages
	turn := 0
	for i, m := range msgs {
		if m.Role == session.RoleUser && !m.Queued {
			turn = i + 1
		}
	}
	for _, m := range msgs[turn:] {
		if m.Role != session.RoleAssistant {
			continue
		}
		if start.IsZero() && !m.Started.IsZero() {
			start = m.Started
		}
		completion += m.Usage.CompletionTokens
	}
	return start, completion
}

// clipSpans cuts a one-row span run at budget columns, ending with an
// ellipsis when anything had to go.
func clipSpans(spans []components.Span, budget int, method xui.WidthMethod) []components.Span {
	total := 0
	for _, sp := range spans {
		total += xui.StringWidth(sp.Text, method)
	}
	if total <= budget {
		return spans
	}
	if budget < 1 {
		return nil
	}
	out := make([]components.Span, 0, len(spans))
	used := 0
	for _, sp := range spans {
		if w := xui.StringWidth(sp.Text, method); used+w < budget {
			out = append(out, sp)
			used += w
			continue
		}
		var b strings.Builder
		for _, r := range sp.Text {
			rw := xui.StringWidth(string(r), method)
			if used+rw >= budget {
				break
			}
			b.WriteRune(r)
			used += rw
		}
		if b.Len() > 0 {
			out = append(out, components.Span{Text: b.String(), Style: sp.Style})
		}
		return append(out, components.Span{Text: "…", Style: sp.Style})
	}
	return out
}

// jobLabel counts live sub-agent jobs for the footer.
func jobLabel(n int) string {
	if n == 1 {
		return "1 job"
	}
	return fmt.Sprintf("%d jobs", n)
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

// dotJoin concatenates non-empty label fragments with a " · " separator.
func dotJoin(parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, " · ")
}
