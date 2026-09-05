// Package statuspane renders a dashboard from display-safe snapshots. Loading and
// persistence belong to the shell; Draw never reads disk or starts work.
package statuspane

import (
	"fmt"
	"strings"
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/layout"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/settings"
	"github.com/alvnukov/cozyphi/internal/tui/usagepane"
)

// Stable tab values are the persistence contract.
const (
	Status = "status"
	Config = "config"
	Usage  = "usage"
	Stats  = "stats"
)

var tabs = []string{Status, Config, Usage, Stats}

// Snapshot deliberately excludes credentials, URLs, raw config and MCP errors.
type Snapshot struct {
	Session, Version, CWD, Model, Provider string
	MCP, ConfigSources                     []string
}

// Totals describes recorded history, not estimates of billing.
type Totals struct {
	Sessions, Rounds             int
	Input, Output, Cached, Total int64
}

type Day struct {
	Date   time.Time
	Totals Totals
}

type Model struct {
	Name   string
	Totals Totals
}

// History is a detached presentation snapshot. Unavailable must explain missing data.
type History struct {
	Totals      Totals
	Days        []Day
	Models      []Model
	Unavailable string
	// Warnings must be display-safe summaries, never raw journal lines.
	Warnings                                  []string
	UnknownUsage, UnknownModels, UnknownDates int
	Partial                                   bool
}

// Pane is UI-goroutine confined. Config embeds the existing settings editor.
type Pane struct {
	theme    components.Theme
	config   *settings.Pane
	usage    *usagepane.Pane
	snapshot Snapshot
	history  History
	visible  bool
	tab      string

	scroll, height, contentHeight, period int
	models                                bool
	choose                                func() string
	closed                                func(string)
	onClose                               func()
	historyRequest                        func(int)
	provider                              string
	usageStale                            bool
}

func New(
	theme components.Theme,
	config *settings.Pane,
	session func() controller.SessionStats,
	refresh, close func(),
) *Pane {
	p := &Pane{
		theme: theme, config: config, tab: Usage, onClose: close,
		history: History{Unavailable: "History loader unavailable"},
	}
	p.usage = usagepane.New(theme, session, refresh, nil)
	return p
}

// ConfigureTabs supplies preference callbacks; invalid choices fall back to Usage.
func (p *Pane) ConfigureTabs(choose func() string, closed func(string)) {
	p.choose, p.closed = choose, closed
}

// ConfigureHistory requests days=0 (all time), 7 or 30 outside Draw. The shell
// must discard superseded responses before calling ApplyHistory on the UI goroutine.
func (p *Pane) ConfigureHistory(request func(int)) { p.historyRequest = request }

func (p *Pane) ApplyHistory(h History) {
	h.Days = append([]Day(nil), h.Days...)
	h.Models = append([]Model(nil), h.Models...)
	h.Warnings = append([]string(nil), h.Warnings...)
	p.history = h
	p.scroll = 0
}

func (p *Pane) Tab() string   { return p.tab }
func (p *Pane) Visible() bool { return p != nil && p.visible }

func (p *Pane) Show(s Snapshot) {
	s.MCP = append([]string(nil), s.MCP...)
	s.ConfigSources = append([]string(nil), s.ConfigSources...)
	p.snapshot = s
	p.provider = s.Provider
	p.visible = true
	p.scroll = 0
	p.usageStale = false
	p.tab = Usage
	if p.choose != nil {
		chosen := p.choose()
		for _, tab := range tabs {
			if tab == chosen {
				p.tab = tab
				break
			}
		}
	}
	if p.config != nil {
		p.config.Show()
	}
	p.usage.Show()
	p.requestHistory()
}

func (p *Pane) Hide() {
	if !p.Visible() {
		return
	}
	p.visible = false
	if p.config != nil {
		p.config.Hide()
	}
	p.usage.Hide()
	if p.closed != nil {
		p.closed(p.tab)
	}
	if p.onClose != nil {
		p.onClose()
	}
}

// ApplyQuota rejects results from other providers, including results queued before opening.
func (p *Pane) ApplyQuota(m controller.UsageQuotaMsg, currentProvider string) {
	if !p.Visible() {
		return
	}
	if currentProvider != p.provider {
		p.SetModel(p.snapshot.Model, currentProvider)
	}
	if p.usageStale || m.ProviderID == "" {
		return
	}
	if m.ProviderID != p.provider {
		// Refresh may have been coalesced behind this old provider's fetch.
		// Its slot is released before delivery, so retry for the current one.
		p.usage.Show()
		return
	}
	p.usage.Apply(m)
}

func (p *Pane) requestHistory() {
	p.history = History{Unavailable: "History loader unavailable"}
	if p.historyRequest != nil {
		p.history.Unavailable = "Loading history…"
		p.historyRequest([]int{0, 7, 30}[p.period])
	}
}

func (*Pane) Handle(*components.EventContext, xui.Event) {}

func (p *Pane) HandleEvent(ctx *components.EventContext, ev xui.Event) bool {
	if !p.Visible() {
		return false
	}
	ctx.ConsumeAndRedraw()
	if mouse, ok := ev.(xui.MouseEvent); ok {
		if p.tab != Config {
			switch mouse.Button {
			case xui.MouseWheelDown:
				p.scroll += max(1, mouse.Wheel)
			case xui.MouseWheelUp:
				p.scroll -= max(1, mouse.Wheel)
			}
			p.clampScroll()
		}
		if p.tab == Config && p.config != nil && mouse.Y >= 2 {
			mouse.Y -= 2
			p.handleConfig(ctx, mouse)
		}
		return true
	}
	e, ok := ev.(xui.KeyEvent)
	if !ok {
		if p.tab == Config && p.config != nil {
			p.handleConfig(ctx, ev)
		}
		return true
	}
	if !e.Press {
		return true
	}
	// F2/F3 always switch dashboard tabs, leaving settings' Tab and arrows intact.
	arrowTab := p.tab != Config && (e.Code == xui.KeyTab || e.Code == xui.KeyLeft || e.Code == xui.KeyRight)
	if e.Code == xui.KeyF2 || e.Code == xui.KeyF3 || arrowTab {
		delta := 1
		if e.Code == xui.KeyF2 || e.Code == xui.KeyLeft || e.Mods&xui.ModShift != 0 {
			delta = -1
		}
		for i, tab := range tabs {
			if tab == p.tab {
				p.tab = tabs[(i+delta+len(tabs))%len(tabs)]
				break
			}
		}
		p.scroll = 0
		return true
	}
	if p.tab == Config && p.config != nil {
		p.handleConfig(ctx, ev)
		return true
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
		p.handleRune(e.Rune)
	}
	p.clampScroll()
	return true
}

func (p *Pane) handleConfig(ctx *components.EventContext, ev xui.Event) {
	p.config.HandleEvent(ctx, ev)
	if !p.config.Visible() {
		p.Hide()
	}
}

func (p *Pane) handleRune(r rune) {
	switch r {
	case 'j':
		p.scroll++
	case 'k':
		p.scroll--
	case 'p':
		if p.tab == Stats {
			p.period = (p.period + 1) % 3
			p.requestHistory()
			p.scroll = 0
		}
	case 'm':
		if p.tab == Stats {
			p.models = !p.models
			p.scroll = 0
		}
	case 'r':
		if p.tab == Usage {
			p.usageStale = false
			p.usage.Show()
		}
		if p.tab == Stats {
			p.requestHistory()
		}
	}
}

func (p *Pane) clampScroll() {
	p.scroll = max(0, min(p.scroll, max(0, p.contentHeight-p.height)))
}

func (p *Pane) lines() []string {
	if p.tab == Config {
		return []string{"Settings store unavailable"}
	}
	if p.tab == Status {
		s := p.snapshot
		rows := []string{
			"Current session: " + s.Session,
			"Version: " + s.Version,
			"Directory: " + s.CWD,
			"Model: " + s.Model,
			"Provider: " + s.Provider,
			"Account: unavailable (no account identity API)",
			"", "MCP servers",
		}
		if len(s.MCP) == 0 {
			rows = append(rows, "  None configured")
		}
		rows = append(rows, s.MCP...)
		rows = append(rows, "", "Configuration sources")
		if len(s.ConfigSources) == 0 {
			rows = append(rows, "  Unavailable")
		}
		return append(rows, s.ConfigSources...)
	}
	rows := []string{"Period: " + []string{"All time", "7d", "30d"}[p.period] + " · p change · m Overview/Models"}
	if p.history.Unavailable != "" {
		return append(rows, p.history.Unavailable)
	}
	if p.models {
		rows = append(rows, "Models")
		for _, model := range p.history.Models {
			rows = append(rows, model.Name, formatTotals(model.Totals))
		}
	} else {
		rows = append(rows, "Overview", fmt.Sprintf("%d sessions", p.history.Totals.Sessions),
			formatTotals(p.history.Totals))
		rows = append(rows, recordedSummary(p.history)...)
		rows = append(rows, "", "Activity heatmap · . none / ░ light / ▒ medium / ▓ heavy")
		rows = append(rows, heatmap(p.history.Days)...)
	}
	if p.history.Partial {
		rows = append(rows, "Partial history — some observations are unavailable")
	}
	rows = append(rows, fmt.Sprintf("Unknown usage %d · models %d · dates %d",
		p.history.UnknownUsage, p.history.UnknownModels, p.history.UnknownDates))
	rows = append(rows, p.history.Warnings...)
	return append(rows, "Cost: unavailable · cache writes: unavailable")
}

func formatTotals(t Totals) string {
	return fmt.Sprintf("%d rounds · in %d · out %d · cache read %d · total %d",
		t.Rounds, t.Input, t.Output, t.Cached, t.Total)
}

func (p *Pane) Draw(ctx components.DrawContext) components.Surface {
	w, h := max(0, ctx.Max.Width), max(0, ctx.Max.Height)
	s := components.NewSurface(w, h, p)
	for i := range s.Buffer {
		s.Buffer[i] = xui.Cell{Char: " ", Width: 1, Style: p.theme.Foreground}
	}
	if w == 0 || h == 0 {
		return s
	}
	var header []string
	for _, tab := range tabs {
		label := tab
		if tab == p.tab {
			label = "[" + tab + "]"
		}
		header = append(header, label)
	}
	title := strings.Join(header, "  ")
	if w < 32 {
		title = "[" + p.tab + "] F2/F3"
	}
	s.Print(0, 0, layout.TruncateToWidth(title, w, ctx.Method), p.theme.Warning, ctx.Method)
	if h < 2 {
		return s
	}
	hint := "F2/F3 tabs · Esc close · ↑↓/PgUp/PgDn scroll"
	s.Print(0, 1, layout.TruncateToWidth(hint, w, ctx.Method), p.theme.Muted, ctx.Method)
	p.height = max(0, h-2)
	if p.height == 0 {
		return s
	}
	if p.tab == Config && p.config != nil {
		child := p.config.Draw(ctx.WithConstraints(components.Size{}, components.Size{Width: w, Height: p.height}))
		s.Children = append(s.Children, components.SubSurface{Origin: components.Point{Y: 2}, Surface: child})
		return s
	}
	rows := p.lines()
	if p.tab == Usage {
		if p.usageStale {
			rows = []string{"Model/provider changed — r refresh usage"}
		} else {
			rows = p.usageLines(ctx, w)
		}
	}
	rows = wrapLines(rows, w, ctx.Method)
	p.contentHeight = len(rows)
	p.clampScroll()
	for y := 0; y < p.height && y+p.scroll < len(rows); y++ {
		s.Print(0, y+2, rows[y+p.scroll], p.theme.Foreground, ctx.Method)
	}
	return s
}

func (p *Pane) usageLines(ctx components.DrawContext, width int) []string {
	// Render the whole report at a readable width, then reflow its text. A
	// fixed-height viewport would silently discard long profile histories.
	usage := p.usage.Report(ctx.WithConstraints(components.Size{}, components.Size{Width: max(width, 160)}))
	var rows []string
	for y := 0; y < usage.Size.Height; y++ {
		var line strings.Builder
		for x := 0; x < usage.Size.Width; x++ {
			line.WriteString(usage.Buffer[y*usage.Size.Width+x].Char)
		}
		if text := strings.TrimSpace(line.String()); text != "" {
			rows = append(rows, text)
		}
	}
	return append(rows, "Cost: unavailable", "Cache writes: unavailable")
}
