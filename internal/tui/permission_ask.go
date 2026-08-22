package tui

import (
	"fmt"
	"strings"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/layout"
	"github.com/pulseaiclub/phi/internal/permission"
	"github.com/pulseaiclub/phi/internal/tui/controller"
)

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

// permAskState holds the permission confirmation UI that replaces the chat composer.
type permAskState struct {
	req    permission.Request
	reason string
	reply  chan controller.AskReply

	header       string
	detail       string // command / path / url
	selected     int
	feedbackMode bool
	feedback     string
	feedbackCur  int
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

func (editor *Editor) beginPermissionAsk(msg controller.PermissionAskMsg) {
	if editor.permAsk != nil {
		editor.resolvePermission(controller.AskReply{})
	}
	if editor.continueAsk != nil {
		editor.resolveContinue(controller.ContinueReply{})
	}
	editor.composer.HideCompleters()
	editor.composer.HidePalette()
	editor.permAsk = newPermAskState(msg.Request, msg.Reason, msg.Reply)
	editor.activity.Apply(controller.ActivityAwaitingApproval)
	// Steal focus from Chat so ↑↓ reach handlePermissionKey (Chat would Consume them).
	if editor.App != nil {
		editor.App.RequestFocus(editor)
	}
}

func (editor *Editor) resolvePermission(r controller.AskReply) {
	st := editor.permAsk
	if st == nil {
		return
	}
	editor.permAsk = nil
	if editor.activity.Current == controller.ActivityAwaitingApproval {
		editor.activity.Apply(controller.ActivityTools)
	}
	if editor.App != nil {
		editor.composer.FocusChat()
	}
	if st.reply != nil {
		select {
		case st.reply <- r:
		default:
		}
	}
}

func (editor *Editor) handlePermissionKey(ctx *components.EventContext, e xui.KeyEvent) bool {
	st := editor.permAsk
	if st == nil || !e.Press {
		return false
	}

	if st.feedbackMode {
		return editor.handlePermissionFeedbackKey(ctx, e)
	}

	// Alt+1..Alt+4 select option directly.
	if e.Mods.Has(xui.ModAlt) && e.Code == xui.KeyRune && e.Rune >= '1' && e.Rune <= '9' {
		idx := int(e.Rune - '1')
		if idx < len(askOptionLabels) {
			editor.acceptPermissionOption(askOption(idx))
			ctx.ConsumeAndRedraw()
			return true
		}
	}

	switch e.Code {
	case xui.KeyEscape:
		editor.resolvePermission(controller.AskReply{})
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
		editor.acceptPermissionOption(askOption(st.selected))
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

func (editor *Editor) acceptPermissionOption(opt askOption) {
	st := editor.permAsk
	if st == nil {
		return
	}
	switch opt {
	case askOptApprove:
		editor.resolvePermission(controller.AskReply{Approved: true})
	case askOptAllowSession:
		editor.resolvePermission(controller.AskReply{Approved: true, AllowSession: true})
	case askOptAllowPersistent:
		editor.resolvePermission(controller.AskReply{Approved: true, AllowPersistent: true})
	case askOptDenyFeedback:
		st.feedbackMode = true
		st.feedback = ""
		st.feedbackCur = 0
	}
}

func (editor *Editor) handlePermissionFeedbackKey(ctx *components.EventContext, e xui.KeyEvent) bool {
	st := editor.permAsk
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
		editor.resolvePermission(controller.AskReply{Feedback: fb})
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

// preferredAskHeight estimates rows needed for the bottom confirmation panel.
func (st *permAskState) preferredAskHeight(width int, method xui.WidthMethod) int {
	if st == nil {
		return 8
	}
	innerW := width - 4
	innerW = max(innerW, 20)
	h := 2 // top/bottom border
	h++    // header
	if st.detail != "" {
		h += strings.Count(st.detail, "\n") + 1
	}
	if st.reason != "" {
		h++
	}
	h++ // blank
	if st.feedbackMode {
		h += 3 // denied line + input + hint
	} else {
		h += len(askOptionLabels)
		h++ // navigate hint
	}
	h++ // padding
	if h < 8 {
		h = 8
	}
	_ = method
	_ = innerW
	return h
}

// drawPermissionAsk draws the permission confirmation in the composer slot (full width).
func (editor *Editor) drawPermissionAsk(ctx components.DrawContext, width, height int) components.Surface {
	st := editor.permAsk
	if st == nil {
		return components.NewSurface(width, height, nil)
	}
	th := editor.theme
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

	primary := th.Success // selection accent; ToolName overrides when set
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

// detailLines renders st.detail; bash commands are shown with a "$ " prompt.
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

// optionLines renders the selectable options and the navigation hint.
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

// feedbackLines renders the denied-feedback editor.
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

// paintAskPanel paints body lines inside a rounded border, clipped to height.
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
