// Package sidebar renders the right-hand status panel: context-window fill,
// per-turn token usage for recent turns, and configured MCP servers.
// Toggled with Ctrl+O, opencode-style.
package sidebar

import (
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/layout"
	"github.com/pulseaiclub/phi/internal/session"
	"github.com/pulseaiclub/phi/internal/tui/tokens"
)

const (
	// Width is the panel width in columns when reserved.
	Width = 30

	// minChatWidth keeps the transcript readable; on narrower terminals the
	// panel is suppressed even while toggled on.
	minChatWidth = 80

	// barWidth is the context fill bar length inside the panel.
	barWidth = 20

	// historyLen caps how many recent turns the tokens section lists.
	historyLen = 5
)

// Sidebar owns the status panel state: visibility, recent token usage, and
// MCP server names. Draw is a pure projection of that state.
type Sidebar struct {
	theme         components.Theme
	contextWindow int
	visible       bool
	servers       []string
	turns         []session.TokenUsage
}

// NewSidebar builds a hidden panel; Toggle or Ctrl+O shows it.
func NewSidebar(theme components.Theme, contextWindow int) *Sidebar {
	return &Sidebar{theme: theme, contextWindow: contextWindow}
}

// Toggle flips panel visibility.
func (s *Sidebar) Toggle() {
	if s != nil {
		s.visible = !s.visible
	}
}

// Visible reports whether the panel is toggled on.
func (s *Sidebar) Visible() bool {
	return s != nil && s.visible
}

// ReserveWidth reports how many right-hand columns the editor should give the
// panel: the full width while visible and the chat keeps minChatWidth columns,
// zero otherwise (hidden, or terminal too narrow).
func (s *Sidebar) ReserveWidth(total int) int {
	if !s.Visible() || total-minChatWidth < Width {
		return 0
	}
	return Width
}

// HandleToggleKey consumes Ctrl+O toggle presses; other keys pass through.
func (s *Sidebar) HandleToggleKey(ctx *components.EventContext, ev xui.KeyEvent) bool {
	if s == nil || !ev.Press || !ev.Mods.Has(xui.ModCtrl) || ev.Code != xui.KeyRune {
		return false
	}
	if ev.Rune != 'o' && ev.Rune != 'O' {
		return false
	}
	s.Toggle()
	ctx.ConsumeAndRedraw()
	return true
}

// SetServers replaces the MCP server name list.
func (s *Sidebar) SetServers(names []string) {
	if s == nil {
		return
	}
	s.servers = append([]string(nil), names...)
}

// UpdateUsage records one completed turn's token usage.
func (s *Sidebar) UpdateUsage(u session.TokenUsage) {
	if s == nil || !u.Reported() {
		return
	}
	s.turns = append(s.turns, u)
	if len(s.turns) > historyLen {
		s.turns = s.turns[len(s.turns)-historyLen:]
	}
}

// ClearUsage drops recorded turns (e.g. after /clear).
func (s *Sidebar) ClearUsage() {
	if s != nil {
		s.turns = nil
	}
}

// SetTheme updates panel styling.
func (s *Sidebar) SetTheme(th components.Theme) {
	if s != nil {
		s.theme = th
	}
}

// panelLine is one row of panel content.
type panelLine struct {
	text  string
	style xui.Style
}

// Draw renders the panel at its fixed width and the given full height.
// Sections clip bottom-up when the panel is short; the context bar survives
// longest.
func (s *Sidebar) Draw(ctx components.DrawContext, height int) components.Surface {
	if s == nil {
		return components.Surface{}
	}
	surf := components.NewSurface(Width, height, nil)
	layout.DrawRoundedBorder(&surf, layout.BorderRounded, s.theme.Border,
		&layout.BorderLabel{Text: "status", Style: s.theme.Muted}, nil, nil, nil, ctx.Method)
	if height <= 2 {
		return surf
	}
	for i, line := range s.contentLines() {
		if i >= height-2 {
			break
		}
		text := layout.TruncateToWidth(line.text, Width-2, ctx.Method)
		if text != "" {
			surf.Print(1, i+1, text, line.style, ctx.Method)
		}
	}
	return surf
}

// contentLines builds panel rows: context fill, recent token turns, MCP
// servers. Headers are muted; later sections clip first.
func (s *Sidebar) contentLines() []panelLine {
	header := func(name string) panelLine { return panelLine{text: name, style: s.theme.Muted} }

	lines := []panelLine{header("context")}
	used := 0
	if n := len(s.turns); n > 0 {
		used = s.turns[n-1].ContextTokens()
	}
	if used > 0 && s.contextWindow > 0 {
		ratio := tokens.ContextFillRatio(used, s.contextWindow)
		filled := min(max(int(math.Round(ratio*float64(barWidth))), 0), barWidth)
		pct := min(max(int(ratio*100), 0), 100)
		bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
		style := tokens.FillStyle(s.theme, tokens.ContextFillLevelFor(ratio, s.contextWindow))
		lines = append(lines,
			panelLine{text: bar + " " + strconv.Itoa(pct) + "%", style: style},
			panelLine{
				text:  tokens.FormatTokens(used) + "/" + tokens.FormatTokens(s.contextWindow),
				style: s.theme.Muted,
			},
		)
	} else {
		lines = append(lines, panelLine{text: "awaiting usage", style: s.theme.Muted})
	}

	lines = append(lines, panelLine{}, header("tokens"))
	for _, turn := range slices.Backward(s.turns) {
		if row := tokens.FormatUsageStats(turn); row != "" {
			lines = append(lines, panelLine{text: row, style: s.theme.Foreground})
		}
	}

	lines = append(lines, panelLine{}, header("mcp"))
	if len(s.servers) == 0 {
		lines = append(lines, panelLine{text: "none", style: s.theme.Muted})
	} else {
		for _, name := range s.servers {
			lines = append(lines, panelLine{text: name, style: s.theme.ToolName})
		}
	}
	return lines
}
