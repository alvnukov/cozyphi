// Package usagepane renders the full-screen usage browser (/usage): the
// active provider's subscription quota — plan name, one bar per usage window,
// reset times — next to the running session's token totals. The pane is a
// dumb view: the session snapshot comes from an injected seam, the quota from
// UsageQuotaMsg published by the controller, and a refresh is a callback, so
// no network or engine state lives here.
package usagepane

import (
	"fmt"
	"strings"
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/layout"
	"github.com/alvnukov/cozyphi/internal/provider"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/tokens"
)

// barWidth is the fixed width, in cells, of a subscription usage bar.
const barWidth = 14

// Pane is the usage browser. Mutated and rendered on the UI goroutine.
type Pane struct {
	theme components.Theme

	// sessionStats re-pulls cumulative session totals.
	sessionStats func() controller.SessionStats
	// onRefresh asks the shell to fetch the subscription quota again; the
	// controller skips a fetch already in flight, so callers may hammer it.
	onRefresh func()
	onReset   func(*provider.QuotaResetTarget)
	// onClose fires once whenever the pane stops being visible, so the
	// shell can hand the keyboard back to the composer.
	onClose func()

	session controller.SessionStats
	quota   controller.UsageQuotaMsg
	// loading covers the gap between Show/refresh and the UsageQuotaMsg.
	loading bool
	visible bool

	scroll, height, contentHeight int
	reset                         resetState
}

// SetTheme restyles the browser; the pane takes every color from it.
func (p *Pane) SetTheme(th components.Theme) {
	if p != nil {
		p.theme = th
	}
}

// New builds a hidden pane. Every side effect goes back through these seams.
func New(
	theme components.Theme,
	sessionStats func() controller.SessionStats,
	onRefresh func(),
	onReset func(*provider.QuotaResetTarget),
	onClose func(),
) *Pane {
	return &Pane{
		theme:        theme,
		sessionStats: sessionStats,
		onRefresh:    onRefresh,
		onClose:      onClose,
		onReset:      onReset,
	}
}

// Show pulls the session snapshot, kicks off a quota fetch and opens the pane.
func (p *Pane) Show() {
	p.InvalidateReset()
	p.pullSession()
	p.reset.invalidated = false
	p.quota = controller.UsageQuotaMsg{}
	p.loading = true
	p.visible = true
	p.scroll = 0
	if p.onRefresh != nil {
		p.onRefresh()
	}
}

// Hide closes the pane and hands the keyboard back to the shell.
func (p *Pane) Hide() {
	if !p.visible {
		return
	}
	p.InvalidateReset()
	p.visible = false
	if p.onClose != nil {
		p.onClose()
	}
}

// Visible reports whether the pane covers the screen.
func (p *Pane) Visible() bool { return p != nil && p.visible }

// Apply takes a quota result already accepted by the controller's generation
// check. A later generation can restore reset availability after completion.
func (p *Pane) Apply(msg controller.UsageQuotaMsg) {
	p.pullSession()
	if !p.visible || msg.ProviderID == "" || (p.sessionStats != nil && msg.ProviderID != p.session.ProviderID) {
		return
	}
	p.cancelReset()
	if msg.Generation > p.quota.Generation {
		p.reset.invalidated = false
	}
	if p.reset.invalidated || p.reset.busy || msg.Err != nil || msg.Unsupported {
		msg.Snapshot.ResetTarget = nil
	}
	p.quota = msg
	p.loading = false
}

// Handle implements components.Widget; the editor owns dispatch and calls
// HandleEvent instead, so this entry point is intentionally inert.
func (*Pane) Handle(*components.EventContext, xui.Event) {}

// HandleEvent drives the pane while visible. It consumes every key and mouse
// event so nothing leaks into the shell underneath: Esc closes, r refreshes,
// the wheel scrolls, everything else stays put.
func (p *Pane) HandleEvent(ctx *components.EventContext, ev xui.Event) bool {
	if p == nil || !p.visible {
		return false
	}
	switch e := ev.(type) {
	case xui.ResizeEvent:
		p.cancelReset()
		return false
	case xui.KeyEvent:
		if !e.Press {
			return true
		}
		p.handleKey(e)
		ctx.ConsumeAndRedraw()
		return true
	case xui.MouseEvent:
		p.handleResetMouse(e)
		switch e.Button {
		case xui.MouseWheelDown:
			p.scroll += max(1, e.Wheel)
		case xui.MouseWheelUp:
			p.scroll -= max(1, e.Wheel)
		}
		p.clampScroll()
		ctx.ConsumeAndRedraw()
		return true
	default:
		return false
	}
}

func (p *Pane) handleKey(e xui.KeyEvent) {
	if p.handleResetKey(e) {
		return
	}
	switch e.Code {
	case xui.KeyEscape:
		p.Hide()
	case xui.KeyUp:
		p.scroll--
	case xui.KeyDown:
		p.scroll++
	case xui.KeyPageUp:
		p.scroll -= max(1, p.height)
	case xui.KeyPageDown:
		p.scroll += max(1, p.height)
	case xui.KeyHome:
		p.scroll = 0
	case xui.KeyEnd:
		p.scroll = p.contentHeight
	case xui.KeyRune:
		if e.Rune == 'r' {
			p.InvalidateReset()
			p.pullSession()
			p.reset.invalidated = false
			p.loading = true
			p.scroll = 0
			if p.onRefresh != nil {
				p.onRefresh()
			}
		}
	}
	p.clampScroll()
}

func (p *Pane) clampScroll() {
	p.scroll = max(0, min(p.scroll, max(0, p.contentHeight-p.height)))
}

func (p *Pane) pullSession() {
	if p.sessionStats != nil {
		current := p.sessionStats()
		if current.ProviderID != p.session.ProviderID || current.Model != p.session.Model {
			p.InvalidateReset()
		}
		p.session = current
	}
}

// Draw paints the pane over the whole available area with an opaque
// background, so the transcript does not bleed through.
func (p *Pane) Draw(ctx components.DrawContext) components.Surface {
	w, h := ctx.Max.Width, ctx.Max.Height
	if w <= 0 {
		w = 40
	}
	if h <= 0 {
		h = 24
	}

	th := p.theme
	s := components.NewSurface(w, h, p)
	fill := xui.Style{Fg: th.Foreground.Fg}
	for row := 0; row < h; row++ {
		for col := 0; col < w; col++ {
			s.SetCell(col, row, xui.Cell{Char: " ", Width: 1, Style: fill})
		}
	}

	method := ctx.Method
	y := 0
	s.Print(1, y, layout.TruncateToWidth("Usage — subscription and session", w-2, method), th.Warning, method)
	y++
	s.Print(
		1,
		y,
		layout.TruncateToWidth("Esc close · r refresh · ↑↓/PgUp/PgDn/Home/End scroll", w-2, method),
		th.Muted,
		method,
	)

	// Measure with the same renderer so optional rows cannot drift from the
	// scroll bounds. Only the viewport gets a buffer, even for large reports.
	p.contentHeight = p.drawReport(components.Surface{}, th, method, w, 0)
	footerHeight := 0
	if p.onReset != nil {
		footerHeight = 3
	}
	p.height = max(0, h-3-footerHeight)
	p.clampScroll()
	body := components.Surface{
		Size:   components.Size{Width: w, Height: p.height},
		Buffer: s.Buffer[min(3, h)*w : min(3+p.height, h)*w],
	}
	p.drawReport(body, th, method, w, -p.scroll)
	p.drawReset(s, ctx.Method, w, h)
	return s
}

// Report renders all report rows without a header or viewport state. Embedding
// dashboards own their scrolling and must not capture a clipped viewport.
func (p *Pane) Report(ctx components.DrawContext) components.Surface {
	w := max(1, ctx.Max.Width)
	h := p.drawReport(components.Surface{}, p.theme, ctx.Method, w, 0)
	s := components.NewSurface(w, h, p)
	p.drawReport(s, p.theme, ctx.Method, w, 0)
	return s
}

func (p *Pane) drawReport(s components.Surface, th components.Theme, method xui.WidthMethod, w, y int) int {
	y = p.drawSubscription(s, th, method, w, y)
	return p.drawSession(s, th, method, w, y+1)
}

// drawSubscription renders the quota section according to the fetch state.
func (p *Pane) drawSubscription(s components.Surface, th components.Theme, method xui.WidthMethod, w, y int) int {
	s.Print(1, y, "Subscription", th.Foreground, method)
	y++
	switch {
	case p.loading:
		s.Print(1, y, "  fetching subscription usage…", th.Muted, method)
		return y + 1
	case p.quota.Unsupported:
		s.Print(1, y, fmt.Sprintf("  %s has no subscription endpoint yet", p.providerLabel()), th.Muted, method)
		return y + 1
	case p.quota.Err != nil:
		// The manager scrubs credentials from quota errors; still truncate,
		// because transport error text can be long.
		s.Print(1, y, layout.TruncateToWidth("  "+p.quota.Err.Error(), w-2, method), th.Destructive, method)
		return y + 1
	}
	if p.quota.Snapshot.PlanName != "" {
		s.Print(1, y, "  plan  "+p.quota.Snapshot.PlanName, th.Foreground, method)
		y++
	}
	for _, limit := range p.quota.Snapshot.Limits {
		usageBar := bar(limit)
		if usageBar == "" {
			continue
		}
		label := fmt.Sprintf("  %-7s %s  %s", limit.Window, usageBar, limitText(limit))
		s.Print(1, y, layout.TruncateToWidth(label, w-2, method), th.Foreground, method)
		y++
		reset := "  reset time unavailable"
		if !limit.ResetsAt.IsZero() {
			reset = "  resets " + tokens.FormatReset(limit.ResetsAt)
		}
		s.Print(1, y, layout.TruncateToWidth(reset, w-2, method), th.Muted, method)
		y++
	}
	if len(p.quota.Snapshot.Limits) == 0 {
		s.Print(1, y, "  rate-limit data unavailable", th.Muted, method)
		y++
	}
	if p.quota.Snapshot.Reset.Supported {
		label := fmt.Sprintf("  limit resets  %d available", p.quota.Snapshot.Reset.Available)
		s.Print(1, y, layout.TruncateToWidth(label, w-2, method), th.Foreground, method)
		y++
	}
	if p.quota.Snapshot.Reset.Note != "" {
		s.Print(
			1,
			y,
			layout.TruncateToWidth("  reset action: "+p.quota.Snapshot.Reset.Note, w-2, method),
			th.Muted,
			method,
		)
		y++
	}
	return y
}

// drawSession renders cumulative session totals; the section stands alone, so
// it stays informative even when the provider has no quota endpoint.
func (p *Pane) drawSession(s components.Surface, th components.Theme, method xui.WidthMethod, w, y int) int {
	s.Print(1, y, "Session", th.Foreground, method)
	y++
	if p.session.Model != "" {
		s.Print(1, y, "  model  "+p.session.Model, th.Muted, method)
		y++
	}
	stats := fmt.Sprintf(
		"  rounds %d  ·  in %s  out %s  cache %s  total %s",
		p.session.Rounds, tokens.FormatTokens(int(p.session.InputTokens)),
		tokens.FormatTokens(int(p.session.OutputTokens)), tokens.FormatTokens(int(p.session.CachedTokens)),
		tokens.FormatTokens(int(p.session.TotalTokens)),
	)
	s.Print(1, y, layout.TruncateToWidth(stats, w-2, method), th.Foreground, method)
	y++

	contextLine := "  context " + tokens.FormatTokens(p.session.ContextTokens)
	if p.session.ContextWindow > 0 {
		fill := float64(p.session.ContextTokens) / float64(p.session.ContextWindow)
		contextLine += fmt.Sprintf(" / %s (%d%%)", tokens.FormatTokens(p.session.ContextWindow), int(fill*100))
	}
	s.Print(1, y, layout.TruncateToWidth(contextLine, w-2, method), th.Foreground, method)
	y++

	if !p.session.StartedAt.IsZero() {
		s.Print(1, y, "  wall "+time.Since(p.session.StartedAt).Round(time.Minute).String(), th.Muted, method)
		y++
	}
	return y
}

// providerLabel names the provider the quota section is about; it falls back
// to a placeholder when the controller has not named one yet.
func (p *Pane) providerLabel() string {
	if id := p.quota.ProviderID; id != "" {
		return id
	}
	return "this provider"
}

func limitText(limit provider.QuotaLimit) string {
	if limit.Unit == "percent" {
		return fmt.Sprintf("%.0f%% used · %.0f%% remaining", limit.UsedPercent, max(0, 100-limit.UsedPercent))
	}
	used := tokens.FormatTokens(int(limit.Used))
	total := tokens.FormatTokens(int(limit.Total))
	if limit.Unit == "credits" {
		return fmt.Sprintf("%d / %d credits", limit.Used, limit.Total)
	}
	return fmt.Sprintf("%s / %s", used, total)
}

// bar renders a fixed-width used/total bar in two segments — filled then
// empty — so one bar reads at a glance; the caller colors the whole row.
func bar(limit provider.QuotaLimit) string {
	ratio := 0.0
	if limit.Unit == "percent" {
		ratio = limit.UsedPercent / 100
	} else {
		total := limit.Total
		if total <= 0 {
			total = limit.Used + limit.Remaining
		}
		if total > 0 {
			ratio = float64(limit.Used) / float64(total)
		}
	}
	if ratio < 0.01 {
		return ""
	}
	// max keeps a rounding overshoot from spilling past the width.
	filled := max(0, min(int(ratio*float64(barWidth)), barWidth))
	full := strings.Repeat("█", filled)
	empty := strings.Repeat("░", barWidth-filled)
	return full + empty
}
