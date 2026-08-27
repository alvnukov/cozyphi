// Package sidebar renders the resizable right-hand runtime and plan panel.
// The Status/Settings tab window and the plan pane render as separate boxes.
package sidebar

import (
	"math"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/layout"
	"github.com/alvnukov/cozyphi/internal/lsp"
	"github.com/alvnukov/cozyphi/internal/mcp"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/tokens"
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
	LSP      []lsp.Language
}

// tabID selects which top block the sidebar shows above the plan.
type tabID int

const (
	tabStatus tabID = iota
	tabSettings
)

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
	approved           bool
	planScroll         int
	planTop            int
	planHeight         int
	planLines          int
	focusActive        bool
	resizing           bool
	widthChanged       bool
	onWidthCommit      func(int) error
	onVisibilityCommit func(bool) error
	onApproveCommit    func(bool) error
	onClearPlan        func() error
	approveRowY        int // -1 when not drawn; hit-test target for the checkbox
	autoApprove        atomic.Bool
	autoRowY           int // -1 when not drawn; hit-test target for the auto checkbox
	autoToggleX        int // x column where the auto checkbox starts on the approval row
	clearToggleX       int // x column where the clear action starts on the approval row
	tab                tabID
	stopOnLimit        bool
	tabRowY            int
	statusTabMinX      int
	statusTabMaxX      int
	settingsTabMinX    int
	settingsTabMaxX    int
	stopRowY           int
	onStopCommit       func(bool) error
}

// NewSidebar builds a hidden panel; Toggle or Ctrl+O shows it.
func NewSidebar(theme components.Theme, contextWindow int) *Sidebar {
	return &Sidebar{
		theme:         theme,
		contextWindow: contextWindow,
		width:         Width,
		tab:           tabStatus,
		stopOnLimit:   true,
		tabRowY:       -1,
	}
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

// ConfigureApprove binds the approval toggle to a persist callback.
func (s *Sidebar) ConfigureApprove(onCommit func(bool) error) {
	if s == nil {
		return
	}
	s.onApproveCommit = onCommit
}

// ConfigureClearPlan binds the plan-pane clear action to a callback that drops
// the durable plan and resets its revision counter.
func (s *Sidebar) ConfigureClearPlan(onClear func() error) {
	if s == nil {
		return
	}
	s.onClearPlan = onClear
}

// CurrentWidth returns the live panel width.
func (s *Sidebar) CurrentWidth() int {
	if s == nil || s.width == 0 {
		return Width
	}
	return s.width
}

func (s *Sidebar) toggleApproved(ctx *components.EventContext) error {
	if s == nil {
		return nil
	}
	next := !s.approved
	if s.onApproveCommit != nil {
		if err := s.onApproveCommit(next); err != nil {
			return err
		}
	}
	s.approved = next
	s.plan.Approved = next
	ctx.ConsumeAndRedraw()
	return nil
}

// toggleAuto flips the auto-approve flag. It only changes how the next plan
// request is received — the durable plan itself is untouched.
func (s *Sidebar) toggleAuto(ctx *components.EventContext) {
	if s == nil {
		return
	}
	s.autoApprove.Store(!s.autoApprove.Load())
	ctx.ConsumeAndRedraw()
}

// clearPlan invokes the bound clear callback, dropping the durable plan and
// resetting its revision counter. Local state follows the callback, not a
// guessed snapshot.
func (s *Sidebar) clearPlan(ctx *components.EventContext) error {
	if s == nil {
		return nil
	}
	if s.onClearPlan != nil {
		if err := s.onClearPlan(); err != nil {
			return err
		}
	}
	ctx.ConsumeAndRedraw()
	return nil
}

// AutoApprove reports whether incoming plans are approved automatically.
func (s *Sidebar) AutoApprove() bool { return s != nil && s.autoApprove.Load() }

// Approved reports the local approval state; it is authoritative only after a
// successful commit (the durable plan update is the source of truth).
func (s *Sidebar) Approved() bool { return s != nil && s.approved }

// StopOnLimit reports whether the tool-round stop is enabled in the panel.
func (s *Sidebar) StopOnLimit() bool { return s != nil && s.stopOnLimit }

// ConfigureStopOnLimit restores the panel's stop toggle and binds its persist
// callback (the editor passes the controller's engine setter).
func (s *Sidebar) ConfigureStopOnLimit(enabled bool, onCommit func(bool) error) {
	if s == nil {
		return
	}
	s.stopOnLimit = enabled
	s.onStopCommit = onCommit
}

// toggleStop flips the tool-round stop toggle and persists the choice.
func (s *Sidebar) toggleStop(ctx *components.EventContext) error {
	if s == nil {
		return nil
	}
	next := !s.stopOnLimit
	if s.onStopCommit != nil {
		if err := s.onStopCommit(next); err != nil {
			return err
		}
	}
	s.stopOnLimit = next
	ctx.ConsumeAndRedraw()
	return nil
}

// setTab switches which top block the panel shows.
func (s *Sidebar) setTab(tab tabID) {
	if s != nil {
		s.tab = tab
	}
}

// HandleApproveKey consumes Ctrl+A and toggles the plan approval checkbox,
// returning any persistence failure so the editor can surface it.
func (s *Sidebar) HandleApproveKey(ctx *components.EventContext, ev xui.KeyEvent) (bool, error) {
	if s == nil || !ev.Press || !ev.Mods.Has(xui.ModCtrl) || ev.Code != xui.KeyRune {
		return false, nil
	}
	if ev.HotkeyRune() != 'a' && ev.HotkeyRune() != 'A' {
		return false, nil
	}
	return true, s.toggleApproved(ctx)
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
	if ev.HotkeyRune() != 'o' && ev.HotkeyRune() != 'O' {
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
	if mouse.Action == xui.MousePress && mouse.Button == xui.MouseLeft && mouse.Y == s.tabRowY && mouse.X > 0 &&
		mouse.X < s.CurrentWidth() {
		switch {
		case mouse.X >= s.statusTabMinX && mouse.X < s.statusTabMaxX:
			s.setTab(tabStatus)
		case mouse.X >= s.settingsTabMinX && mouse.X < s.settingsTabMaxX:
			s.setTab(tabSettings)
		default:
			return
		}
		ctx.ConsumeAndRedraw()
		return
	}
	if mouse.Action == xui.MousePress && mouse.Button == xui.MouseLeft {
		if mouse.Y == s.approveRowY && mouse.X > 0 && mouse.X < s.CurrentWidth() {
			if s.autoRowY == s.approveRowY && mouse.X >= s.clearToggleX {
				_ = s.clearPlan(ctx)
			} else if s.autoRowY == s.approveRowY && mouse.X >= s.autoToggleX {
				s.toggleAuto(ctx)
			} else {
				_ = s.toggleApproved(ctx) // click path has no toast; the key path surfaces errors
			}
			return
		}
		if s.tab == tabSettings && mouse.Y == s.stopRowY && mouse.X > 0 && mouse.X < s.CurrentWidth() {
			_ = s.toggleStop(ctx)
			return
		}
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
	runtime.LSP = append([]lsp.Language(nil), runtime.LSP...)
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
	changed := s.plan.Revision != plan.Revision
	if changed {
		s.planScroll = 0
		s.focusActive = true
	}
	s.plan = plan.Clone()
	s.approved = plan.Approved
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

// Draw renders the Status/Settings tab window above an independently clipped
// plan pane. The tab window reserves every row the runtime snapshot needs
// regardless of the active tab, so switching tabs never moves or resizes the
// plan below the divider.
func (s *Sidebar) Draw(ctx components.DrawContext) components.Surface {
	if s == nil {
		return components.Surface{}
	}
	height := ctx.Max.Height
	width := s.CurrentWidth()
	s.planTop = 0
	s.planHeight = 0
	s.planLines = 0
	s.approveRowY = -1
	s.autoRowY = -1
	s.stopRowY = -1
	s.tabRowY = -1
	s.clearToggleX = 0
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

	// The tab block owns the tabs row plus every row the runtime status would
	// fill. Sizing it from the runtime snapshot (not the active tab) keeps the
	// plan pane pinned in place.
	rows := len(s.runtimeLines())
	s.drawTabs(&surf, 2, ctx.Method)
	bottom := min(rows+2, height-2)
	if s.tab == tabSettings {
		s.drawSettings(&surf, width, 3, bottom, ctx.Method)
	} else {
		y := 3
		for _, line := range s.runtimeLines() {
			if y > bottom {
				break
			}
			printPanelLine(&surf, width, y, line, ctx.Method)
			y++
		}
	}

	// One blank row separates the tab window from the plan pane so the two
	// boxes read as windows of their own, not one continuous panel.
	divider := rows + 4
	if divider > height-2 {
		return surf
	}
	s.drawPlanDivider(&surf, divider, width, ctx.Method)

	y := divider + 1
	box := "[ ]"
	style := s.theme.Muted
	if s.approved {
		box = "[x]"
		style = s.theme.Success
	}
	autoBox := "[ ]"
	autoStyle := s.theme.Muted
	if s.autoApprove.Load() {
		autoBox = "[x]"
		autoStyle = s.theme.ToolName
	}
	approveText := box + " approved"
	printPanelLine(&surf, width, y, panelLine{text: approveText, style: style}, ctx.Method)
	s.approveRowY = y
	s.autoRowY = y
	s.autoToggleX = 2 + xui.StringWidth(approveText, ctx.Method) + 2
	contentRight := 1 + panelPad + contentWidth(width)
	autoText := layout.TruncateToWidth(autoBox+" auto", contentRight-s.autoToggleX, ctx.Method)
	surf.Print(s.autoToggleX, y, autoText, autoStyle, ctx.Method)
	clearX := s.autoToggleX + xui.StringWidth(autoText, ctx.Method) + 2
	clearText := layout.TruncateToWidth("clear", contentRight-clearX, ctx.Method)
	surf.Print(clearX, y, clearText, s.theme.Muted, ctx.Method)
	s.clearToggleX = clearX
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

// drawTabs renders the Status/Settings tab row and records its hit-test bounds.
func (s *Sidebar) drawTabs(surf *components.Surface, y int, method xui.WidthMethod) {
	x := 1 + panelPad
	statusStyle := s.theme.Muted
	if s.tab == tabStatus {
		statusStyle = s.theme.ToolName
	}
	s.statusTabMinX = x
	surf.Print(x, y, "status", statusStyle, method)
	x += xui.StringWidth("status", method)
	s.statusTabMaxX = x
	x += 2
	settingsStyle := s.theme.Muted
	if s.tab == tabSettings {
		settingsStyle = s.theme.ToolName
	}
	s.settingsTabMinX = x
	surf.Print(x, y, "settings", settingsStyle, method)
	x += xui.StringWidth("settings", method)
	s.settingsTabMaxX = x
	s.tabRowY = y
}

// drawSettings renders the settings tab body — only the stop@128 toggle. The
// approval toggles live next to the plan, not here.
func (s *Sidebar) drawSettings(surf *components.Surface, width, y, bottom int, method xui.WidthMethod) {
	if y > bottom {
		return
	}
	stopBox := "[ ]"
	stopStyle := s.theme.Muted
	if s.stopOnLimit {
		stopBox = "[x]"
		stopStyle = s.theme.ToolName
	}
	printPanelLine(surf, width, y, panelLine{text: stopBox + " stop@128", style: stopStyle}, method)
	s.stopRowY = y
}

// drawPlanDivider renders the plan pane's top edge on the row the plan title
// used to occupy: a pane separator that doubles as the "plan n/m" label, so
// the tab window above and the plan below read as separate boxes.
func (s *Sidebar) drawPlanDivider(surf *components.Surface, y, width int, method xui.WidthMethod) {
	completed := 0
	for _, item := range s.plan.Items {
		if item.Status == session.PlanCompleted || item.Status == session.PlanCancelled {
			completed++
		}
	}
	title := " plan "
	if len(s.plan.Items) > 0 {
		title += strconv.Itoa(completed) + "/" + strconv.Itoa(len(s.plan.Items)) + " "
	}
	for x := 1; x < width-1; x++ {
		surf.SetCell(x, y, xui.Cell{Char: "─", Width: 1, Style: s.theme.Border})
	}
	surf.SetCell(0, y, xui.Cell{Char: "├", Width: 1, Style: s.theme.Border})
	surf.SetCell(width-1, y, xui.Cell{Char: "┤", Width: 1, Style: s.theme.Border})
	surf.Print(1+panelPad, y, layout.TruncateToWidth(title, contentWidth(width), method), s.theme.Muted, method)
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
	lines = append(lines, panelLine{}, header("tokens"))
	for _, row := range tokens.BreakdownLines(s.usage) {
		lines = append(lines, panelLine{text: row, style: s.theme.Foreground})
	}

	sectionHeader := func(name string) panelLine { return panelLine{text: name, style: s.theme.Foreground} }

	lines = append(lines, panelLine{}, sectionHeader("MCP"))
	if len(s.runtime.MCP) == 0 {
		lines = append(lines, panelLine{text: "none", style: s.theme.Muted})
	} else {
		for _, status := range s.runtime.MCP {
			marker, style := mcpMarker(status.State, s.theme)
			lines = append(lines, panelLine{text: marker + " " + status.Name, style: style})
		}
	}

	lines = append(lines, panelLine{}, sectionHeader("LSP"))
	if len(s.runtime.LSP) == 0 {
		lines = append(lines, panelLine{text: "none", style: s.theme.Muted})
	} else {
		for _, lang := range s.runtime.LSP {
			marker, style := lspMarker(lang, s.theme)
			lines = append(lines, panelLine{text: marker + " " + lspName(lang), style: style})
		}
	}
	return lines
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

// lspMarker maps the bounded language record onto the same marker vocabulary
// as MCP: running means a live client generation, a missing binary or a failed
// start is destructive, and an installed-but-idle server stays muted.
func lspMarker(lang lsp.Language, theme components.Theme) (string, xui.Style) {
	switch {
	case lang.Running:
		return "●", theme.Success
	case lang.Error != "" || !lang.Installed:
		return "×", theme.Destructive
	default:
		return "○", theme.Muted
	}
}

// lspName renders the server label, falling back to the language id when the
// server field is empty (it is always populated for the V1 gopls profile).
func lspName(lang lsp.Language) string {
	if lang.Server != "" {
		return lang.Server
	}
	return lang.Language
}

func clampWidth(width int) int {
	if width == 0 {
		return Width
	}
	return min(max(width, minWidth), maxWidth)
}
