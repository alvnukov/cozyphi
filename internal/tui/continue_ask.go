package tui

import (
	"fmt"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/layout"
)

// ContinueReply is the user's response when the tool-round budget is exhausted.
type ContinueReply struct {
	Continue bool
}

var continueOptionLabels = []string{
	"Continue",
	"Stop",
}

// continueAskState holds the max-rounds continuation UI in the composer slot.
type continueAskState struct {
	maxRounds int
	reply     chan ContinueReply
	selected  int
}

func newContinueAskState(maxRounds int, reply chan ContinueReply) *continueAskState {
	return &continueAskState{
		maxRounds: maxRounds,
		reply:     reply,
		selected:  0,
	}
}

func (editor *Editor) beginContinueAsk(msg ContinueAskMsg) {
	if editor.continueAsk != nil {
		editor.resolveContinue(ContinueReply{})
	}
	if editor.permAsk != nil {
		editor.resolvePermission(AskReply{})
	}
	editor.hideCompleters()
	if editor.palette.Open {
		editor.palette.Hide()
	}
	editor.continueAsk = newContinueAskState(msg.MaxRounds, msg.Reply)
	editor.activity.Apply(ActivityAwaitingApproval)
	if editor.App != nil {
		editor.App.RequestFocus(editor)
	}
}

func (editor *Editor) resolveContinue(r ContinueReply) {
	st := editor.continueAsk
	if st == nil {
		return
	}
	editor.continueAsk = nil
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

func (editor *Editor) handleContinueKey(ctx *components.EventContext, e xui.KeyEvent) bool {
	st := editor.continueAsk
	if st == nil || !e.Press {
		return false
	}

	if e.Mods.Has(xui.ModAlt) && e.Code == xui.KeyRune && e.Rune >= '1' && e.Rune <= '2' {
		idx := int(e.Rune - '1')
		editor.acceptContinueOption(idx)
		ctx.ConsumeAndRedraw()
		return true
	}

	switch e.Code {
	case xui.KeyEscape:
		editor.resolveContinue(ContinueReply{})
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
		editor.acceptContinueOption(st.selected)
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

func (editor *Editor) acceptContinueOption(idx int) {
	switch idx {
	case 0:
		editor.resolveContinue(ContinueReply{Continue: true})
	default:
		editor.resolveContinue(ContinueReply{})
	}
}

func (st *continueAskState) preferredAskHeight() int {
	h := 2 + 1 + 1 + len(continueOptionLabels) + 1 + 1
	if h < 8 {
		h = 8
	}
	return h
}

func (editor *Editor) drawContinueAsk(ctx components.DrawContext, width, height int) components.Surface {
	st := editor.continueAsk
	if st == nil {
		return components.NewSurface(width, height, nil)
	}
	th := editor.theme
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

	panel := components.NewSurface(width, height, nil)
	layout.DrawRoundedBorder(&panel, layout.BorderRounded, warn, nil, nil, nil, nil, ctx.Method)
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
