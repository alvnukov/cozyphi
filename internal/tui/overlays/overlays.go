package overlays

import (
	"fmt"
	"strings"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/input"
	"github.com/alvnukov/cozyphi/internal/components/layout"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/tui/browse"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/keys"
	"github.com/alvnukov/cozyphi/internal/tui/pathutil"
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

	// askResolved reports an answered ask to whoever routed it here, by the
	// owner it was stamped with. Unwired, an overlay simply answers for
	// itself — which is what a lone session wants.
	askResolved func(owner string)

	// panel* is the bottom panel's on-screen rectangle from the latest
	// frame, recorded at draw time so a mouse event can be traced back to
	// the panel row it landed on.
	panelX, panelY, panelW, panelH int
	panelMethod                    xui.WidthMethod
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

// Withdraw takes back every ask this overlay is showing for one session
// without answering it: nothing is sent on the reply channel, so the call
// stays blocked and the ask can be put up somewhere else. It is how an ask
// follows the user from one screen to the next; answering it is Escape's job,
// and ending it for good belongs to whoever owns the call.
func (o *Overlays) Withdraw(owner string) bool {
	if o == nil {
		return false
	}
	taken := false
	if o.perm != nil && o.perm.origin.Owner == owner {
		o.perm, taken = nil, true
	}
	if o.cont != nil && o.cont.origin.Owner == owner {
		o.cont, taken = nil, true
	}
	if o.question != nil && o.question.origin.Owner == owner {
		o.question, taken = nil, true
	}
	o.endAsk(taken)
	return taken
}

// SetAskResolved installs the seam that reports an ask answered here — by a
// key, by a click or by the interrupt path. The owner is the one the ask was
// stamped with, so whoever routed it can tell which of its asks is done.
// Withdraw stays silent: taking an ask back is not answering it.
func (o *Overlays) SetAskResolved(fn func(owner string)) {
	if o != nil {
		o.askResolved = fn
	}
}

// askAnswered reports one resolved ask to the router that placed it.
func (o *Overlays) askAnswered(origin AskOrigin) {
	if o.askResolved != nil {
		o.askResolved(origin.Owner)
	}
}

// dismissAll resolves every ask with an empty reply and drops the connect
// flow, leaving no overlay showing.
func (o *Overlays) dismissAll() {
	o.resolvePermission(controller.AskReply{})
	o.resolveContinue(controller.ContinueReply{})
	o.resolveQuestion(controller.QuestionReply{})
	o.clearConnect()
}

// AskOrigin names the session an ask came from when that session is not the
// one whose overlay shows it. A sub-agent has no screen of its own while it
// is hidden, so its asks are shown here on its behalf.
type AskOrigin struct {
	// Owner identifies the asking session; "" means the ask is this
	// overlay's own. A dismissal only lands on an ask its owner started,
	// so two sessions cannot cancel each other's questions.
	Owner string
	// Label is what the header wears in brackets — role(description) for a
	// sub-agent, the parent's own name for the session that owns them. It is
	// empty when the asking session is the one on screen: the ask needs no
	// label to say whose it is when the user is looking at it.
	Label string
	// Child marks an ask a sub-agent raised. The permanent allow-all is not
	// offered for one: that rule outlives every session, and the user is
	// answering here on behalf of a session they did not open. The parent's
	// own ask keeps it, whichever screen it is being answered on.
	Child bool
}

// Apply routes overlay-related bus messages this session raised itself.
func (o *Overlays) Apply(m controller.Msg) { o.ApplyFrom(m, AskOrigin{}) }

// ApplyFrom routes overlay-related bus messages, stamping asks with the
// session they came from. Everything but the three asks belongs to the
// session that owns this overlay and ignores the origin.
func (o *Overlays) ApplyFrom(m controller.Msg, from AskOrigin) {
	if o == nil {
		return
	}
	switch msg := m.(type) {
	case controller.PermissionAskMsg:
		o.beginPermissionAsk(msg, from)
	case controller.PermissionDismissMsg:
		o.dismissPermission(from)
	case controller.ContinueAskMsg:
		o.beginContinueAsk(msg, from)
	case controller.ContinueDismissMsg:
		o.dismissContinue(from)
	case controller.QuestionAskMsg:
		o.beginQuestionAsk(msg, from)
	case controller.QuestionDismissMsg:
		o.dismissQuestion(from)
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
	o.panelW, o.panelH, o.panelMethod = width, height, ctx.Method
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

func (o *Overlays) beginPermissionAsk(msg controller.PermissionAskMsg, from AskOrigin) {
	o.beginAsk()
	o.perm = newPermAskState(msg, from)
}

func (o *Overlays) dismissPermission(from AskOrigin) {
	st := o.perm
	if st == nil || st.origin.Owner != from.Owner {
		return
	}
	o.perm = nil
	o.endAsk(true)
}

func (o *Overlays) resolvePermission(r controller.AskReply) {
	st := o.perm
	o.perm = nil
	o.endAsk(st != nil)
	if st != nil {
		sendReply(st.reply, r)
		o.askAnswered(st.origin)
	}
}

func (o *Overlays) beginContinueAsk(msg controller.ContinueAskMsg, from AskOrigin) {
	o.beginAsk()
	o.cont = newContinueAskState(msg.MaxRounds, msg.Reply, from)
}

func (o *Overlays) dismissContinue(from AskOrigin) {
	st := o.cont
	if st == nil || st.origin.Owner != from.Owner {
		return
	}
	o.cont = nil
	o.endAsk(true)
}

func (o *Overlays) resolveContinue(r controller.ContinueReply) {
	st := o.cont
	o.cont = nil
	o.endAsk(st != nil)
	if st != nil {
		sendReply(st.reply, r)
		o.askAnswered(st.origin)
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

	if st.confirm.Armed() {
		if st.confirm.Key(e) {
			st.hint = ""
			ctx.ConsumeAndRedraw()
			return true
		}
		// The question is withdrawn; the key still acts below — acting
		// elsewhere is how a y/n question is abandoned everywhere.
	}

	if o.applyPermissionKey(st, e) {
		st.hint = ""
	} else {
		st.hint = unboundKeyHint(len(st.options()))
	}
	ctx.ConsumeAndRedraw()
	return true
}

// applyPermissionKey reports whether e did something. A digit picks its option
// with or without Alt — a panel that prints "[1]" has to honor a bare 1 — and
// y/n answer outright the two cases worth a single keystroke.
func (o *Overlays) applyPermissionKey(st *permAskState, e xui.KeyEvent) bool {
	if e.Code == xui.KeyRune && e.Rune >= '1' && e.Rune <= '9' && !e.Mods.Has(xui.ModCtrl) {
		opt, ok := st.option(int(e.Rune - '1'))
		if !ok {
			return false
		}
		o.acceptPermissionOption(opt)
		return true
	}
	if st.expanded && st.applyDetailKey(e) {
		return true
	}

	switch e.Code {
	case xui.KeyEscape:
		// Esc backs out one level: an open detail folds down first, and
		// only a second Esc denies the request.
		if st.expanded {
			st.expanded = false
			return true
		}
		o.resolvePermission(controller.AskReply{})
		return true
	case xui.KeyUp:
		st.ring.Step(-1)
		return true
	case xui.KeyDown, xui.KeyTab:
		st.ring.Step(1)
		return true
	case xui.KeyEnter:
		o.acceptPermissionOption(st.selected())
		return true
	case xui.KeyRune:
		if e.Mods.Has(xui.ModCtrl) || e.Mods.Has(xui.ModAlt) {
			return false
		}
		switch e.HotkeyRune() {
		case ' ':
			o.acceptPermissionOption(st.selected())
			return true
		case 'k', 'K':
			st.ring.Step(-1)
			return true
		case 'j', 'J':
			st.ring.Step(1)
			return true
		case 'y', 'Y':
			o.acceptPermissionOption(askOptApprove)
			return true
		case 'n', 'N':
			o.resolvePermission(controller.AskReply{})
			return true
		case 'v', 'V':
			st.expanded = !st.expanded
			st.detailScroll = 0
			return true
		}
	}
	return false
}

// applyDetailKey scrolls the expanded detail. While the detail is open the
// row motions belong to the text; digits, y/n, Space and Enter still
// answer with the current selection, so reading never blocks deciding.
// The scroll is clamped at render, where the row count is known.
func (st *permAskState) applyDetailKey(e xui.KeyEvent) bool {
	switch e.Code {
	case xui.KeyUp:
		st.detailScroll--
		return true
	case xui.KeyDown:
		st.detailScroll++
		return true
	case xui.KeyRune:
		if e.Mods.Has(xui.ModCtrl) || e.Mods.Has(xui.ModAlt) {
			return false
		}
		switch e.HotkeyRune() {
		case 'k', 'K':
			st.detailScroll--
			return true
		case 'j', 'J':
			st.detailScroll++
			return true
		}
	}
	return false
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
		// A silent permanent grant is the one choice worth a second beat:
		// the armed question names the file the rule lands in, and only y
		// writes it (the standard's Deletion-and-confirmation shape).
		st.confirm.Arm(
			"Turn off permission asks for every session, writing "+st.persistDisplay()+"?",
			func() {
				o.resolvePermission(controller.AskReply{Approved: true, AllowPersistent: true})
			},
		)
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
		st.ring.Step(-1)
		return true
	case xui.KeyDown, xui.KeyTab:
		st.ring.Step(1)
		return true
	case xui.KeyEnter:
		o.acceptContinueOption(st.ring.Selected())
		return true
	case xui.KeyRune:
		if e.Mods.Has(xui.ModCtrl) || e.Mods.Has(xui.ModAlt) {
			return false
		}
		switch e.HotkeyRune() {
		case ' ':
			o.acceptContinueOption(st.ring.Selected())
			return true
		case 'k', 'K':
			st.ring.Step(-1)
			return true
		case 'j', 'J':
			st.ring.Step(1)
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
	body, _ := st.askRows(o.theme, askInnerWidth(width), ctx.Method)
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

// ownAskOptions is what a session offers for its own call. A sub-agent's ask
// drops the permanent grant: that rule is written to the global config and
// would outlive every session, and a child's ask is the one place the user
// is answering for a session they did not open. Its session-wide grant stays,
// bound to the child's controller inside its role ceiling.
var (
	ownAskOptions   = []askOption{askOptApprove, askOptAllowSession, askOptAllowPersistent, askOptDenyFeedback}
	childAskOptions = []askOption{askOptApprove, askOptAllowSession, askOptDenyFeedback}
)

// askOriginSpans is the bracketed label a routed ask wears before its header,
// so an ask that arrived from elsewhere says whose call it is.
func askOriginSpans(origin AskOrigin, th components.Theme) []components.Span {
	if origin.Label == "" {
		return nil
	}
	return []components.Span{{Text: "[" + origin.Label + "] ", Style: th.Warning}}
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
	ring         browse.Ring
	feedbackMode bool
	feedback     input.Line

	// expanded opens the full detail for scrolling; detailScroll is the
	// first detail row the expanded window shows. The full detail always
	// lives in state — the clip is a render decision, so expanding never
	// has to re-ask anyone for the rest.
	expanded     bool
	detailScroll int

	// persistPath is the config file the persistent allow-all would write;
	// confirm arms the y/n question that guards that write.
	persistPath string
	confirm     browse.Confirm

	// hint replaces the standard key hint after a key the ask cannot use.
	hint string

	// origin names the session that asked when it is not the one whose
	// overlay this is.
	origin AskOrigin
}

// options is the answer list this ask offers, in the order it draws them.
func (st *permAskState) options() []askOption {
	if st.origin.Child {
		return childAskOptions
	}
	return ownAskOptions
}

// option is the choice drawn at idx, which is not the option's own value
// once an ask leaves one out.
func (st *permAskState) option(idx int) (askOption, bool) {
	opts := st.options()
	if idx < 0 || idx >= len(opts) {
		return askOptApprove, false
	}
	return opts[idx], true
}

// selected is the option the ring highlights.
func (st *permAskState) selected() askOption {
	opt, _ := st.option(st.ring.Selected())
	return opt
}

type continueAskState struct {
	maxRounds int
	reply     chan controller.ContinueReply
	ring      browse.Ring

	// hint replaces the standard key hint after a key the ask cannot use.
	hint string

	// origin names the session that asked when it is not the one whose
	// overlay this is.
	origin AskOrigin
}

// askDetailLines is the detail window an ask paints at once. Three lines
// used to hide the redirect at the end of a heredoc — the part actually
// worth approving — while twelve still leaves the options room on any
// usable terminal. Longer detail is windowed at render time, never cut
// from state: v expands it and the window scrolls.
const askDetailLines = 12

func describeAsk(req permission.Request) (header, detail string) {
	switch req.Action {
	case permission.ActionBash:
		return "Run this command?", req.Command
	case permission.ActionEdit:
		return pathHeader("Allow editing file", req.Paths), askEvidence(req)
	case permission.ActionWrite:
		return pathHeader("Allow creating file", req.Paths), askEvidence(req)
	default:
		return fmt.Sprintf("Invoke tool %s?", req.Tool), permission.Summarize(req)
	}
}

// askEvidence is what an edit or write shows under its header: the paths,
// and under them the diff the call would apply when the executor could
// render one. Approving a change the panel never showed was the standard's
// biggest leap of faith.
func askEvidence(req permission.Request) string {
	paths := strings.Join(req.Paths, "\n")
	if req.Preview == "" {
		return paths
	}
	return paths + "\n" + req.Preview
}

// pathHeader pluralises the header, because a request that touches three files
// must not read as a request about one — every path is listed as detail.
func pathHeader(base string, paths []string) string {
	if len(paths) > 1 {
		return base + "s:"
	}
	return base + ":"
}

func newPermAskState(msg controller.PermissionAskMsg, origin AskOrigin) *permAskState {
	h, d := describeAsk(msg.Request)
	st := &permAskState{
		req:         msg.Request,
		reason:      msg.Reason,
		reply:       msg.Reply,
		header:      h,
		detail:      d,
		persistPath: msg.PersistPath,
		origin:      origin,
	}
	st.ring.SetLen(len(st.options()))
	return st
}

func newContinueAskState(
	maxRounds int,
	reply chan controller.ContinueReply,
	origin AskOrigin,
) *continueAskState {
	st := &continueAskState{
		maxRounds: maxRounds,
		reply:     reply,
		origin:    origin,
	}
	st.ring.SetLen(len(continueOptionLabels))
	return st
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

	add(append(askOriginSpans(st.origin, th), components.Span{Text: st.header, Style: th.Foreground})...)
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
	body, _ := st.askRows(th, askInnerWidth(width), method)
	return max(len(body)+2, 8)
}

// askRows renders the ask and, like the permission ask's, reports how many
// trailing rows are its answer section — the options and the hint.
func (st *continueAskState) askRows(
	th components.Theme,
	innerW int,
	method xui.WidthMethod,
) (body []components.RichLine, answer int) {
	primary := askPrimary(th)
	body = append(body, components.WrapSpans(append(askOriginSpans(st.origin, th), components.Span{
		Text:  fmt.Sprintf("Reached max tool rounds (%d). Continue for another %d?", st.maxRounds, st.maxRounds),
		Style: th.Foreground,
	}), innerW, method)...)
	body = append(body, components.RichLine{})

	prose := len(body)
	for _, block := range st.optionBlocks(th, primary, innerW, method) {
		body = append(body, block...)
	}
	hint := fmt.Sprintf("1-%d or y/n · %s", len(continueOptionLabels), keys.Hints(keys.ScopeContinue))
	hintSt := th.Muted
	if st.hint != "" {
		hint, hintSt = st.hint, th.Warning
	}
	body = append(body, components.WrapSpans([]components.Span{
		{Text: hint, Style: hintSt},
	}, innerW, method)...)
	return body, len(body) - prose
}

// optionBlocks mirrors the permission ask's: one row block per option.
func (st *continueAskState) optionBlocks(
	th components.Theme,
	primary xui.Style,
	innerW int,
	method xui.WidthMethod,
) [][]components.RichLine {
	blocks := make([][]components.RichLine, 0, len(continueOptionLabels))
	for i, label := range continueOptionLabels {
		sel := i == st.ring.Selected()
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
		blocks = append(blocks, components.WrapSpans([]components.Span{
			{Text: arrow, Style: primary},
			{Text: dot, Style: dotSt},
			{Text: " " + label, Style: labelSt},
			{Text: fmt.Sprintf(" [%d]", i+1), Style: th.Muted},
		}, innerW, method))
	}
	return blocks
}

// detailLines is the detail window the panel paints. Collapsed, it clips
// to askDetailLines rows and names the key that expands; expanded, it
// slides an askDetailLines-row window over the full detail with markers
// counting what is above and below, so a fifty-line command is readable
// end to end without the options ever leaving the panel.
func (st *permAskState) detailLines(th components.Theme, innerW int, method xui.WidthMethod) []components.RichLine {
	rows := st.detailRows(th, innerW, method)
	if len(rows) <= askDetailLines {
		st.detailScroll = 0
		return rows
	}
	if !st.expanded {
		st.detailScroll = 0
		out := make([]components.RichLine, 0, askDetailLines+1)
		out = append(out, rows[:askDetailLines]...)
		return append(out, components.WrapSpans([]components.Span{
			{Text: fmt.Sprintf("… %d more lines — press v to expand", len(rows)-askDetailLines), Style: th.Muted},
		}, innerW, method)...)
	}
	maxStart := len(rows) - askDetailLines
	st.detailScroll = min(max(st.detailScroll, 0), maxStart)
	start := st.detailScroll
	out := make([]components.RichLine, 0, askDetailLines+2)
	if start > 0 {
		out = append(out, components.WrapSpans([]components.Span{
			{Text: fmt.Sprintf("… %d lines above", start), Style: th.Muted},
		}, innerW, method)...)
	}
	out = append(out, rows[start:start+askDetailLines]...)
	if below := maxStart - start; below > 0 {
		out = append(out, components.WrapSpans([]components.Span{
			{Text: fmt.Sprintf("… %d lines below", below), Style: th.Muted},
		}, innerW, method)...)
	}
	return out
}

// detailRows renders the whole detail, one styled row set per source line:
// a command in prompt style, a diff colored by its unified-diff prefixes,
// anything else bold.
func (st *permAskState) detailRows(th components.Theme, innerW int, method xui.WidthMethod) []components.RichLine {
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
		case st.req.Preview != "":
			spans = []components.Span{{Text: line, Style: diffLineStyle(th, line)}}
		default:
			spans = []components.Span{{Text: line, Style: xui.Style{Bold: true, Fg: th.Foreground.Fg}}}
		}
		out = append(out, components.WrapSpans(spans, innerW, method)...)
	}
	return out
}

// diffLineStyle colors one preview row by its unified-diff prefix. A row
// that is not diff syntax — the path list above the hunks — stays plain.
func diffLineStyle(th components.Theme, line string) xui.Style {
	switch {
	case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
		return th.Muted
	case strings.HasPrefix(line, "+"):
		return th.Success
	case strings.HasPrefix(line, "-"):
		return th.Destructive
	case strings.HasPrefix(line, "@@"):
		return th.Secondary
	default:
		return th.Foreground
	}
}

func (st *permAskState) optionLines(
	th components.Theme,
	primary xui.Style,
	innerW int,
	method xui.WidthMethod,
) []components.RichLine {
	out := make([]components.RichLine, 0, len(st.options())+2)
	for _, block := range st.optionBlocks(th, primary, innerW, method) {
		out = append(out, block...)
	}
	out = append(out, st.explainRow(th, innerW, method)...)
	// An armed y/n question takes the hint row: it is the one thing the
	// next keypress answers. Esc denies the call otherwise; it does not
	// put the ask back for later — calling that "cancel" taught a reflex
	// the tool never honored.
	if st.confirm.Armed() {
		return append(out, components.WrapSpans([]components.Span{
			{Text: st.confirm.Label() + " (y/n)", Style: th.Warning},
		}, innerW, method)...)
	}
	scope := keys.ScopeAsk
	if st.expanded {
		scope = keys.ScopeAskDetail
	}
	hint := fmt.Sprintf("1-%d or y/n · %s", len(st.options()), keys.Hints(scope))
	hintSt := th.Muted
	if st.hint != "" {
		hint, hintSt = st.hint, th.Warning
	}
	out = append(out, components.WrapSpans([]components.Span{
		{Text: hint, Style: hintSt},
	}, innerW, method)...)
	return out
}

// explainRow subscripts the highlighted option with the rule it would
// create, so the two-beat gesture — select, then activate — always reads
// the fine print in between. The allow-alls explain in warning style:
// both are far wider grants than the single call on screen.
func (st *permAskState) explainRow(th components.Theme, innerW int, method xui.WidthMethod) []components.RichLine {
	text, warn := st.explainOption(st.selected())
	style := th.Muted
	if warn {
		style = th.Warning
	}
	return components.WrapSpans([]components.Span{
		{Text: "  " + text, Style: style},
	}, innerW, method)
}

func (st *permAskState) explainOption(opt askOption) (text string, warn bool) {
	switch opt {
	case askOptAllowSession:
		return "Stops asking for every tool and command until CozyPhi exits.", true
	case askOptAllowPersistent:
		return "Turns off permission asks in every session: writes permissions.dangerously_allow_all to " +
			st.persistDisplay() + ".", true
	case askOptDenyFeedback:
		return "Rejects the call and lets you say what to do instead.", false
	default:
		return "Runs this call once; the next call asks again.", false
	}
}

// persistDisplay names the file the persistent rule lands in; without a
// loaded project there is no file to name, only the fact of one.
func (st *permAskState) persistDisplay() string {
	if st.persistPath == "" {
		return "the global config"
	}
	return pathutil.ShortPath(st.persistPath)
}

// optionBlocks renders each option as its own row block, in option order,
// so drawing can flatten them and a mouse hit can be walked back to the
// option that owns the row.
func (st *permAskState) optionBlocks(
	th components.Theme,
	primary xui.Style,
	innerW int,
	method xui.WidthMethod,
) [][]components.RichLine {
	opts := st.options()
	blocks := make([][]components.RichLine, 0, len(opts))
	for i, opt := range opts {
		label := askOptionLabels[opt]
		sel := i == st.ring.Selected()
		arrow, dot := " ", "○"
		labelSt, dotSt := th.Foreground, th.Muted
		if sel {
			arrow, dot = "▸", "●"
			labelSt = xui.Style{Bold: true, Fg: primary.Fg}
			dotSt = primary
		}
		blocks = append(blocks, components.WrapSpans([]components.Span{
			{Text: arrow, Style: primary},
			{Text: dot, Style: dotSt},
			{Text: " " + label, Style: labelSt},
			{Text: fmt.Sprintf(" [%d]", i+1), Style: th.Muted},
		}, innerW, method))
	}
	return blocks
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
		{Text: keys.Hints(keys.ScopeAnswer), Style: th.Muted},
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
