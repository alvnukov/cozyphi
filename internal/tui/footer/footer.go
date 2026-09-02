package footer

import (
	"fmt"
	"strings"
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/status"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/tokens"
	"github.com/alvnukov/cozyphi/internal/watch"
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
	liveWatches  func() []watch.Watch
	sessionID    func() string
	modelSource  func() string

	// hits are the columns the watch indicator's runs occupied on the last
	// frame, so a click can be read back into the watch it landed on.
	hits []watchHit
	// pointer is the footer surface's widget: the hand over the indicator.
	pointer *indicatorPointer
}

// rowSpan is one styled run of the footer row and, when the run belongs to
// the watch indicator, the watch a click on it addresses: a label names its
// watch, the glyph and count (empty id) address every live one.
type rowSpan struct {
	components.Span
	target bool
	watch  string
}

// watchHit is the column range one indicator run landed on.
type watchHit struct {
	x0, x1 int
	watch  string
}

// NewFooterChrome builds footer chrome with a fresh spinner and activity handler.
func NewFooterChrome(theme components.Theme, contextWindow int) *FooterChrome {
	spin := status.NewSpinner(theme.ToolName)
	f := &FooterChrome{
		theme:         theme,
		spin:          spin,
		activity:      controller.NewActivityHandler(spin),
		contextWindow: contextWindow,
	}
	f.pointer = &indicatorPointer{f: f}
	return f
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

// SetLiveWatches supplies the watch snapshot for the footer. Only live
// watches make it into the row; the editor points this at
// controller.WatchList.
func (f *FooterChrome) SetLiveWatches(fn func() []watch.Watch) {
	if f != nil {
		f.liveWatches = fn
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
	f.hits = f.hits[:0]
	var snap session.Snapshot
	if f.labelContext != nil {
		snap = f.labelContext()
	}
	if f.activity.ShowSpinner() {
		return f.drawLive(ctx, width, snap)
	}
	footer := components.NewSurface(width, 1, f.pointer)
	dim := f.theme.Muted
	run := joinRuns(dim,
		textRun(f.hookStatus, dim),
		textRun(f.activity.Label(snap), dim),
		textRun(f.jobLabel(), dim),
		f.watchRun(dim),
		textRun(f.sessionLabel(), dim),
	)

	hint := strings.TrimSpace(f.updateHint)
	hintW := 0
	if hint != "" {
		hintW = xui.StringWidth(hint, ctx.Method)
	}

	x := 1
	if len(run) > 0 {
		// The status yields to the right-aligned hint: stop a column short of
		// its gap, or short of the row edge when there is no hint.
		budget := width - 1 - x
		if hintW > 0 {
			budget = min(budget, width-hintW-2-x)
		}
		x = f.paintRun(&footer, x, budget, run, ctx.Method)
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
	footer := components.NewSurface(width, 1, f.pointer)
	dim := f.theme.Muted
	label := f.activity.Label(snap)
	verb := strings.TrimSuffix(label, "…")

	run := []rowSpan{{Span: components.Span{Text: "✻ ", Style: components.PulseStyle(f.theme.ToolName, dim)}}}
	if hs := strings.TrimSpace(f.hookStatus); hs != "" {
		run = append(run, plainSpan(hs+" · ", dim))
	}
	// The loader names the engine's live model — the who behind the phase
	// label — so it never drops to generic mid-run (waiting, tools, and
	// compaction included).
	if f.modelSource != nil {
		if model := strings.TrimSpace(f.modelSource()); model != "" {
			run = append(run, plainSpan(model+" · ", dim))
		}
	}
	for _, sp := range components.WaveLabel(verb, f.theme.ToolName, dim) {
		run = append(run, rowSpan{Span: sp})
	}
	if verb != label {
		run = append(run, plainSpan("…", dim))
	}
	start, turnTokens := liveTurn(snap)
	if !start.IsZero() {
		if d := time.Since(start); d >= time.Second {
			run = append(run, plainSpan(" · "+components.FormatDuration(d), dim))
		}
	}
	if turnTokens > 0 {
		run = append(run, plainSpan(" · ↓"+tokens.FormatTokens(turnTokens), dim))
	}
	if lbl := f.jobLabel(); lbl != "" {
		run = append(run, plainSpan(" · "+lbl, dim))
	}
	if wr := f.watchRun(dim); len(wr) > 0 {
		run = append(run, plainSpan(" · ", dim))
		run = append(run, wr...)
	}
	if sid := f.sessionLabel(); sid != "" {
		run = append(run, plainSpan(" · "+sid, dim))
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
	x := f.paintRun(&footer, 1, budget, run, ctx.Method)
	if hx := width - hintW - 1; hx >= x+2 {
		footer.Print(hx, 0, hint, hintSt, ctx.Method)
	}
	return footer
}

// paintRun prints the runs from column x, clipped to budget, and records
// the columns the watch indicator's runs landed on. It returns the next
// free column.
func (f *FooterChrome) paintRun(
	dst *components.Surface, x, budget int, run []rowSpan, method xui.WidthMethod,
) int {
	for _, sp := range clipRun(run, budget, method) {
		dst.Print(x, 0, sp.Text, sp.Style, method)
		w := xui.StringWidth(sp.Text, method)
		if sp.target && w > 0 {
			f.hits = append(f.hits, watchHit{x0: x, x1: x + w, watch: sp.watch})
		}
		x += w
	}
	return x
}

// watchRun renders the live-watch indicator — a breathing ⏱, the count and
// the labels — as click-addressable runs. A watch set with nothing live
// reads as no watches at all: finished watches are history the transcript
// already shows.
func (f *FooterChrome) watchRun(dim xui.Style) []rowSpan {
	live := f.watchesLive()
	if len(live) == 0 {
		return nil
	}
	noun := "watches"
	if len(live) == 1 {
		noun = "watch"
	}
	run := []rowSpan{
		{Span: components.Span{Text: "⏱ ", Style: components.PulseStyle(f.theme.ToolName, dim)}, target: true},
		{Span: components.Span{Text: fmt.Sprintf("%d %s: ", len(live), noun), Style: dim}, target: true},
	}
	for i, w := range live {
		if i > 0 {
			run = append(run, rowSpan{Span: components.Span{Text: ", ", Style: dim}, target: true})
		}
		label := w.Label
		if label == "" {
			label = "(unlabeled)"
		}
		run = append(run, rowSpan{Span: components.Span{Text: label, Style: dim}, target: true, watch: w.ID})
	}
	return run
}

// watchesLive filters the watch snapshot down to the running ones.
func (f *FooterChrome) watchesLive() []watch.Watch {
	if f == nil || f.liveWatches == nil {
		return nil
	}
	var live []watch.Watch
	for _, w := range f.liveWatches() {
		if w.Live {
			live = append(live, w)
		}
	}
	return live
}

// WatchesLive reports whether any watch is running. The indicator breathes
// while one is, so the app loop has to keep drawing frames for it.
func (f *FooterChrome) WatchesLive() bool {
	return len(f.watchesLive()) > 0
}

// WatchesAt maps a click column on the footer row to the live watches it
// addresses: a label names one watch, the glyph and count name them all.
// It reports false off the indicator — and for a label whose watch ended
// since the last frame, which leaves the click nothing to act on. Columns
// are those of the last Draw.
func (f *FooterChrome) WatchesAt(x int) ([]watch.Watch, bool) {
	if f == nil {
		return nil, false
	}
	for _, h := range f.hits {
		if x < h.x0 || x >= h.x1 {
			continue
		}
		live := f.watchesLive()
		if h.watch == "" {
			return live, len(live) > 0
		}
		for _, w := range live {
			if w.ID == h.watch {
				return []watch.Watch{w}, true
			}
		}
		return nil, false
	}
	return nil, false
}

// indicatorPointer is the footer surface's widget. Its only job is the
// pointer shape: the hand over the watch indicator, the default elsewhere.
// Clicks are not handled here — the editor routes them after its modal
// check, so a click on the indicator never outranks an open ask.
type indicatorPointer struct {
	f *FooterChrome
}

// Handle is inert: the editor owns footer clicks.
func (*indicatorPointer) Handle(*components.EventContext, xui.Event) {}

// Draw satisfies components.Widget; the editor draws the footer directly.
func (p *indicatorPointer) Draw(ctx components.DrawContext) components.Surface {
	return p.f.Draw(ctx, ctx.Max.Width)
}

// PointerShape offers the hand exactly where a click folds watch rows.
func (p *indicatorPointer) PointerShape(x, _ int) string {
	if _, ok := p.f.WatchesAt(x); ok {
		return components.ShapePointer
	}
	return ""
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

// clipRun cuts a one-row span run at budget columns, ending with an
// ellipsis when anything had to go. The ellipsis inherits the cut run's
// click target, so a clipped watch label still folds its watch.
func clipRun(run []rowSpan, budget int, method xui.WidthMethod) []rowSpan {
	total := 0
	for _, sp := range run {
		total += xui.StringWidth(sp.Text, method)
	}
	if total <= budget {
		return run
	}
	if budget < 1 {
		return nil
	}
	out := make([]rowSpan, 0, len(run))
	used := 0
	for _, sp := range run {
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
			cut := sp
			cut.Text = b.String()
			out = append(out, cut)
		}
		tail := sp
		tail.Text = "…"
		return append(out, tail)
	}
	return out
}

// plainSpan is one unstyled-beyond-st run with no click target.
func plainSpan(text string, st xui.Style) rowSpan {
	return rowSpan{Span: components.Span{Text: text, Style: st}}
}

// textRun is a trimmed label as a run, or nothing for an empty label.
func textRun(text string, st xui.Style) []rowSpan {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	return []rowSpan{plainSpan(text, st)}
}

// joinRuns concatenates the non-empty runs with a " · " separator.
func joinRuns(sep xui.Style, parts ...[]rowSpan) []rowSpan {
	var out []rowSpan
	for _, p := range parts {
		if len(p) == 0 {
			continue
		}
		if len(out) > 0 {
			out = append(out, plainSpan(" · ", sep))
		}
		out = append(out, p...)
	}
	return out
}

// jobLabel counts live sub-agent jobs for the footer.
func (f *FooterChrome) jobLabel() string {
	if f == nil || f.liveJobs == nil {
		return ""
	}
	n := f.liveJobs()
	switch {
	case n <= 0:
		return ""
	case n == 1:
		return "1 job"
	default:
		return fmt.Sprintf("%d jobs", n)
	}
}

// sessionLabel is the short form of the current session id, or nothing.
func (f *FooterChrome) sessionLabel() string {
	if f == nil || f.sessionID == nil {
		return ""
	}
	if sid := strings.TrimSpace(f.sessionID()); sid != "" {
		return session.ShortID(sid)
	}
	return ""
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
