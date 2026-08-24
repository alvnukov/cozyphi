package overlays

import (
	"fmt"
	"strings"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/layout"
	"github.com/pulseaiclub/phi/internal/permission"
	"github.com/pulseaiclub/phi/internal/tui/controller"
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
	return o != nil && (o.perm != nil || o.cont != nil || o.connect != nil)
}

// PermissionActive reports whether the permission overlay is showing.
func (o *Overlays) PermissionActive() bool {
	return o != nil && o.perm != nil
}

// ContinueActive reports whether the continue overlay is showing.
func (o *Overlays) ContinueActive() bool {
	return o != nil && o.cont != nil
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

// PreferredBottomHeight estimates rows for the bottom overlay or composer slot.
func (o *Overlays) PreferredBottomHeight(width int, method xui.WidthMethod) (height int, overlay bool) {
	if o == nil {
		return 0, false
	}
	if o.perm != nil {
		return o.perm.preferredAskHeight(width, method), true
	}
	if o.cont != nil {
		return o.cont.preferredAskHeight(), true
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
	if o.connect != nil {
		return o.drawConnect(ctx, width, height), true
	}
	return components.Surface{}, false
}

func (o *Overlays) beginPermissionAsk(msg controller.PermissionAskMsg) {
	if o.perm != nil {
		o.resolvePermission(controller.AskReply{})
	}
	if o.cont != nil {
		o.resolveContinue(controller.ContinueReply{})
	}
	o.clearConnect()
	if o.composer != nil {
		o.composer.HideCompleters()
		o.composer.HidePalette()
	}
	o.perm = newPermAskState(msg.Request, msg.Reason, msg.Reply)
	o.activity.Apply(controller.ActivityAwaitingApproval)
	if o.focusEditor != nil {
		o.focusEditor()
	}
}

func (o *Overlays) dismissPermission() {
	wasAsk := o.perm != nil
	o.perm = nil
	if !wasAsk {
		return
	}
	if o.activity != nil && o.activity.Current == controller.ActivityAwaitingApproval {
		o.activity.Apply(controller.ActivityTools)
	}
	if o.focusChat != nil {
		o.focusChat()
	}
}

func (o *Overlays) resolvePermission(r controller.AskReply) {
	st := o.perm
	if st == nil {
		return
	}
	o.perm = nil
	if o.activity != nil && o.activity.Current == controller.ActivityAwaitingApproval {
		o.activity.Apply(controller.ActivityTools)
	}
	if o.focusChat != nil {
		o.focusChat()
	}
	if st.reply != nil {
		select {
		case st.reply <- r:
		default:
		}
	}
}

func (o *Overlays) beginContinueAsk(msg controller.ContinueAskMsg) {
	if o.cont != nil {
		o.resolveContinue(controller.ContinueReply{})
	}
	if o.perm != nil {
		o.resolvePermission(controller.AskReply{})
	}
	o.clearConnect()
	if o.composer != nil {
		o.composer.HideCompleters()
		o.composer.HidePalette()
	}
	o.cont = newContinueAskState(msg.MaxRounds, msg.Reply)
	o.activity.Apply(controller.ActivityAwaitingApproval)
	if o.focusEditor != nil {
		o.focusEditor()
	}
}

func (o *Overlays) dismissContinue() {
	wasAsk := o.cont != nil
	o.cont = nil
	if !wasAsk {
		return
	}
	if o.activity != nil && o.activity.Current == controller.ActivityAwaitingApproval {
		o.activity.Apply(controller.ActivityTools)
	}
	if o.focusChat != nil {
		o.focusChat()
	}
}

func (o *Overlays) resolveContinue(r controller.ContinueReply) {
	st := o.cont
	if st == nil {
		return
	}
	o.cont = nil
	if o.activity != nil && o.activity.Current == controller.ActivityAwaitingApproval {
		o.activity.Apply(controller.ActivityTools)
	}
	if o.focusChat != nil {
		o.focusChat()
	}
	if st.reply != nil {
		select {
		case st.reply <- r:
		default:
		}
	}
}

func (o *Overlays) handlePermissionKey(ctx *components.EventContext, e xui.KeyEvent) bool {
	st := o.perm
	if st == nil || !e.Press {
		return false
	}

	if st.feedbackMode {
		return o.handlePermissionFeedbackKey(ctx, e)
	}

	if e.Mods.Has(xui.ModAlt) && e.Code == xui.KeyRune && e.Rune >= '1' && e.Rune <= '9' {
		idx := int(e.Rune - '1')
		if idx < len(askOptionLabels) {
			o.acceptPermissionOption(askOption(idx))
			ctx.ConsumeAndRedraw()
			return true
		}
	}

	switch e.Code {
	case xui.KeyEscape:
		o.resolvePermission(controller.AskReply{})
		ctx.ConsumeAndRedraw()
		return true
	case xui.KeyUp:
		if st.selected > 0 {
			st.selected--
		} else {
			st.selected = len(askOptionLabels) - 1
		}
		ctx.ConsumeAndRedraw()
		return true
	case xui.KeyDown, xui.KeyTab:
		st.selected = (st.selected + 1) % len(askOptionLabels)
		ctx.ConsumeAndRedraw()
		return true
	case xui.KeyEnter:
		o.acceptPermissionOption(askOption(st.selected))
		ctx.ConsumeAndRedraw()
		return true
	case xui.KeyRune:
		if e.Mods.Has(xui.ModCtrl) || e.Mods.Has(xui.ModAlt) {
			ctx.ConsumeAndRedraw()
			return true
		}
		switch e.Rune {
		case 'k', 'K':
			if st.selected > 0 {
				st.selected--
			}
			ctx.ConsumeAndRedraw()
			return true
		case 'j', 'J':
			st.selected = (st.selected + 1) % len(askOptionLabels)
			ctx.ConsumeAndRedraw()
			return true
		}
	}
	ctx.ConsumeAndRedraw()
	return true
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
		st.feedback = ""
		st.feedbackCur = 0
	}
}

func (o *Overlays) handlePermissionFeedbackKey(ctx *components.EventContext, e xui.KeyEvent) bool {
	st := o.perm
	if st == nil {
		return false
	}
	switch e.Code {
	case xui.KeyEscape:
		st.feedbackMode = false
		st.feedback = ""
		st.feedbackCur = 0
		ctx.ConsumeAndRedraw()
		return true
	case xui.KeyEnter:
		fb := strings.TrimSpace(st.feedback)
		o.resolvePermission(controller.AskReply{Feedback: fb})
		ctx.ConsumeAndRedraw()
		return true
	case xui.KeyBackspace:
		runes := []rune(st.feedback)
		if st.feedbackCur > 0 && st.feedbackCur <= len(runes) {
			st.feedback = string(append(runes[:st.feedbackCur-1], runes[st.feedbackCur:]...))
			st.feedbackCur--
		}
		ctx.ConsumeAndRedraw()
		return true
	case xui.KeyLeft:
		if st.feedbackCur > 0 {
			st.feedbackCur--
		}
		ctx.ConsumeAndRedraw()
		return true
	case xui.KeyRight:
		if st.feedbackCur < len([]rune(st.feedback)) {
			st.feedbackCur++
		}
		ctx.ConsumeAndRedraw()
		return true
	case xui.KeyRune:
		if e.Mods.Has(xui.ModCtrl) || e.Mods.Has(xui.ModAlt) {
			ctx.ConsumeAndRedraw()
			return true
		}
		runes := []rune(st.feedback)
		ch := string(e.Rune)
		st.feedback = string(append(runes[:st.feedbackCur], append([]rune(ch), runes[st.feedbackCur:]...)...))
		st.feedbackCur++
		ctx.ConsumeAndRedraw()
		return true
	}
	ctx.ConsumeAndRedraw()
	return true
}

func (o *Overlays) handleContinueKey(ctx *components.EventContext, e xui.KeyEvent) bool {
	st := o.cont
	if st == nil || !e.Press {
		return false
	}

	if e.Mods.Has(xui.ModAlt) && e.Code == xui.KeyRune && e.Rune >= '1' && e.Rune <= '2' {
		idx := int(e.Rune - '1')
		o.acceptContinueOption(idx)
		ctx.ConsumeAndRedraw()
		return true
	}

	switch e.Code {
	case xui.KeyEscape:
		o.resolveContinue(controller.ContinueReply{})
		ctx.ConsumeAndRedraw()
		return true
	case xui.KeyUp:
		if st.selected > 0 {
			st.selected--
		} else {
			st.selected = len(continueOptionLabels) - 1
		}
		ctx.ConsumeAndRedraw()
		return true
	case xui.KeyDown, xui.KeyTab:
		st.selected = (st.selected + 1) % len(continueOptionLabels)
		ctx.ConsumeAndRedraw()
		return true
	case xui.KeyEnter:
		o.acceptContinueOption(st.selected)
		ctx.ConsumeAndRedraw()
		return true
	case xui.KeyRune:
		if e.Mods.Has(xui.ModCtrl) || e.Mods.Has(xui.ModAlt) {
			ctx.ConsumeAndRedraw()
			return true
		}
		switch e.Rune {
		case 'k', 'K':
			if st.selected > 0 {
				st.selected--
			}
			ctx.ConsumeAndRedraw()
			return true
		case 'j', 'J':
			st.selected = (st.selected + 1) % len(continueOptionLabels)
			ctx.ConsumeAndRedraw()
			return true
		}
	}
	ctx.ConsumeAndRedraw()
	return true
}

func (o *Overlays) acceptContinueOption(idx int) {
	switch idx {
	case 0:
		o.resolveContinue(controller.ContinueReply{Continue: true})
	default:
		o.resolveContinue(controller.ContinueReply{})
	}
}

func (o *Overlays) drawPermissionAsk(ctx components.DrawContext, width, height int) components.Surface {
	st := o.perm
	if st == nil {
		return components.NewSurface(width, height, nil)
	}
	th := o.theme
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = st.preferredAskHeight(width, ctx.Method)
	}
	innerW := width - 4
	if innerW < 10 {
		innerW = width
	}

	primary := th.Success
	if th.ToolName.Fg.Kind != 0 {
		primary = th.ToolName
	}

	var body []components.RichLine
	add := func(spans ...components.Span) {
		body = append(body, components.WrapSpans(spans, innerW, ctx.Method)...)
	}

	add(components.Span{Text: st.header, Style: th.Foreground})
	body = append(body, st.detailLines(th, innerW, ctx.Method)...)
	if st.reason != "" {
		add(components.Span{Text: "(" + st.reason + ")", Style: th.Muted})
	}
	body = append(body, components.RichLine{})

	if st.feedbackMode {
		body = append(body, st.feedbackLines(th, primary, innerW, ctx.Method)...)
	} else {
		body = append(body, st.optionLines(th, primary, innerW, ctx.Method)...)
	}

	return paintAskPanel(body, width, height, th.Warning, ctx.Method)
}

func (o *Overlays) drawContinueAsk(ctx components.DrawContext, width, height int) components.Surface {
	st := o.cont
	if st == nil {
		return components.NewSurface(width, height, nil)
	}
	th := o.theme
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = st.preferredAskHeight()
	}
	innerW := width - 4
	if innerW < 10 {
		innerW = width
	}

	warn := th.Warning
	primary := th.Success
	if th.ToolName.Fg.Kind != 0 {
		primary = th.ToolName
	}

	var body []components.RichLine
	body = append(body, components.WrapSpans([]components.Span{
		{
			Text:  fmt.Sprintf("Reached max tool rounds (%d). Continue for another %d?", st.maxRounds, st.maxRounds),
			Style: th.Foreground,
		},
	}, innerW, ctx.Method)...)
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
		shortcut := fmt.Sprintf(" [Alt+%d]", i+1)
		body = append(body, components.WrapSpans([]components.Span{
			{Text: arrow, Style: primary},
			{Text: dot, Style: dotSt},
			{Text: " " + label, Style: labelSt},
			{Text: shortcut, Style: th.Muted},
		}, innerW, ctx.Method)...)
	}
	body = append(body, components.WrapSpans([]components.Span{
		{Text: "↑↓ navigate • Enter select • Esc stop", Style: th.Muted},
	}, innerW, ctx.Method)...)

	return paintAskPanel(body, width, height, warn, ctx.Method)
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
	feedback     string
	feedbackCur  int
}

type continueAskState struct {
	maxRounds int
	reply     chan controller.ContinueReply
	selected  int
}

func formatAskHeader(req permission.Request) (header, detail string) {
	switch req.Action {
	case permission.ActionBash:
		cmd := req.Command
		lines := strings.Split(cmd, "\n")
		if len(lines) > 3 {
			cmd = strings.Join(lines[:3], "\n") + "\n..."
		}
		return "Run this command?", cmd
	case permission.ActionEdit:
		path := ""
		if len(req.Paths) > 0 {
			path = req.Paths[0]
		}
		return "Allow editing file:", path
	case permission.ActionWrite:
		path := ""
		if len(req.Paths) > 0 {
			path = req.Paths[0]
		}
		return "Allow creating file:", path
	default:
		return fmt.Sprintf("Invoke tool %s?", req.Tool), permission.Summarize(req)
	}
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

func (st *permAskState) preferredAskHeight(width int, method xui.WidthMethod) int {
	if st == nil {
		return 8
	}
	innerW := width - 4
	innerW = max(innerW, 20)
	h := 2
	h++
	if st.detail != "" {
		h += strings.Count(st.detail, "\n") + 1
	}
	if st.reason != "" {
		h++
	}
	h++
	if st.feedbackMode {
		h += 3
	} else {
		h += len(askOptionLabels)
		h++
	}
	h++
	if h < 8 {
		h = 8
	}
	_ = method
	_ = innerW
	return h
}

func (*continueAskState) preferredAskHeight() int {
	h := 2 + 1 + 1 + len(continueOptionLabels) + 1 + 1
	h = max(h, 8)
	return h
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
			{Text: fmt.Sprintf(" [Alt+%d]", i+1), Style: th.Muted},
		}, innerW, method)...)
	}
	out = append(out, components.WrapSpans([]components.Span{
		{Text: "↑↓ navigate • Enter select • Esc cancel", Style: th.Muted},
	}, innerW, method)...)
	return out
}

func (st *permAskState) feedbackLines(
	th components.Theme,
	primary xui.Style,
	innerW int,
	method xui.WidthMethod,
) []components.RichLine {
	runes := []rune(st.feedback)
	if st.feedbackCur > len(runes) {
		st.feedbackCur = len(runes)
	}
	shown := string(runes[:st.feedbackCur]) + "▎" + string(runes[st.feedbackCur:])
	var out []components.RichLine
	out = append(out, components.WrapSpans([]components.Span{
		{Text: "✗ ", Style: th.Destructive},
		{Text: "Denied", Style: xui.Style{Bold: true, Fg: th.Destructive.Fg}},
		{Text: " — tell Phi what to do instead", Style: th.Muted},
	}, innerW, method)...)
	out = append(out, components.WrapSpans([]components.Span{
		{Text: "› ", Style: xui.Style{Bold: true, Fg: primary.Fg}},
		{Text: shown, Style: th.Foreground},
	}, innerW, method)...)
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
