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
	// onClose fires once whenever the pane stops being visible, so the
	// shell can hand the keyboard back to the composer.
	onClose func()

	session controller.SessionStats
	quota   controller.UsageQuotaMsg
	// loading covers the gap between Show/refresh and the UsageQuotaMsg.
	loading bool
	visible bool
}

// New builds a hidden pane. Every side effect goes back through these seams.
func New(
	theme components.Theme,
	sessionStats func() controller.SessionStats,
	onRefresh func(),
	onClose func(),
) *Pane {
	return &Pane{
		theme:        theme,
		sessionStats: sessionStats,
		onRefresh:    onRefresh,
		onClose:      onClose,
	}
}

// Show pulls the session snapshot, kicks off a quota fetch and opens the pane.
func (p *Pane) Show() {
	p.pullSession()
	p.quota = controller.UsageQuotaMsg{}
	p.loading = true
	p.visible = true
	if p.onRefresh != nil {
		p.onRefresh()
	}
}

// Hide closes the pane and hands the keyboard back to the shell.
func (p *Pane) Hide() {
	if !p.visible {
		return
	}
	p.visible = false
	if p.onClose != nil {
		p.onClose()
	}
}

// Visible reports whether the pane covers the screen.
func (p *Pane) Visible() bool { return p != nil && p.visible }

// Apply takes the latest quota fetch result. An empty ProviderID means the
// fetch belongs to a pane that closed meanwhile — ignored, not rendered.
func (p *Pane) Apply(msg controller.UsageQuotaMsg) {
	if msg.ProviderID == "" {
		return
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
	case xui.KeyEvent:
		if !e.Press {
			return true
		}
		p.handleKey(e)
		ctx.ConsumeAndRedraw()
		return true
	case xui.MouseEvent:
		// The pane covers the screen, so every click stays here; the short
		// content has nothing to scroll.
		ctx.ConsumeAndRedraw()
		return true
	default:
		return false
	}
}

func (p *Pane) handleKey(e xui.KeyEvent) {
	switch e.Code {
	case xui.KeyEscape:
		p.Hide()
	case xui.KeyRune:
		if e.Rune == 'r' {
			p.pullSession()
			p.loading = true
			if p.onRefresh != nil {
				p.onRefresh()
			}
		}
	}
}

func (p *Pane) pullSession() {
	if p.sessionStats != nil {
		p.session = p.sessionStats()
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
	s.Print(1, y, layout.TruncateToWidth("Esc close · r refresh", w-2, method), th.Muted, method)
	y++

	y = p.drawSubscription(s, th, method, w, y+1)
	p.drawSession(s, th, method, w, y+1)
	return s
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
		used := tokens.FormatTokens(int(limit.Used))
		total := tokens.FormatTokens(int(limit.Total))
		label := fmt.Sprintf("  %-7s %s  %s / %s", limit.Window, bar(limit), used, total)
		if !limit.ResetsAt.IsZero() {
			label += "  · resets " + formatReset(limit.ResetsAt)
		}
		s.Print(1, y, layout.TruncateToWidth(label, w-2, method), th.Foreground, method)
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

// bar renders a fixed-width used/total bar in two segments — filled then
// empty — so one bar reads at a glance; the caller colors the whole row.
func bar(limit provider.QuotaLimit) string {
	total := limit.Total
	if total <= 0 {
		total = limit.Used + limit.Remaining
	}
	ratio := 0.0
	if total > 0 {
		ratio = float64(limit.Used) / float64(total)
	}
	filled := 0
	if total > 0 {
		// max keeps a rounding overshoot from spilling past the width.
		filled = max(0, min(int(ratio*float64(barWidth)), barWidth))
	}
	full := strings.Repeat("█", filled)
	empty := strings.Repeat("░", barWidth-filled)
	return full + empty
}

// formatReset keeps reset timestamps short: relative for the near future,
// absolute once it is more than a day out.
func formatReset(at time.Time) string {
	until := time.Until(at).Round(time.Minute)
	if until > 0 && until < 24*time.Hour {
		return "in " + until.String()
	}
	return at.Format("Mon 2 Jan 15:04")
}
