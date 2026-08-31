package overlays

import (
	"fmt"
	"strings"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/input"
	"github.com/alvnukov/cozyphi/internal/components/layout"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

type overlayComposer interface {
	HideCompleters()
	HidePalette()
}

// Overlays owns permission and continue-ask UI that replaces the composer slot.
type Overlays struct {
	theme    components.Theme
	perm     *permAskState
	cont     *continueAskState
	question *questionAskState
	connect  *connectState
	activity *controller.ActivityHandler
	composer overlayComposer

	focusEditor func()
	focusChat   func()
}

// NewOverlays builds overlay state handlers.
func NewOverlays(
	theme components.Theme,
	activity *controller.ActivityHandler,
	composer overlayComposer,
	focusEditor, focusChat func(),
) *Overlays {
	return &Overlays{
		theme:       theme,
		activity:    activity,
		composer:    composer,
		focusEditor: focusEditor,
		focusChat:   focusChat,
	}
}

// SetTheme updates overlay chrome styling.
func (o *Overlays) SetTheme(th components.Theme) {
	if o != nil {
		o.theme = th
	}
}

// Active reports whether a modal overlay is showing.
func (o *Overlays) Active() bool {
	return o != nil && (o.perm != nil || o.cont != nil || o.question != nil || o.connect != nil)
}

// PermissionActive reports whether the permission overlay is showing.
func (o *Overlays) PermissionActive() bool {
	return o != nil && o.perm != nil
}

// ContinueActive reports whether the continue overlay is showing.
func (o *Overlays) ContinueActive() bool {
	return o != nil && o.cont != nil
}

// CancelActive dismisses whatever ask or connect flow is showing — the empty
// replies mean "declined", as Escape does inside each overlay — and reports
// whether anything was showing. It is the interrupt path: Ctrl+C is handled
// by the runtime before any overlay key handler sees it.
func (o *Overlays) CancelActive() bool {
	if !o.Active() {
		return false
	}
	o.dismissAll()
	return true
}

// dismissAll resolves every ask with an empty reply and drops the connect
// flow, leaving no overlay showing.
func (o *Overlays) dismissAll() {
	o.resolvePermission(controller.AskReply{})
	o.resolveContinue(controller.ContinueReply{})
	o.resolveQuestion(controller.QuestionReply{})
	o.clearConnect()
}

// Apply routes overlay-related bus messages.
func (o *Overlays) Apply(m controller.Msg) {
	if o == nil {
		return
	}
	switch msg := m.(type) {
	case controller.PermissionAskMsg:
		o.beginPermissionAsk(msg)
	case controller.PermissionDismissMsg:
		o.dismissPermission()
	case controller.ContinueAskMsg:
		o.beginContinueAsk(msg)
	case controller.ContinueDismissMsg:
		o.dismissContinue()
	case controller.QuestionAskMsg:
		o.beginQuestionAsk(msg)
	case controller.QuestionDismissMsg:
		o.dismissQuestion()
	case controller.ProviderCatalogMsg:
		o.updateConnectCatalog(msg.Providers, msg.ErrText)
	case controller.ProviderDeviceCodeMsg:
		o.showDeviceCode(msg)
	case controller.ProviderAuthorizationMsg:
		o.showAuthorization(msg)
	case controller.ProviderConnectResultMsg:
		o.finishConnect(msg.ProviderID, msg.ErrText)
	}
}

// HandlePermissionKey handles keyboard input while permission ask is active.
func (o *Overlays) HandlePermissionKey(ctx *components.EventContext, e xui.KeyEvent) bool {
	return o != nil && o.perm != nil && o.handlePermissionKey(ctx, e)
}

// HandleContinueKey handles keyboard input while continue ask is active.
func (o *Overlays) HandleContinueKey(ctx *components.EventContext, e xui.KeyEvent) bool {
	return o != nil && o.cont != nil && o.handleContinueKey(ctx, e)
}

// ResolvePermission sends a permission reply and clears the overlay.
func (o *Overlays) ResolvePermission(r controller.AskReply) {
	o.resolvePermission(r)
}

// ResolveContinue sends a continue reply and clears the overlay.
func (o *Overlays) ResolveContinue(r controller.ContinueReply) {
	o.resolveContinue(r)
}

// HandleQuestionKey handles keyboard input while a question overlay is active.
func (o *Overlays) HandleQuestionKey(ctx *components.EventContext, e xui.KeyEvent) bool {
	return o != nil && o.question != nil && o.handleQuestionKey(ctx, e)
}

// HandleAskPaste routes a paste into whichever ask overlay is taking text right
// now. The asks are modal — while one is up the composer never sees the event —
// so without this a pasted path or reason silently vanished, and the field it
// was meant for was the one place retyping it by hand hurt most.
func (o *Overlays) HandleAskPaste(ctx *components.EventContext, e xui.PasteEvent) bool {
	if o == nil {
		return false
	}
	var line *input.Line
	switch {
	case o.perm != nil && o.perm.feedbackMode:
		line = &o.perm.feedback
	case o.question != nil && o.question.editing && o.question.tab < len(o.question.customs):
		line = &o.question.customs[o.question.tab]
	default:
		return false
	}
	line.Insert(e.Text)
	ctx.ConsumeAndRedraw()
	return true
}

// ResolveQuestion sends a question reply and clears the overlay.
func (o *Overlays) ResolveQuestion(r controller.QuestionReply) {
	o.resolveQuestion(r)
}

// PreferredBottomHeight estimates rows for the bottom overlay or composer slot.
// The estimate counts the rows the panel actually renders — wrapping included —
// so no option ends up truncated out of reach on a narrow terminal.
func (o *Overlays) PreferredBottomHeight(width int, method xui.WidthMethod) (height int, overlay bool) {
	if o == nil {
		return 0, false
	}
	if o.perm != nil {
		return o.perm.preferredAskHeight(o.theme, width, method), true
	}
	if o.cont != nil {
		return o.cont.preferredAskHeight(o.theme, width, method), true
	}
	if o.question != nil {
		return o.question.preferredAskHeight(o.theme, width, method), true
	}
	if o.connect != nil {
		return o.connect.preferredHeight(), true
	}
	return 0, false
}

// DrawBottom renders the overlay panel when active.
func (o *Overlays) DrawBottom(ctx components.DrawContext, width, height int) (components.Surface, bool) {
	if o == nil {
		return components.Surface{}, false
	}
	if o.perm != nil {
		return o.drawPermissionAsk(ctx, width, height), true
	}
	if o.cont != nil {
		return o.drawContinueAsk(ctx, width, height), true
	}
	if o.question != nil {
		return o.drawQuestionAsk(ctx, width, height), true
	}
	if o.connect != nil {
		return o.drawConnect(ctx, width, height), true
	}
	return components.Surface{}, false
}

// beginAsk runs the shared opening routine of every modal ask: resolve any
// ask already showing, drop the connect flow, hide composer popups, mark the
// session awaiting approval, and focus the overlay. The caller then installs
// its own state.
func (o *Overlays) beginAsk() {
	o.dismissAll()
	if o.composer != nil {
		o.composer.HideCompleters()
		o.composer.HidePalette()
	}
	o.activity.Apply(controller.ActivityAwaitingApproval)
	if o.focusEditor != nil {
		o.focusEditor()
	}
}

// endAsk runs the shared close-down of every modal ask: restore the activity
// line and refocus the chat. It is a no-op when the ask was not showing.
func (o *Overlays) endAsk(wasShowing bool) {
	if !wasShowing {
		return
	}
	if o.activity != nil && o.activity.Current == controller.ActivityAwaitingApproval {
		o.activity.Apply(controller.ActivityTools)
	}
	if o.focusChat != nil {
		o.focusChat()
	}
}

// sendReply delivers an ask reply without ever blocking the UI goroutine: a
// full or absent channel means the controller already stopped waiting.
func sendReply[T any](reply chan T, r T) {
	select {
	case reply <- r:
	default:
	}
}

func (o *Overlays) beginPermissionAsk(msg controller.PermissionAskMsg) {
	o.beginAsk()
	o.perm = newPermAskState(msg.Request, msg.Reason, msg.Reply)
}

func (o *Overlays) dismissPermission() {
	st := o.perm
	o.perm = nil
	o.endAsk(st != nil)
}

func (o *Overlays) resolvePermission(r controller.AskReply) {
	st := o.perm
	o.perm = nil
	o.endAsk(st != nil)
	if st != nil {
		sendReply(st.reply, r)
	}
}

func (o *Overlays) beginContinueAsk(msg controller.ContinueAskMsg) {
	o.beginAsk()
	o.cont = newContinueAskState(msg.MaxRounds, msg.Reply)
}

func (o *Overlays) dismissContinue() {
	st := o.cont
	o.cont = nil
	o.endAsk(st != nil)
}

func (o *Overlays) resolveContinue(r controller.ContinueReply) {
	st := o.cont
	o.cont = nil
	o.endAsk(st != nil)
	if st != nil {
		sendReply(st.reply, r)
	}
}

// handlePermissionKey routes a key to the modal ask. Every key stops here — the
// ask is modal — but one that does nothing now says so, instead of vanishing
// into a frame that never changes.
func (o *Overlays) handlePermissionKey(ctx *components.EventContext, e xui.KeyEvent) bool {
	st := o.perm
	if st == nil || !e.Press {
		return false
	}

	if st.feedbackMode {
		return o.handlePermissionFeedbackKey(ctx, e)
	}

	if o.applyPermissionKey(st, e) {
		st.hint = ""
	} else {
		st.hint = unboundKeyHint(len(askOptionLabels))
	}
	ctx.ConsumeAndRedraw()
	return true
}

// applyPermissionKey reports whether e did something. A digit picks its option
// with or without Alt — a panel that prints "[1]" has to honor a bare 1 — and
// y/n answer outright the two cases worth a single keystroke.
func (o *Overlays) applyPermissionKey(st *permAskState, e xui.KeyEvent) bool {
	if e.Code == xui.KeyRune && e.Rune >= '1' && e.Rune <= '9' && !e.Mods.Has(xui.ModCtrl) {
		idx := int(e.Rune - '1')
		if idx >= len(askOptionLabels) {
			return false
		}
		o.acceptPermissionOption(askOption(idx))
		return true
	}

	switch e.Code {
	case xui.KeyEscape:
		o.resolvePermission(controller.AskReply{})
		return true
	case xui.KeyUp:
		st.selectPrev()
		return true
	case xui.KeyDown, xui.KeyTab:
		st.selectNext()
		return true
	case xui.KeyEnter:
		o.acceptPermissionOption(askOption(st.selected))
		return true
	case xui.KeyRune:
		if e.Mods.Has(xui.ModCtrl) || e.Mods.Has(xui.ModAlt) {
			return false
		}
		switch e.HotkeyRune() {
		case 'k', 'K':
			st.selectPrev()
			return true
		case 'j', 'J':
			st.selectNext()
			return true
		case 'y', 'Y':
			o.acceptPermissionOption(askOptApprove)
			return true
		case 'n', 'N':
			o.resolvePermission(controller.AskReply{})
			return true
		}
	}
	return false
}

// selectPrev and selectNext walk the options as a ring. Stopping dead at the
// top row made k a key that did nothing, with nothing on screen to say why.
func (st *permAskState) selectPrev() {
	st.selected = (st.selected + len(askOptionLabels) - 1) % len(askOptionLabels)
}

func (st *permAskState) selectNext() {
	st.selected = (st.selected + 1) % len(askOptionLabels)
}

// unboundKeyHint names the keys that do work, shown in place of the hint row.
func unboundKeyHint(options int) string {
	return fmt.Sprintf("That key does nothing here — press 1-%d, y, n, or Esc", options)
}

func (o *Overlays) acceptPermissionOption(opt askOption) {
	st := o.perm
	if st == nil {
		return
	}
	switch opt {
	case askOptApprove:
		o.resolvePermission(controller.AskReply{Approved: true})
	case askOptAllowSession:
		o.resolvePermission(controller.AskReply{Approved: true, AllowSession: true})
	case askOptAllowPersistent:
		o.resolvePermission(controller.AskReply{Approved: true, AllowPersistent: true})
	case askOptDenyFeedback:
		st.feedbackMode = true
		st.feedback.Clear()
	}
}

// handlePermissionFeedbackKey answers the two keys the prompt owns and hands
// the rest to the shared line editor, so the feedback field types like every
// other field in the app. Everything still stops here — the ask is modal.
func (o *Overlays) handlePermissionFeedbackKey(ctx *components.EventContext, e xui.KeyEvent) bool {
	st := o.perm
	if st == nil {
		return false
	}
	switch e.Code {
	case xui.KeyEscape:
		st.feedbackMode = false
		st.feedback.Clear()
	case xui.KeyEnter:
		o.resolvePermission(controller.AskReply{Feedback: st.feedback.Trimmed()})
	default:
		st.feedback.Key(e)
	}
	ctx.ConsumeAndRedraw()
	return true
}

// handleContinueKey mirrors the permission ask — same keys, same wrapping, same
// answer to a key that does nothing. Two modals sharing one slot must not teach
// two different key sets.
func (o *Overlays) handleContinueKey(ctx *components.EventContext, e xui.KeyEvent) bool {
	st := o.cont
	if st == nil || !e.Press {
		return false
	}

	if o.applyContinueKey(st, e) {
		st.hint = ""
	} else {
		st.hint = unboundKeyHint(len(continueOptionLabels))
	}
	ctx.ConsumeAndRedraw()
	return true
}

func (o *Overlays) applyContinueKey(st *continueAskState, e xui.KeyEvent) bool {
	if e.Code == xui.KeyRune && e.Rune >= '1' && e.Rune <= '9' && !e.Mods.Has(xui.ModCtrl) {
		idx := int(e.Rune - '1')
		if idx >= len(continueOptionLabels) {
			return false
		}
		o.acceptContinueOption(idx)
		return true
	}

	switch e.Code {
	case xui.KeyEscape:
		o.resolveContinue(controller.ContinueReply{})
		return true
	case xui.KeyUp:
		st.selectPrev()
		return true
	case xui.KeyDown, xui.KeyTab:
		st.selectNext()
		return true
	case xui.KeyEnter:
		o.acceptContinueOption(st.selected)
		return true
	case xui.KeyRune:
		if e.Mods.Has(xui.ModCtrl) || e.Mods.Has(xui.ModAlt) {
			return false
		}
		switch e.HotkeyRune() {
		case 'k', 'K':
			st.selectPrev()
			return true
		case 'j', 'J':
			st.selectNext()
			return true
		case 'y', 'Y':
			o.acceptContinueOption(0)
			return true
		case 'n', 'N':
			o.resolveContinue(controller.ContinueReply{})
			return true
		}
	}
	return false
}

func (st *continueAskState) selectPrev() {
	st.selected = (st.selected + len(continueOptionLabels) - 1) % len(continueOptionLabels)
}

func (st *continueAskState) selectNext() {
	st.selected = (st.selected + 1) % len(continueOptionLabels)
}

func (o *Overlays) acceptContinueOption(idx int) {
	switch idx {
	case 0:
		o.resolveContinue(controller.ContinueReply{Continue: true})
	default:
		o.resolveContinue(controller.ContinueReply{})
	}
}

// askInnerWidth is the usable width inside the ask panel's rounded border;
// every ask draws and measures its rows at this width.
func askInnerWidth(width int) int {
	innerW := width - 4
	if innerW < 10 {
		return width
	}
	return innerW
}

// askPrimary is the accent every ask highlights selection with.
func askPrimary(th components.Theme) xui.Style {
	if th.ToolName.Fg.Kind != 0 {
		return th.ToolName
	}
	return th.Success
}

func (o *Overlays) drawPermissionAsk(ctx components.DrawContext, width, height int) components.Surface {
	st := o.perm
	if st == nil {
		return components.NewSurface(width, height, nil)
	}
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = st.preferredAskHeight(o.theme, width, ctx.Method)
	}
	innerW := askInnerWidth(width)
	body, answer := st.askRows(o.theme, innerW, ctx.Method)
	body = fitAskBody(o.theme, body, height-2, answer, innerW, ctx.Method)
	return paintAskPanel(body, width, height, o.theme.Warning, ctx.Method)
}

// fitAskBody drops detail rows from the middle when the slot is shorter than
// the body was measured for, keeping the answer rows on screen: an ask nobody
// can answer stalls the run, while an elided command is still readable.
func fitAskBody(
	th components.Theme,
	body []components.RichLine,
	avail, answer, innerW int,
	method xui.WidthMethod,
) []components.RichLine {
	if avail <= 0 || len(body) <= avail {
		return body
	}
	head := avail - answer - 1 // one row reports what was dropped
	if head < 1 {
		return body[len(body)-avail:]
	}
	out := make([]components.RichLine, 0, avail)
	out = append(out, body[:head]...)
	out = append(out, components.WrapSpans([]components.Span{
		{Text: fmt.Sprintf("… %d more lines", len(body)-answer-head), Style: th.Muted},
	}, innerW, method)...)
	out = append(out, body[len(body)-answer:]...)
	if len(out) > avail {
		out = out[len(out)-avail:]
	}
	return out
}

func (o *Overlays) drawContinueAsk(ctx components.DrawContext, width, height int) components.Surface {
	st := o.cont
	if st == nil {
		return components.NewSurface(width, height, nil)
	}
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = st.preferredAskHeight(o.theme, width, ctx.Method)
	}
	body := st.askRows(o.theme, askInnerWidth(width), ctx.Method)
	return paintAskPanel(body, width, height, o.theme.Warning, ctx.Method)
}

type askOption int

const (
	askOptApprove askOption = iota
	askOptAllowSession
	askOptAllowPersistent
	askOptDenyFeedback
)

var askOptionLabels = []string{
	"Approve",
	"Allow All for This Session",
	"Allow All for Every Session",
	"Deny with feedback",
}

var continueOptionLabels = []string{
	"Continue",
	"Stop",
}

type permAskState struct {
	req    permission.Request
	reason string
	reply  chan controller.AskReply

	header       string
	detail       string
	selected     int
	feedbackMode bool
	feedback     input.Line

	// hint replaces the standard key hint after a key the ask cannot use.
	hint string
}

type continueAskState struct {
	maxRounds int
	reply     chan controller.ContinueReply
	selected  int

	// hint replaces the standard key hint after a key the ask cannot use.
	hint string
}

// askDetailLines caps the detail rows an ask shows. Three lines used to hide
// the redirect at the end of a heredoc — the part actually worth approving —
// while twelve still leaves the options room on any usable terminal.
const askDetailLines = 12

// formatAskHeader turns a request into the question and the evidence for it.
// The detail is clipped rather than shown whole: an unbounded command would
// push the options off the panel.
func formatAskHeader(req permission.Request) (header, detail string) {
	header, detail = describeAsk(req)
	return header, clipLines(detail, askDetailLines)
}

func describeAsk(req permission.Request) (header, detail string) {
	switch req.Action {
	case permission.ActionBash:
		return "Run this command?", req.Command
	case permission.ActionEdit:
		return pathHeader("Allow editing file", req.Paths), strings.Join(req.Paths, "\n")
	case permission.ActionWrite:
		return pathHeader("Allow creating file", req.Paths), strings.Join(req.Paths, "\n")
	default:
		return fmt.Sprintf("Invoke tool %s?", req.Tool), permission.Summarize(req)
	}
}

// pathHeader pluralises the header, because a request that touches three files
// must not read as a request about one — every path is listed as detail.
func pathHeader(base string, paths []string) string {
	if len(paths) > 1 {
		return base + "s:"
	}
	return base + ":"
}

// clipLines keeps the first n lines and says how many it dropped, so clipped
// detail reads as clipped instead of as the whole thing.
func clipLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[:n], "\n") + fmt.Sprintf("\n… %d more lines", len(lines)-n)
}

func newPermAskState(req permission.Request, reason string, reply chan controller.AskReply) *permAskState {
	h, d := formatAskHeader(req)
	return &permAskState{
		req:      req,
		reason:   reason,
		reply:    reply,
		header:   h,
		detail:   d,
		selected: 0,
	}
}

func newContinueAskState(maxRounds int, reply chan controller.ContinueReply) *continueAskState {
	return &continueAskState{
		maxRounds: maxRounds,
		reply:     reply,
		selected:  0,
	}
}

// preferredAskHeight is the panel height that fits every rendered row: the
// wrapped body plus the border. Counting the real rows (not newline
// arithmetic) is what keeps late options reachable on narrow terminals.
func (st *permAskState) preferredAskHeight(th components.Theme, width int, method xui.WidthMethod) int {
	if st == nil {
		return 8
	}
	body, _ := st.askRows(th, askInnerWidth(width), method)
	return max(len(body)+2, 8)
}

// askRows renders the ask and reports how many trailing rows are its answer
// section — the options, or the feedback prompt. A panel too short for the
// whole body drops detail rows; those trailing rows are what it must keep.
func (st *permAskState) askRows(
	th components.Theme,
	innerW int,
	method xui.WidthMethod,
) (body []components.RichLine, answer int) {
	primary := askPrimary(th)
	add := func(spans ...components.Span) {
		body = append(body, components.WrapSpans(spans, innerW, method)...)
	}

	add(components.Span{Text: st.header, Style: th.Foreground})
	body = append(body, st.detailLines(th, innerW, method)...)
	if st.reason != "" {
		add(components.Span{Text: "(" + st.reason + ")", Style: th.Muted})
	}
	body = append(body, components.RichLine{})

	var rows []components.RichLine
	if st.feedbackMode {
		rows = st.feedbackLines(th, primary, innerW, method)
	} else {
		rows = st.optionLines(th, primary, innerW, method)
	}
	return append(body, rows...), len(rows)
}

func (st *continueAskState) preferredAskHeight(th components.Theme, width int, method xui.WidthMethod) int {
	if st == nil {
		return 8
	}
	return max(len(st.askRows(th, askInnerWidth(width), method))+2, 8)
}

func (st *continueAskState) askRows(th components.Theme, innerW int, method xui.WidthMethod) []components.RichLine {
	primary := askPrimary(th)
	var body []components.RichLine
	body = append(body, components.WrapSpans([]components.Span{
		{
			Text:  fmt.Sprintf("Reached max tool rounds (%d). Continue for another %d?", st.maxRounds, st.maxRounds),
			Style: th.Foreground,
		},
	}, innerW, method)...)
	body = append(body, components.RichLine{})

	for i, label := range continueOptionLabels {
		sel := i == st.selected
		arrow := " "
		dot := "○"
		labelSt := th.Foreground
		dotSt := th.Muted
		if sel {
			arrow = "▸"
			dot = "●"
			labelSt = xui.Style{Bold: true, Fg: primary.Fg}
			dotSt = primary
		}
		body = append(body, components.WrapSpans([]components.Span{
			{Text: arrow, Style: primary},
			{Text: dot, Style: dotSt},
			{Text: " " + label, Style: labelSt},
			{Text: fmt.Sprintf(" [%d]", i+1), Style: th.Muted},
		}, innerW, method)...)
	}
	hint := fmt.Sprintf("1-%d or y/n • ↑↓ move • Enter select • Esc stop", len(continueOptionLabels))
	hintSt := th.Muted
	if st.hint != "" {
		hint, hintSt = st.hint, th.Warning
	}
	body = append(body, components.WrapSpans([]components.Span{
		{Text: hint, Style: hintSt},
	}, innerW, method)...)
	return body
}

func (st *permAskState) detailLines(th components.Theme, innerW int, method xui.WidthMethod) []components.RichLine {
	if st.detail == "" {
		return nil
	}
	lines := strings.Split(st.detail, "\n")
	out := make([]components.RichLine, 0, len(lines))
	for i, line := range lines {
		var spans []components.Span
		switch {
		case st.req.Action == permission.ActionBash && i == 0:
			spans = []components.Span{
				{Text: "$ ", Style: xui.Style{Bold: true, Fg: th.Success.Fg}},
				{Text: line, Style: th.Foreground},
			}
		case st.req.Action == permission.ActionBash:
			spans = []components.Span{{Text: "  " + line, Style: th.Foreground}}
		default:
			spans = []components.Span{{Text: line, Style: xui.Style{Bold: true, Fg: th.Foreground.Fg}}}
		}
		out = append(out, components.WrapSpans(spans, innerW, method)...)
	}
	return out
}

func (st *permAskState) optionLines(
	th components.Theme,
	primary xui.Style,
	innerW int,
	method xui.WidthMethod,
) []components.RichLine {
	out := make([]components.RichLine, 0, len(askOptionLabels)+1)
	for i, label := range askOptionLabels {
		sel := i == st.selected
		arrow, dot := " ", "○"
		labelSt, dotSt := th.Foreground, th.Muted
		if sel {
			arrow, dot = "▸", "●"
			labelSt = xui.Style{Bold: true, Fg: primary.Fg}
			dotSt = primary
		}
		out = append(out, components.WrapSpans([]components.Span{
			{Text: arrow, Style: primary},
			{Text: dot, Style: dotSt},
			{Text: " " + label, Style: labelSt},
			{Text: fmt.Sprintf(" [%d]", i+1), Style: th.Muted},
		}, innerW, method)...)
	}
	// Esc denies the call; it does not put the ask back for later. Calling
	// that "cancel" taught a reflex the tool never honored.
	hint := fmt.Sprintf("1-%d or y/n • ↑↓ move • Enter select • Esc deny", len(askOptionLabels))
	hintSt := th.Muted
	if st.hint != "" {
		hint, hintSt = st.hint, th.Warning
	}
	out = append(out, components.WrapSpans([]components.Span{
		{Text: hint, Style: hintSt},
	}, innerW, method)...)
	return out
}

func (st *permAskState) feedbackLines(
	th components.Theme,
	primary xui.Style,
	innerW int,
	method xui.WidthMethod,
) []components.RichLine {
	var out []components.RichLine
	out = append(out, components.WrapSpans([]components.Span{
		{Text: "✗ ", Style: th.Destructive},
		{Text: "Denied", Style: xui.Style{Bold: true, Fg: th.Destructive.Fg}},
		{Text: " — tell CozyPhi what to do instead", Style: th.Muted},
	}, innerW, method)...)
	// The field scrolls instead of wrapping: a growing prompt would shove the
	// hint row out of a panel whose height was already measured.
	out = append(out, components.RichLine{
		{Text: "› ", Style: xui.Style{Bold: true, Fg: primary.Fg}},
		{Text: st.feedback.Display(innerW-2, method), Style: th.Foreground},
	})
	out = append(out, components.WrapSpans([]components.Span{
		{Text: "Enter send  •  Esc cancel", Style: th.Muted},
	}, innerW, method)...)
	return out
}

func paintAskPanel(
	body []components.RichLine,
	width, height int,
	border xui.Style,
	method xui.WidthMethod,
) components.Surface {
	panel := components.NewSurface(width, height, nil)
	layout.DrawRoundedBorder(&panel, layout.BorderRounded, border, nil, nil, nil, nil, method)
	y := 1
	for _, line := range body {
		if y >= height-1 {
			break
		}
		components.PaintSpans(&panel, 2, y, line, method)
		y++
	}
	return panel
}
