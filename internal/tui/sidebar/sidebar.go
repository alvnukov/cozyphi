// Package sidebar renders the resizable right-hand runtime and plan panel.
// Runtime state stays fixed at the top; only the plan viewport scrolls.
package sidebar

import (
	"math"
	"strconv"
	"strings"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/layout"
	"github.com/pulseaiclub/phi/internal/mcp"
	"github.com/pulseaiclub/phi/internal/session"
	"github.com/pulseaiclub/phi/internal/tui/tokens"
)

const (
	// Width is the default panel width. The live width is user-resizable.
	Width    = 30
	minWidth = 24
	maxWidth = 60

	// minChatWidth keeps the transcript readable; on narrower terminals the
	// panel is suppressed even while toggled on.
	minChatWidth = 80
	barWidth     = 20

	// panelPad is the blank ring kept between the frame and panel text, so
	// blocks breathe instead of touching the border glyphs.
	panelPad = 1
)

// Runtime is the fixed status area above the plan viewport.
type Runtime struct {
	Model    string
	Mode     string
	Activity string
	MCP      []mcp.ServerStatus
}

// Sidebar owns panel-local presentation state. It is mutated and rendered on
// the UI goroutine; producers publish snapshots through controller.Bus.
type Sidebar struct {
	theme              components.Theme
	contextWindow      int
	visible            bool
	width              int
	runtime            Runtime
	usage              session.TokenUsage
	plan               session.Plan
	planScroll         int
	planTop            int
	planHeight         int
	planLines          int
	focusActive        bool
	resizing           bool
	widthChanged       bool
	onWidthCommit      func(int) error
	onVisibilityCommit func(bool) error
}

// NewSidebar builds a hidden panel; Toggle or Ctrl+O shows it.
func NewSidebar(theme components.Theme, contextWindow int) *Sidebar {
	return &Sidebar{theme: theme, contextWindow: contextWindow, width: Width}
}

// ConfigureWidth restores a preferred width and optionally persists future
// drag commits. Callback errors are returned to the editor instead of ignored.
func (s *Sidebar) ConfigureWidth(width int, onCommit func(int) error) {
	if s == nil {
		return
	}
	s.width = clampWidth(width)
	s.onWidthCommit = onCommit
}

// ConfigureVisibility restores visibility and persists future keyboard toggles.
func (s *Sidebar) ConfigureVisibility(visible bool, onCommit func(bool) error) {
	if s == nil {
		return
	}
	s.visible = visible
	s.onVisibilityCommit = onCommit
}

// CurrentWidth returns the live panel width.
func (s *Sidebar) CurrentWidth() int {
	if s == nil || s.width == 0 {
		return Width
	}
	return s.width
}

// Toggle flips panel visibility.
func (s *Sidebar) Toggle() {
	if s != nil {
		s.visible = !s.visible
	}
}

// Visible reports whether the panel is toggled on.
func (s *Sidebar) Visible() bool { return s != nil && s.visible }

// PointerShape marks the left border as a horizontal resize handle.
func (*Sidebar) PointerShape(x, _ int) string {
	if x == 0 {
		return components.ShapeResizeEW
	}
	return ""
}

// ReserveWidth reports how many columns the editor should reserve.
func (s *Sidebar) ReserveWidth(total int) int {
	width := s.CurrentWidth()
	if !s.Visible() || total-minChatWidth < width {
		return 0
	}
	return width
}

// HandleToggleKey consumes Ctrl+O toggle presses and returns persistence
// failures so the editor can surface them without undoing the responsive UI.
func (s *Sidebar) HandleToggleKey(ctx *components.EventContext, ev xui.KeyEvent) (bool, error) {
	if s == nil || !ev.Press || !ev.Mods.Has(xui.ModCtrl) || ev.Code != xui.KeyRune {
		return false, nil
	}
	if ev.Rune != 'o' && ev.Rune != 'O' {
		return false, nil
	}
	s.Toggle()
	ctx.ConsumeAndRedraw()
	if s.onVisibilityCommit == nil {
		return true, nil
	}
	return true, s.onVisibilityCommit(s.visible)
}

// HandleScrollKey moves only the plan viewport while the panel is visible.
// Ctrl+Up/Down moves one row; Ctrl+PageUp/PageDown moves one viewport.
func (s *Sidebar) HandleScrollKey(ctx *components.EventContext, ev xui.KeyEvent) bool {
	if s == nil || !s.Visible() || !ev.Press || !ev.Mods.Has(xui.ModCtrl) || s.planHeight <= 0 {
		return false
	}
	step := 1
	switch ev.Code {
	case xui.KeyPageUp, xui.KeyPageDown:
		step = max(s.planHeight-1, 1)
	case xui.KeyUp, xui.KeyDown:
	default:
		return false
	}
	if ev.Code == xui.KeyUp || ev.Code == xui.KeyPageUp {
		s.planScroll -= step
	} else {
		s.planScroll += step
	}
	s.clampPlanScroll()
	ctx.ConsumeAndRedraw()
	return true
}

// Handle consumes local sidebar mouse events. Wheel input only changes the
// plan viewport; pressing the left border starts a global resize drag.
func (s *Sidebar) Handle(ctx *components.EventContext, ev xui.Event) {
	if s == nil {
		return
	}
	mouse, ok := ev.(xui.MouseEvent)
	if !ok {
		return
	}
	if mouse.Action == xui.MousePress && mouse.Button == xui.MouseLeft && mouse.X == 0 {
		s.resizing = true
		s.widthChanged = false
		ctx.ConsumeAndRedraw()
		return
	}
	if mouse.Button != xui.MouseWheelUp && mouse.Button != xui.MouseWheelDown {
		return
	}
	ctx.Consume = true
	if mouse.Y < s.planTop || mouse.Y >= s.planTop+s.planHeight || s.planHeight <= 0 {
		return
	}
	step := max(mouse.Wheel, 1) * 3
	if mouse.Button == xui.MouseWheelUp {
		s.planScroll -= step
	} else {
		s.planScroll += step
	}
	s.clampPlanScroll()
	ctx.Redraw = true
}

// HandleGlobalMouse continues a resize after the pointer leaves the sidebar.
// It expects terminal-absolute coordinates and returns any persistence error
// on release so the editor can surface it.
func (s *Sidebar) HandleGlobalMouse(ctx *components.EventContext, ev xui.MouseEvent, totalWidth int) (bool, error) {
	if s == nil || !s.resizing {
		return false, nil
	}
	if ev.Action != xui.MouseDrag && ev.Action != xui.MouseMotion && ev.Action != xui.MouseRelease {
		return false, nil
	}
	width := clampWidth(totalWidth - ev.X)
	width = min(width, max(totalWidth-minChatWidth, minWidth))
	if width != s.width {
		s.width = width
		s.widthChanged = true
	}
	ctx.ConsumeAndRedraw()
	if ev.Action != xui.MouseRelease {
		return true, nil
	}
	s.resizing = false
	if !s.widthChanged || s.onWidthCommit == nil {
		return true, nil
	}
	s.widthChanged = false
	return true, s.onWidthCommit(s.width)
}

// SetRuntime replaces the fixed runtime snapshot.
func (s *Sidebar) SetRuntime(runtime Runtime) {
	if s == nil {
		return
	}
	runtime.MCP = append([]mcp.ServerStatus(nil), runtime.MCP...)
	s.runtime = runtime
}

// SetServers is retained for simple callers and tests; configured is the only
// truthful state until the first connection attempt.
func (s *Sidebar) SetServers(names []string) {
	statuses := make([]mcp.ServerStatus, len(names))
	for i, name := range names {
		statuses[i] = mcp.ServerStatus{Name: name, State: mcp.StateConfigured}
	}
	runtime := s.runtime
	runtime.MCP = statuses
	s.SetRuntime(runtime)
}

// SetPlan replaces the durable plan snapshot and resets its viewport only
// when a different revision arrives.
func (s *Sidebar) SetPlan(plan session.Plan) {
	if s == nil {
		return
	}
	if s.plan.Revision != plan.Revision {
		s.planScroll = 0
		s.focusActive = true
	}
	s.plan = plan.Clone()
}

// UpdateUsage replaces the current token usage snapshot.
func (s *Sidebar) UpdateUsage(u session.TokenUsage) {
	if s == nil || !u.Reported() {
		return
	}
	s.usage = u
}

// ClearUsage drops the current usage snapshot (e.g. after /clear).
func (s *Sidebar) ClearUsage() {
	if s != nil {
		s.usage = session.TokenUsage{}
	}
}

// SetTheme updates panel styling.
func (s *Sidebar) SetTheme(th components.Theme) {
	if s != nil {
		s.theme = th
	}
}

type panelLine struct {
	text  string
	style xui.Style
}

// Draw renders fixed runtime rows followed by an independently clipped plan.
func (s *Sidebar) Draw(ctx components.DrawContext) components.Surface {
	if s == nil {
		return components.Surface{}
	}
	height := ctx.Max.Height
	width := s.CurrentWidth()
	s.planTop = 0
	s.planHeight = 0
	s.planLines = 0
	surf := components.NewSurface(width, height, s)
	layout.DrawRoundedBorder(
		&surf,
		layout.BorderRounded,
		s.theme.Border,
		&layout.BorderLabel{Text: "session", Style: s.theme.Muted},
		&layout.BorderLabel{Text: "Ctrl+O hide", Style: s.theme.Muted},
		nil,
		nil,
		ctx.Method,
	)
	if height <= 2 {
		return surf
	}

	y := 1 + panelPad
	for _, line := range s.runtimeLines() {
		if y >= height-1 {
			return surf
		}
		printPanelLine(&surf, width, y, line, ctx.Method)
		y++
	}
	if y >= height-1 {
		return surf
	}

	completed := 0
	for _, item := range s.plan.Items {
		if item.Status == session.PlanCompleted || item.Status == session.PlanCancelled {
			completed++
		}
	}
	title := "plan"
	if len(s.plan.Items) > 0 {
		title += " " + strconv.Itoa(completed) + "/" + strconv.Itoa(len(s.plan.Items))
	}
	printPanelLine(&surf, width, y, panelLine{text: title, style: s.theme.Muted}, ctx.Method)
	y++

	s.planTop = y
	s.planHeight = max(height-1-y, 0)
	lines, activeLine := s.planContent(contentWidth(width), ctx.Method)
	s.planLines = len(lines)
	if s.focusActive && activeLine >= 0 && s.planHeight > 0 {
		if activeLine < s.planScroll {
			s.planScroll = activeLine
		} else if activeLine >= s.planScroll+s.planHeight {
			s.planScroll = activeLine - s.planHeight + 1
		}
		s.focusActive = false
	}
	s.clampPlanScroll()
	for row := 0; row < s.planHeight && row+s.planScroll < len(lines); row++ {
		printPanelLine(&surf, width, y+row, lines[row+s.planScroll], ctx.Method)
	}
	if len(lines) > s.planHeight && s.planHeight > 0 {
		// The thumb lives in the right gutter, keeping the frame intact.
		thumb := min(s.planHeight-1, s.planScroll*s.planHeight/max(len(lines), 1))
		surf.Print(width-1-panelPad, y+thumb, "│", s.theme.ToolName, ctx.Method)
	}
	return surf
}

func (s *Sidebar) runtimeLines() []panelLine {
	header := func(name string) panelLine { return panelLine{text: name, style: s.theme.Muted} }
	lines := []panelLine{header("context")}
	used := s.usage.ContextTokens()
	if used > 0 && s.contextWindow > 0 {
		ratio := tokens.ContextFillRatio(used, s.contextWindow)
		width := min(barWidth, max(s.CurrentWidth()-8-2*panelPad, 4))
		filled := min(max(int(math.Round(ratio*float64(width))), 0), width)
		pct := min(max(int(ratio*100), 0), 100)
		bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
		style := tokens.FillStyle(s.theme, tokens.ContextFillLevelFor(ratio, s.contextWindow))
		lines = append(
			lines,
			panelLine{text: bar + " " + strconv.Itoa(pct) + "%", style: style},
			panelLine{
				text:  tokens.FormatTokens(used) + "/" + tokens.FormatTokens(s.contextWindow),
				style: s.theme.Muted,
			},
		)
	} else {
		lines = append(lines, panelLine{text: "awaiting usage", style: s.theme.Muted})
	}
	if s.runtime.Model != "" {
		lines = append(lines, panelLine{text: "model  " + s.runtime.Model, style: s.theme.Foreground})
	}
	if s.runtime.Mode != "" {
		lines = append(lines, panelLine{text: "mode   " + s.runtime.Mode, style: s.theme.Foreground})
	}
	if s.runtime.Activity != "" {
		lines = append(lines, panelLine{text: "state  " + s.runtime.Activity, style: s.theme.ToolName})
	}

	lines = append(lines, panelLine{}, header("tokens"))
	if row := tokens.FormatUsageStats(s.usage); row != "" {
		lines = append(lines, panelLine{text: row, style: s.theme.Foreground})
	}

	lines = append(lines, panelLine{}, header("mcp"))
	if len(s.runtime.MCP) == 0 {
		return append(lines, panelLine{text: "none", style: s.theme.Muted}, panelLine{})
	}
	for _, status := range s.runtime.MCP {
		marker, style := mcpMarker(status.State, s.theme)
		lines = append(lines, panelLine{text: marker + " " + status.Name, style: style})
	}
	return append(lines, panelLine{})
}

func (s *Sidebar) planContent(width int, method xui.WidthMethod) ([]panelLine, int) {
	if len(s.plan.Items) == 0 {
		return []panelLine{{text: "No plan yet", style: s.theme.Muted}}, -1
	}
	var out []panelLine
	appendWrapped := func(value, prefix string, style xui.Style) {
		wrapped := components.WrapSpans(
			[]components.Span{{Text: value, Style: style}},
			max(width-len([]rune(prefix)), 1),
			method,
		)
		if len(wrapped) == 0 {
			wrapped = []components.RichLine{nil}
		}
		continuation := strings.Repeat(" ", len([]rune(prefix)))
		for i, rich := range wrapped {
			var text strings.Builder
			for _, span := range rich {
				text.WriteString(span.Text)
			}
			linePrefix := continuation
			if i == 0 {
				linePrefix = prefix
			}
			out = append(out, panelLine{text: linePrefix + text.String(), style: style})
		}
	}

	activeLine := -1
	for _, item := range s.plan.Items {
		if item.Status == session.PlanInProgress {
			activeLine = len(out)
		}
		marker, style := planMarker(item.Status, s.theme)
		appendWrapped(item.Content, marker+" ", style)
		if item.Note != "" {
			appendWrapped(item.Note, "  ", s.theme.Muted)
		}
		if item.Status == session.PlanCompleted && item.Evidence != "" {
			appendWrapped(item.Evidence, "  ✓ ", s.theme.Muted)
		}
	}
	return out, activeLine
}

func (s *Sidebar) clampPlanScroll() {
	maxScroll := max(s.planLines-s.planHeight, 0)
	s.planScroll = min(max(s.planScroll, 0), maxScroll)
}

// contentWidth is the printable column count inside the frame and gutters.
func contentWidth(panelWidth int) int {
	return max(panelWidth-2-2*panelPad, 1)
}

func printPanelLine(surf *components.Surface, width, y int, line panelLine, method xui.WidthMethod) {
	text := layout.TruncateToWidth(line.text, contentWidth(width), method)
	if text != "" {
		surf.Print(1+panelPad, y, text, line.style, method)
	}
}

func planMarker(status session.PlanStatus, theme components.Theme) (string, xui.Style) {
	switch status {
	case session.PlanInProgress:
		return "●", theme.ToolName
	case session.PlanBlocked:
		return "!", theme.Warning
	case session.PlanCompleted:
		return "✓", theme.Muted
	case session.PlanCancelled:
		return "–", theme.Muted
	default:
		return "○", theme.Foreground
	}
}

func mcpMarker(state mcp.ConnectionState, theme components.Theme) (string, xui.Style) {
	switch state {
	case mcp.StateConnected:
		return "●", theme.Success
	case mcp.StateFailed:
		return "×", theme.Destructive
	default:
		return "○", theme.Muted
	}
}

func clampWidth(width int) int {
	if width == 0 {
		return Width
	}
	return min(max(width, minWidth), maxWidth)
}
