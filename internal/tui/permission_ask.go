package tui

import (
	"fmt"
	"strings"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/layout"
	"github.com/pulseaiclub/phi/internal/permission"
	"github.com/pulseaiclub/xui"
)

// AskReply is the user's response for a gated tool confirmation.
type AskReply struct {
	Approved        bool
	Feedback        string
	AllowSession    bool // Allow All for This Session
	AllowPersistent bool // Allow All for Every Session
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

// permAskState holds the permission confirmation UI that replaces the chat composer.
type permAskState struct {
	req    permission.Request
	reason string
	reply  chan AskReply

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
	case permission.ActionFetch:
		return "Allow fetching URL?", req.URL
	default:
		return fmt.Sprintf("Invoke tool %s?", req.Tool), permission.Summarize(req)
	}
}

func newPermAskState(req permission.Request, reason string, reply chan AskReply) *permAskState {
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

func (editor *Editor) beginPermissionAsk(msg PermissionAskMsg) {
	if editor.permAsk != nil {
		editor.resolvePermission(AskReply{})
	}
	editor.hideCompleters()
	if editor.palette.Open {
		editor.palette.Hide()
	}
	editor.permAsk = newPermAskState(msg.Request, msg.Reason, msg.Reply)
	editor.activity.Apply(ActivityAwaitingApproval)
	// Steal focus from Chat so ↑↓ reach handlePermissionKey (Chat would Consume them).
	if editor.App != nil {
		editor.App.RequestFocus(editor)
	}
}

func (editor *Editor) resolvePermission(r AskReply) {
	st := editor.permAsk
	if st == nil {
		return
	}
	editor.permAsk = nil
	if editor.activity.Current == ActivityAwaitingApproval {
		editor.activity.Apply(ActivityTools)
	}
	if editor.App != nil {
		editor.App.RequestFocus(&editor.Chat)
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
		editor.resolvePermission(AskReply{})
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
		editor.resolvePermission(AskReply{Approved: true})
	case askOptAllowSession:
		editor.resolvePermission(AskReply{Approved: true, AllowSession: true})
	case askOptAllowPersistent:
		editor.resolvePermission(AskReply{Approved: true, AllowPersistent: true})
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
		editor.resolvePermission(AskReply{Feedback: fb})
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
	if innerW < 20 {
		innerW = 20
	}
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

	warn := th.Warning
	primary := th.Success // selection accent; ToolName overrides when set
	if th.ToolName.Fg.Kind != 0 {
		primary = th.ToolName
	}

	var body []components.RichLine
	body = append(body, components.WrapSpans([]components.Span{
		{Text: st.header, Style: th.Foreground},
	}, innerW, ctx.Method)...)

	if st.detail != "" {
		detailLines := strings.Split(st.detail, "\n")
		for i, line := range detailLines {
			var spans []components.Span
			if i == 0 && st.req.Action == permission.ActionBash {
				spans = []components.Span{
					{Text: "$ ", Style: xui.Style{Bold: true, Fg: th.Success.Fg}},
					{Text: line, Style: th.Foreground},
				}
			} else if st.req.Action == permission.ActionBash {
				spans = []components.Span{{Text: "  " + line, Style: th.Foreground}}
			} else {
				spans = []components.Span{{Text: line, Style: xui.Style{Bold: true, Fg: th.Foreground.Fg}}}
			}
			body = append(body, components.WrapSpans(spans, innerW, ctx.Method)...)
		}
	}
	if st.reason != "" {
		body = append(body, components.WrapSpans([]components.Span{
			{Text: "(" + st.reason + ")", Style: th.Muted},
		}, innerW, ctx.Method)...)
	}

	body = append(body, components.RichLine{})

	if st.feedbackMode {
		body = append(body, components.WrapSpans([]components.Span{
			{Text: "✗ ", Style: th.Destructive},
			{Text: "Denied", Style: xui.Style{Bold: true, Fg: th.Destructive.Fg}},
			{Text: " — tell Phi what to do instead", Style: th.Muted},
		}, innerW, ctx.Method)...)
		runes := []rune(st.feedback)
		if st.feedbackCur > len(runes) {
			st.feedbackCur = len(runes)
		}
		shown := string(runes[:st.feedbackCur]) + "▎" + string(runes[st.feedbackCur:])
		body = append(body, components.WrapSpans([]components.Span{
			{Text: "› ", Style: xui.Style{Bold: true, Fg: primary.Fg}},
			{Text: shown, Style: th.Foreground},
		}, innerW, ctx.Method)...)
		body = append(body, components.WrapSpans([]components.Span{
			{Text: "Enter send  •  Esc cancel", Style: th.Muted},
		}, innerW, ctx.Method)...)
	} else {
		for i, label := range askOptionLabels {
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
			{Text: "↑↓ navigate • Enter select • Esc cancel", Style: th.Muted},
		}, innerW, ctx.Method)...)
	}

	panel := components.NewSurface(width, height, nil)
	border := warn
	layout.DrawRoundedBorder(&panel, layout.BorderRounded, border, nil, nil, nil, nil, ctx.Method)
	y := 1
	for _, line := range body {
		if y >= height-1 {
			break
		}
		components.PaintSpans(&panel, 2, y, line, ctx.Method)
		y++
	}
	return panel
}
