package transcript

import (
	"slices"
	"strings"
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/block"
	"github.com/alvnukov/cozyphi/internal/components/splash"
	"github.com/alvnukov/cozyphi/internal/components/status"
	"github.com/alvnukov/cozyphi/internal/components/toast"
	msglist "github.com/alvnukov/cozyphi/internal/components/transcript"
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
)

// textSel tracks drag selection over the transcript.
// Coordinates are content-space (relative to MessageList content origin),
// so the highlight stays on the selected text when the list scrolls.
type textSel struct {
	pending  bool
	dragging bool
	active   bool
	ax, ay   int
	ex, ey   int

	// Edge auto-scroll state: the pointer's screen row and the zone it sits
	// in while the drag is held at a viewport edge. dir is -1 toward
	// history, +1 toward the tail, 0 when the pointer is mid-view.
	py       int
	edgeDir  int
	edgeStep int
}

type projectionSyncMode uint8

const (
	projectionSyncNone projectionSyncMode = iota
	projectionSyncTail
	projectionSyncFull
)

func (s *textSel) clear() {
	*s = textSel{}
}

// TranscriptPane owns session snapshot, transcript widgets, and list interaction.
type TranscriptPane struct {
	theme components.Theme

	list      msglist.MessageList
	listIDs   []string
	state     *session.Reducer
	mapper    *Mapper
	subagents *SubagentStore
	welcome   splash.Screen
	syncMode  projectionSyncMode

	sel          textSel
	listH        int
	lastListSurf components.Surface

	onUsage func(session.TokenUsage)
	copyFn  func(text string) bool
	toastFn func(msg string, kind toast.ToastKind, d time.Duration)
}

// NewTranscriptPane builds an empty transcript view. version is the label
// shown on the welcome screen (e.g. "v0.16.0").
func NewTranscriptPane(theme components.Theme, spin *status.Spinner, version string) *TranscriptPane {
	t := &TranscriptPane{
		theme: theme,
		list: msglist.MessageList{
			Theme:      theme,
			Selected:   -1,
			GapBetween: toolGap,
		},
		welcome: splash.Screen{
			Theme:   theme,
			Version: version,
		},
		subagents: NewSubagentStore(),
		state:     session.NewReducer(session.Snapshot{}),
	}
	t.mapper = NewMapper(theme, spin, func() {
		t.list.InvalidateHeights()
	})
	// A turn-summary toggle changes which rows exist, not just a height:
	// regroup through the full sync path.
	t.mapper.onRegroup = func() {
		t.syncMode = projectionSyncFull
		t.Sync()
	}
	t.mapper.Children = t.subagents.Children
	t.mapper.ChildrenByJob = t.subagents.ChildrenByJob
	return t
}

// toolGap glues consecutive tool-call rows (0 blank rows) while every other
// adjacent pair keeps the list's single-row spacing. A turn-summary fold
// glues to the tool rows it kept visible (its failures) the same way.
func toolGap(prev, next components.Widget) int {
	if isToolRow(prev) && isToolRow(next) {
		return 0
	}
	if _, ok := prev.(*block.TurnSummaryBlock); ok && isToolRow(next) {
		return 0
	}
	return -1
}

// isToolRow reports whether an entry is a tool-call row: a generic tool_use,
// a bash run, or an agent spawn/wait.
func isToolRow(w components.Widget) bool {
	switch w.(type) {
	case *block.ToolBlock, *block.BashBlock, *block.AgentBlock, *block.DiffBlock:
		return true
	default:
		return false
	}
}

// SetUsageCallback fires when an assistant message reports token usage.
func (t *TranscriptPane) SetUsageCallback(fn func(session.TokenUsage)) {
	if t != nil {
		t.onUsage = fn
	}
}

// SetCopyHandlers wires clipboard copy and user feedback toasts.
func (t *TranscriptPane) SetCopyHandlers(
	copyFn func(text string) bool,
	toastFn func(msg string, kind toast.ToastKind, d time.Duration),
) {
	if t == nil {
		return
	}
	t.copyFn = copyFn
	t.toastFn = toastFn
}

// Snapshot returns the current session model (read-only use on UI goroutine).
func (t *TranscriptPane) Snapshot() session.Snapshot {
	if t == nil {
		return session.Snapshot{}
	}
	if t.state == nil {
		return session.Snapshot{}
	}
	return t.state.Snapshot()
}

// IsStreaming reports whether the agent stream is in flight.
func (t *TranscriptPane) IsStreaming() bool {
	if t == nil {
		return false
	}
	return session.IsStreaming(t.Snapshot())
}

// IsEmpty reports whether the transcript has no committed entries.
func (t *TranscriptPane) IsEmpty() bool {
	if t == nil {
		return true
	}
	return len(t.list.Entries) == 0
}

// LastCopyText returns copy text for the last message block.
func (t *TranscriptPane) LastCopyText() string {
	if t == nil {
		return ""
	}
	return t.list.LastCopyText()
}

// AtBottom reports whether the list is pinned to the latest content.
func (t *TranscriptPane) AtBottom() bool {
	if t == nil {
		return true
	}
	return t.list.ScrollFromBottom == 0
}

// StickToBottom scrolls the list to the latest content.
func (t *TranscriptPane) StickToBottom() {
	if t != nil {
		t.list.StickToBottom()
	}
}

// SelectionActive reports whether a drag-selection highlight is shown.
func (t *TranscriptPane) SelectionActive() bool {
	return t != nil && t.sel.active
}

// ClearSelection clears transcript text selection state.
func (t *TranscriptPane) ClearSelection() {
	if t != nil {
		t.sel.clear()
	}
}

// ApplySession applies one session event on the UI goroutine.
func (t *TranscriptPane) ApplySession(ev session.Event) {
	if t == nil {
		return
	}
	if t.state == nil {
		t.state = session.NewReducer(session.Snapshot{})
	}
	before := t.state.Snapshot()
	t.state.Apply(ev)
	t.markProjectionChange(ev, before, t.state.Snapshot())
	if usage := usageFromEvent(ev); usage.Reported() && t.onUsage != nil {
		t.onUsage(usage)
	}
	if td, ok := ev.(session.ToolData); ok {
		t.applyAgentToolData(td)
	}
}

func usageFromEvent(ev session.Event) session.TokenUsage {
	switch event := ev.(type) {
	case session.AssistantMessageUpdate:
		return event.Message.Usage
	case session.CompactionComplete:
		if event.Failed || event.Compaction.TokensAfter <= 0 {
			return session.TokenUsage{}
		}
		return session.TokenUsage{
			PromptTokens: event.Compaction.TokensAfter,
			TotalTokens:  event.Compaction.TokensAfter,
			Estimated:    true,
		}
	default:
		return session.TokenUsage{}
	}
}

// ApplyJobProgress updates nested sub-agent rows. Returns true when sync is needed.
func (t *TranscriptPane) ApplyJobProgress(p job.Progress) bool {
	if t == nil || t.subagents == nil {
		return false
	}
	changed := t.subagents.ApplyProgress(p)
	if !changed {
		return false
	}
	if p.ParentToolUseID != "" && lastMessageOwnsTool(t.Snapshot(), p.ParentToolUseID) {
		t.markProjectionTail()
	} else {
		t.syncMode = projectionSyncFull
	}
	return true
}

// Sync rebuilds transcript widgets from snap.
func (t *TranscriptPane) Sync() {
	if t == nil || t.mapper == nil {
		return
	}
	if t.syncMode == projectionSyncNone {
		return
	}
	snap := t.Snapshot()
	if t.syncMode == projectionSyncTail {
		if dirty, ok := t.mapper.syncTail(t.list.Entries, t.listIDs, snap); ok {
			t.list.InvalidateHeightsAt(dirty...)
			t.syncMode = projectionSyncNone
			return
		}
	}
	oldIDs := t.listIDs
	entries, ids, dirty := t.mapper.Sync(t.list.Entries, t.listIDs, snap)
	t.list.ReindexHeights(oldIDs, ids)
	t.list.Entries = entries
	t.listIDs = ids
	t.list.InvalidateHeightsAt(dirty...)
	t.syncMode = projectionSyncNone
}

// LoadReplay replaces snap and clears widget cache after ctrl replay.
func (t *TranscriptPane) LoadReplay(snap session.Snapshot) {
	if t == nil {
		return
	}
	if t.state == nil {
		t.state = session.NewReducer(snap)
	} else {
		t.state.Replace(snap)
	}
	t.list.Entries = nil
	t.listIDs = nil
	if t.mapper != nil {
		t.mapper.Reset()
	}
	t.list.InvalidateHeights()
	t.syncMode = projectionSyncFull
	if usage := latestReportedUsage(snap.Messages); usage.Reported() && t.onUsage != nil {
		t.onUsage(usage)
	}
}

func latestReportedUsage(messages []session.Message) session.TokenUsage {
	for i := range slices.Backward(messages) {
		if messages[i].Usage.Reported() {
			return messages[i].Usage
		}
	}
	return session.TokenUsage{}
}

// ResetSubagents clears nested job UI state (e.g. after /clear).
func (t *TranscriptPane) ResetSubagents() {
	if t == nil {
		return
	}
	t.subagents = NewSubagentStore()
	t.syncMode = projectionSyncFull
	if t.mapper != nil {
		t.mapper.Children = t.subagents.Children
		t.mapper.ChildrenByJob = t.subagents.ChildrenByJob
	}
}

func (t *TranscriptPane) markProjectionChange(ev session.Event, before, after session.Snapshot) {
	switch e := ev.(type) {
	case session.CompactionStarted:
		return
	case session.CompactionComplete:
		if e.Failed {
			return
		}
	case session.AssistantMessageUpdate:
		if len(before.Messages) == len(after.Messages) && len(after.Messages) > 0 {
			beforeLast := before.Messages[len(before.Messages)-1]
			afterLast := after.Messages[len(after.Messages)-1]
			if beforeLast.Role == session.RoleAssistant && afterLast.Role == session.RoleAssistant &&
				beforeLast.ID == afterLast.ID {
				t.markProjectionTail()
				return
			}
		}
	case session.ToolData:
		if lastMessageOwnsTool(after, e.Run.ToolUseID) {
			t.markProjectionTail()
			return
		}
	}
	t.syncMode = projectionSyncFull
}

func (t *TranscriptPane) markProjectionTail() {
	if t.syncMode == projectionSyncNone {
		t.syncMode = projectionSyncTail
	}
}

func lastMessageOwnsTool(snap session.Snapshot, toolUseID string) bool {
	if toolUseID == "" || len(snap.Messages) == 0 {
		return false
	}
	last := snap.Messages[len(snap.Messages)-1]
	if last.Role == session.RoleLocalBash || last.Role == session.RoleWatch {
		return last.ID == toolUseID
	}
	if last.Role != session.RoleAssistant {
		return false
	}
	for _, content := range last.Content {
		if content.Type == session.BlockToolUse && content.ID == toolUseID {
			return true
		}
	}
	return false
}

// SetTheme updates transcript chrome and existing widgets.
func (t *TranscriptPane) SetTheme(th components.Theme) {
	if t == nil {
		return
	}
	t.theme = th
	t.welcome.Theme = th
	t.list.Theme = th
	if t.mapper != nil {
		t.mapper.SetTheme(th)
	}
	applyThemeToWidgets(t.list.Entries, th)
	t.list.InvalidateHeights()
}

// Draw renders the transcript or welcome screen into listH.
func (t *TranscriptPane) Draw(ctx components.DrawContext, width, height int) components.Surface {
	if t == nil {
		return components.Surface{}
	}
	t.listH = height
	constraints := ctx.WithConstraints(components.Size{}, components.Size{Width: width, Height: height})
	var listSurf components.Surface
	if len(t.list.Entries) == 0 {
		listSurf = t.welcome.Draw(constraints)
	} else {
		listSurf = t.list.Draw(constraints)
	}
	if t.sel.active {
		listSurf = components.CloneSurface(listSurf)
		hl := t.theme.SelectionBg
		hl.Fg = t.theme.SelectionFg.Fg
		ax, ay, ex, ey := t.viewSel()
		components.ApplySelectionHighlight(&listSurf, ax, ay, ex, ey, hl)
	}
	t.lastListSurf = listSurf
	return listSurf
}

// HandlePageKey forwards page up/down to the message list. Shift turns the
// page keys into turn jumps: the viewport hops between user prompts instead
// of moving raw screenfuls.
func (t *TranscriptPane) HandlePageKey(ctx *components.EventContext, ev xui.KeyEvent) {
	if t == nil {
		return
	}
	if ev.Mods.Has(xui.ModShift) {
		switch ev.Code {
		case xui.KeyPageUp:
			t.JumpTurn(ctx, -1)
			return
		case xui.KeyPageDown:
			t.JumpTurn(ctx, 1)
			return
		}
	}
	t.list.Handle(ctx, ev)
}

// JumpTurn scrolls to the previous (dir < 0) or next user prompt. Past the
// first it lands on the transcript top; past the last it re-pins the tail.
func (t *TranscriptPane) JumpTurn(ctx *components.EventContext, dir int) {
	if t == nil || len(t.list.Entries) == 0 {
		return
	}
	top := t.list.TopEntryIndex()
	idx := -1
	if dir < 0 {
		for i := top - 1; i >= 0; i-- {
			if isTurnStart(t.list.Entries[i]) {
				idx = i
				break
			}
		}
		if idx < 0 {
			idx = 0
		}
	} else {
		for i := top + 1; i < len(t.list.Entries); i++ {
			if isTurnStart(t.list.Entries[i]) {
				idx = i
				break
			}
		}
		if idx < 0 {
			t.list.StickToBottom()
			ctx.ConsumeAndRedraw()
			return
		}
	}
	t.list.ScrollToEntry(idx)
	ctx.ConsumeAndRedraw()
}

// isTurnStart reports a sent user prompt's row — the anchor a turn jump
// lands on. A queued prompt waits inside someone else's turn and is skipped.
func isTurnStart(w components.Widget) bool {
	u, ok := w.(*block.UserBlock)
	return ok && !u.Queued
}

// ToggleVerbose flips the transcript between condensed — older turns folded
// to summary rows — and verbose, and reports the new verbose state.
func (t *TranscriptPane) ToggleVerbose() bool {
	if t == nil || t.mapper == nil {
		return false
	}
	t.mapper.SetVerbose(!t.mapper.Verbose())
	t.syncMode = projectionSyncFull
	t.Sync()
	return t.mapper.Verbose()
}

// CopySelectionOrLast copies the selected block, or the last message when
// nothing is selected. The chord that runs it lives in the keys table
// (keys.CmdCopyLast); the editor dispatches it here.
func (t *TranscriptPane) CopySelectionOrLast(ctx *components.EventContext) bool {
	if t == nil {
		return false
	}
	text := t.list.SelectedCopyText()
	if text == "" {
		text = t.list.LastCopyText()
	}
	if text == "" {
		return true
	}
	t.copyBlock(text)
	ctx.ConsumeAndRedraw()
	return true
}

// HandleMouse handles wheel, drag-selection, and block selection over the list.
func (t *TranscriptPane) HandleMouse(ctx *components.EventContext, e xui.MouseEvent, focusComposer func()) {
	if t == nil {
		return
	}
	if e.Button == xui.MouseWheelUp || e.Button == xui.MouseWheelDown {
		t.list.Handle(ctx, e)
		return
	}
	if e.Button != xui.MouseLeft && e.Button != 0 {
		if e.Action != xui.MouseMotion && e.Action != xui.MouseDrag {
			return
		}
	}

	inList := e.Y >= 0 && e.Y < t.listH && t.listH > 0 && len(t.list.Entries) > 0

	switch e.Action {
	case xui.MousePress:
		if e.Button != xui.MouseLeft {
			return
		}
		if !inList {
			t.sel.clear()
			ctx.Redraw = true
			return
		}
		cy := t.toContentY(e.Y)
		t.sel = textSel{
			pending: true,
			ax:      e.X,
			ay:      cy,
			ex:      e.X,
			ey:      cy,
		}
		if focusComposer != nil {
			focusComposer()
		}
		ctx.Redraw = true
		return

	case xui.MouseDrag, xui.MouseMotion:
		if !t.sel.pending && !t.sel.dragging {
			return
		}
		if e.Action == xui.MouseMotion && e.Button != xui.MouseLeft {
			return
		}
		t.sel.dragging = true
		t.sel.active = true
		t.sel.ex = e.X
		t.sel.ey = t.toContentY(e.Y)
		t.sel.py = e.Y
		t.sel.edgeDir, t.sel.edgeStep = t.edgeScrollZone(e.Y)
		ctx.ConsumeAndRedraw()
		return

	case xui.MouseRelease:
		if e.Button != xui.MouseLeft {
			return
		}
		if !t.sel.pending && !t.sel.dragging {
			return
		}
		t.sel.ex = e.X
		t.sel.ey = t.toContentY(e.Y)
		if t.sel.dragging && (t.sel.ax != t.sel.ex || t.sel.ay != t.sel.ey) {
			ax, ay, ex, ey := t.viewSel()
			text := components.ExtractSurfaceText(t.lastListSurf, ax, ay, ex, ey)
			t.sel.active = true
			if text != "" {
				t.copyResult(text, "Selection copied to clipboard", "Failed to copy selection")
			}
			t.sel.pending = false
			t.sel.dragging = false
			t.sel.edgeDir = 0
			ctx.ConsumeAndRedraw()
			return
		}
		idx := t.list.IndexAtPoint(e.X, e.Y)
		if idx >= 0 {
			t.list.Selected = idx
			// A clean click — no selection came of it — folds an expanded
			// block wherever it lands. Presses on a title row never get
			// here (the block consumed them), so this is the body path.
			if c, ok := t.list.Entries[idx].(interface{ CollapseOnClick() bool }); ok {
				c.CollapseOnClick()
			}
		}
		t.sel.clear()
		if focusComposer != nil {
			focusComposer()
		}
		ctx.ConsumeAndRedraw()
	}
}

// edgeScrollZone maps the pointer's screen row to a drag-selection
// auto-scroll velocity: nothing mid-view, a slow crawl in the rows just
// inside an edge, faster on the edge row itself, and faster still past the
// edge — the pointer has left the list into the composer/footer zone —
// scaling with how far past it went. y is screen-absolute; the transcript
// starts at screen row 0, so y >= listH means the pointer is below the list.
func (t *TranscriptPane) edgeScrollZone(y int) (dir, step int) {
	if t.listH < 8 {
		return 0, 0
	}
	if y >= t.listH/2 {
		switch {
		case y >= t.listH:
			return 1, min(3+2*(y-t.listH+1), 10)
		case y == t.listH-1:
			return 1, 3
		case y >= t.listH-4:
			return 1, 1
		}
		return 0, 0
	}
	switch {
	case y <= 0:
		return -1, 3
	case y <= 3:
		return -1, 1
	}
	return 0, 0
}

// AdvanceEdgeScroll applies one auto-scroll step while a drag selection is
// held at a viewport edge, extending the selection endpoint by the rows the
// viewport moved. It reports whether another step is wanted; Editor.Draw
// wires that to a wake, so the scroll continues while the button is held
// even without pointer motion.
func (t *TranscriptPane) AdvanceEdgeScroll() bool {
	if t == nil || !t.sel.dragging || t.sel.edgeDir == 0 {
		return false
	}
	moved := t.list.ScrollBy(t.sel.edgeDir * t.sel.edgeStep)
	if moved == 0 {
		t.sel.edgeDir = 0
		return false
	}
	t.sel.ey += moved
	return true
}

func (t *TranscriptPane) applyAgentToolData(td session.ToolData) {
	name := strings.ToLower(td.Run.Name)
	switch name {
	case "agent_spawn", "agent_wait":
	default:
		return
	}
	parsed := tools.ParseAgentResult(td.Run.Output)
	if !parsed.OK {
		return
	}
	t.subagents.Bind(parsed.JobID, td.Run.ToolUseID)
	t.subagents.ApplyResult(td.Run.ToolUseID, parsed)
}

func (t *TranscriptPane) viewSel() (ax, ay, ex, ey int) {
	ox := t.list.ContentOrigin()
	return t.sel.ax, t.sel.ay + ox, t.sel.ex, t.sel.ey + ox
}

func (t *TranscriptPane) toContentY(viewY int) int {
	return viewY - t.list.ContentOrigin()
}

func (t *TranscriptPane) copyResult(text, okMsg, failMsg string) {
	if text == "" {
		return
	}
	ok := t.copyFn != nil && t.copyFn(text)
	if ok && t.toastFn != nil {
		t.toastFn(okMsg, toast.ToastSuccess, 2*time.Second)
	} else if !ok && t.toastFn != nil {
		t.toastFn(failMsg, toast.ToastError, 2*time.Second)
	}
}

// CopyBlock copies text to the clipboard with user feedback.
func (t *TranscriptPane) CopyBlock(text string) {
	t.copyBlock(text)
}

func (t *TranscriptPane) copyBlock(text string) {
	t.copyResult(text, "Copied to clipboard", "Failed to copy")
}

func applyThemeToWidgets(entries []components.Widget, th components.Theme) {
	for _, w := range entries {
		switch b := w.(type) {
		case *block.UserBlock:
			b.Theme = th
		case *block.AssistantBlock:
			b.Theme = th
		case *block.ThinkingBlock:
			b.Theme = th
		case *block.CompactionBlock:
			b.Theme = th
		case *block.ToolBlock:
			b.Theme = th
		case *block.BashBlock:
			b.Theme = th
		case *block.AgentBlock:
			b.Theme = th
		case *block.DiffBlock:
			b.Theme = th
		case *block.TurnSummaryBlock:
			b.Theme = th
		}
	}
}
