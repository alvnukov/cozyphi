package overlays

import (
	"fmt"
	"slices"
	"strings"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/tools/questiontool"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

// questionAskState owns the interactive question overlay (the model asks the
// user to pick from options). It mirrors the continue-ask state, adding
// per-tab option navigation, multi-select and a "type your own answer" row.
type questionAskState struct {
	questions []questiontool.Question
	reply     chan controller.QuestionReply

	tab      int // index into questions, or len(questions) for the submit tab
	selected int
	answers  [][]string
	customs  []string
	editing  bool // editing the custom-answer text
}

func newQuestionAskState(qs []questiontool.Question, reply chan controller.QuestionReply) *questionAskState {
	return &questionAskState{
		questions: qs,
		reply:     reply,
		answers:   make([][]string, len(qs)),
		customs:   make([]string, len(qs)),
	}
}

func (st *questionAskState) tabs() int       { return len(st.questions) + 1 }
func (st *questionAskState) submitTab() bool { return st.tab >= len(st.questions) }
func (st *questionAskState) multi() bool     { return st.question().Multiple }
func (st *questionAskState) question() questiontool.Question {
	if st.submitTab() {
		return questiontool.Question{}
	}
	return st.questions[st.tab]
}

func (st *questionAskState) optionCount() int {
	n := len(st.question().Options)
	if st.question().Custom {
		n++
	}
	return n
}

func (st *questionAskState) toggle(label string) {
	cur := st.answers[st.tab]
	next := make([]string, 0, len(cur)+1)
	for _, l := range cur {
		if l != label {
			next = append(next, l)
		}
	}
	if len(next) == len(cur) {
		next = append(next, label)
	}
	st.answers[st.tab] = next
}

func (st *questionAskState) gotoTab(idx int) {
	tabs := st.tabs()
	st.tab = ((idx % tabs) + tabs) % tabs
	st.selected = 0
}

// beginQuestionAsk routes a QuestionAskMsg into the overlay state.
func (o *Overlays) beginQuestionAsk(msg controller.QuestionAskMsg) {
	if o.question != nil {
		o.resolveQuestion(controller.QuestionReply{})
	}
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
	o.question = newQuestionAskState(msg.Questions, msg.Reply)
	o.activity.Apply(controller.ActivityAwaitingApproval)
	if o.focusEditor != nil {
		o.focusEditor()
	}
}

func (o *Overlays) dismissQuestion() {
	wasAsk := o.question != nil
	o.question = nil
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

func (o *Overlays) resolveQuestion(r controller.QuestionReply) {
	st := o.question
	if st == nil {
		return
	}
	o.question = nil
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

func (o *Overlays) submitQuestion(st *questionAskState) {
	answers := make([]questiontool.Answer, len(st.questions))
	for i := range st.questions {
		answers[i] = append([]string(nil), st.answers[i]...)
	}
	o.resolveQuestion(controller.QuestionReply{Answers: answers})
}

// handleQuestionEditKey handles text entry for the custom-answer row.
func (o *Overlays) handleQuestionEditKey(_ *components.EventContext, e xui.KeyEvent) bool {
	st := o.question
	if st == nil {
		return false
	}
	switch e.Code {
	case xui.KeyEscape:
		st.editing = false
	case xui.KeyEnter:
		text := strings.TrimSpace(st.customs[st.tab])
		if text != "" {
			if st.multi() {
				st.customs[st.tab] = text
				st.toggle(text)
			} else {
				st.answers[st.tab] = []string{text}
			}
		}
		st.editing = false
	case xui.KeyBackspace:
		r := []rune(st.customs[st.tab])
		if len(r) > 0 {
			st.customs[st.tab] = string(r[:len(r)-1])
		}
	case xui.KeyRune:
		if e.Mods.Has(xui.ModCtrl) || e.Mods.Has(xui.ModAlt) {
			return true
		}
		st.customs[st.tab] += string(e.Rune)
	default:
		return false
	}
	return true
}

func (o *Overlays) handleQuestionKey(ctx *components.EventContext, e xui.KeyEvent) bool {
	st := o.question
	if st == nil || !e.Press {
		return false
	}
	if st.editing {
		return o.handleQuestionEditKey(ctx, e)
	}

	switch e.Code {
	case xui.KeyLeft:
		st.gotoTab(st.tab - 1)
		ctx.ConsumeAndRedraw()
		return true
	case xui.KeyRight, xui.KeyTab:
		st.gotoTab(st.tab + 1)
		ctx.ConsumeAndRedraw()
		return true
	case xui.KeyUp:
		if st.selected > 0 {
			st.selected--
		} else if st.optionCount() > 0 {
			st.selected = st.optionCount() - 1
		}
		ctx.ConsumeAndRedraw()
		return true
	case xui.KeyDown:
		if st.optionCount() > 0 {
			st.selected = (st.selected + 1) % st.optionCount()
		}
		ctx.ConsumeAndRedraw()
		return true
	case xui.KeyEnter:
		o.acceptQuestionOption(st)
		ctx.ConsumeAndRedraw()
		return true
	case xui.KeyEscape:
		o.resolveQuestion(controller.QuestionReply{})
		ctx.ConsumeAndRedraw()
		return true
	case xui.KeyRune:
		if e.Mods.Has(xui.ModCtrl) || e.Mods.Has(xui.ModAlt) {
			ctx.ConsumeAndRedraw()
			return true
		}
		switch e.HotkeyRune() {
		case 'h':
			st.gotoTab(st.tab - 1)
		case 'l':
			st.gotoTab(st.tab + 1)
		case 'k':
			if st.selected > 0 {
				st.selected--
			} else if st.optionCount() > 0 {
				st.selected = st.optionCount() - 1
			}
		case 'j':
			if st.optionCount() > 0 {
				st.selected = (st.selected + 1) % st.optionCount()
			}
		default:
			if e.Rune < '1' || e.Rune > '9' {
				return false
			}
			idx := int(e.Rune - '1')
			if idx < st.optionCount() {
				st.selected = idx
				o.acceptQuestionOption(st)
			}
		}
		ctx.ConsumeAndRedraw()
		return true
	}
	ctx.ConsumeAndRedraw()
	return true
}

func (o *Overlays) acceptQuestionOption(st *questionAskState) {
	if st.submitTab() {
		o.submitQuestion(st)
		return
	}
	opts := st.question().Options
	if st.selected < len(opts) {
		label := opts[st.selected].Label
		if st.multi() {
			st.toggle(label)
			return
		}
		st.answers[st.tab] = []string{label}
		if len(st.questions) == 1 {
			o.submitQuestion(st)
			return
		}
		st.gotoTab(st.tab + 1)
		return
	}
	if st.question().Custom && st.selected == len(opts) {
		if st.multi() && st.customs[st.tab] != "" {
			st.toggle(st.customs[st.tab])
			return
		}
		st.editing = true
	}
}

func (o *Overlays) drawQuestionAsk(ctx components.DrawContext, width, height int) components.Surface {
	st := o.question
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

	primary := th.Success
	if th.ToolName.Fg.Kind != 0 {
		primary = th.ToolName
	}

	var body []components.RichLine
	body = append(body, questionTabLine(st, th, primary, innerW, ctx.Method)...)
	body = append(body, components.RichLine{})

	if st.submitTab() {
		body = append(body, components.WrapSpans([]components.Span{
			{Text: "Review answers, then submit:", Style: th.Foreground},
		}, innerW, ctx.Method)...)
		for i, q := range st.questions {
			value := strings.Join(st.answers[i], ", ")
			if value == "" {
				value = "(not answered)"
			}
			body = append(body, components.WrapSpans([]components.Span{
				{Text: q.Header + ": ", Style: th.Muted},
				{Text: value, Style: th.Foreground},
			}, innerW, ctx.Method)...)
		}
	} else {
		q := st.question()
		label := q.Question
		if q.Multiple {
			label += " (select all that apply)"
		}
		body = append(body, components.WrapSpans([]components.Span{
			{Text: label, Style: th.Foreground},
		}, innerW, ctx.Method)...)
		body = append(body, questionOptionLines(st, th, primary, innerW, ctx.Method)...)
	}

	body = append(body, components.WrapSpans([]components.Span{
		{Text: "⇆ tab • ↑↓ select • enter submit • esc dismiss", Style: th.Muted},
	}, innerW, ctx.Method)...)

	return paintAskPanel(body, width, height, th.Warning, ctx.Method)
}

func questionTabLine(
	st *questionAskState,
	th components.Theme,
	primary xui.Style,
	innerW int,
	method xui.WidthMethod,
) []components.RichLine {
	var out []components.RichLine
	for i, q := range st.questions {
		active := i == st.tab
		answered := len(st.answers[i]) > 0
		fg := th.Muted.Fg
		if active {
			fg = primary.Fg
		} else if answered {
			fg = th.Foreground.Fg
		}
		prefix := " "
		if active {
			prefix = "▸ "
		}
		out = append(out, components.WrapSpans([]components.Span{
			{Text: fmt.Sprintf("%s%s", prefix, q.Header), Style: xui.Style{Bold: active, Fg: fg}},
		}, innerW, method)...)
	}
	submit := "Submit"
	if st.submitTab() {
		submit = "▸ Submit"
	}
	out = append(out, components.WrapSpans([]components.Span{
		{Text: submit, Style: xui.Style{Bold: st.submitTab(), Fg: th.Muted.Fg}},
	}, innerW, method)...)
	return out
}

func questionOptionLines(
	st *questionAskState,
	th components.Theme,
	primary xui.Style,
	innerW int,
	method xui.WidthMethod,
) []components.RichLine {
	var out []components.RichLine
	opts := st.question().Options
	for i, opt := range opts {
		active := i == st.selected
		picked := contains(st.answers[st.tab], opt.Label)
		marker := " ○ "
		labelSt := th.Foreground
		if st.multi() {
			if picked {
				marker = " ✓ "
				labelSt = th.Success
			}
			if active {
				marker = " ▸ "
				labelSt = xui.Style{Bold: true, Fg: primary.Fg}
			}
		} else {
			if picked {
				marker = " ● "
				labelSt = th.Success
			}
			if active {
				marker = " ▸ "
				labelSt = xui.Style{Bold: true, Fg: primary.Fg}
			}
		}
		out = append(out, components.WrapSpans([]components.Span{
			{Text: fmt.Sprintf("%s%d. %s", marker, i+1, opt.Label), Style: labelSt},
		}, innerW, method)...)
		if opt.Description != "" {
			out = append(out, components.WrapSpans([]components.Span{
				{Text: "    " + opt.Description, Style: th.Muted},
			}, innerW, method)...)
		}
	}
	if st.question().Custom {
		idx := len(opts)
		active := st.selected == idx
		customPicked := st.customs[st.tab] != ""
		marker := " ○ "
		labelSt := th.Foreground
		if st.multi() {
			if customPicked {
				marker = " ✓ "
			}
			if active {
				marker = " ▸ "
				labelSt = xui.Style{Bold: true, Fg: primary.Fg}
			}
		} else {
			if customPicked {
				marker = " ● "
				labelSt = th.Success
			}
			if active {
				marker = " ▸ "
				labelSt = xui.Style{Bold: true, Fg: primary.Fg}
			}
		}
		out = append(out, components.WrapSpans([]components.Span{
			{Text: fmt.Sprintf("%s%d. Type your own answer", marker, idx+1), Style: labelSt},
		}, innerW, method)...)
		if st.editing {
			out = append(out, components.WrapSpans([]components.Span{
				{Text: "    › " + st.customs[st.tab] + "▎", Style: th.Foreground},
			}, innerW, method)...)
		} else if customPicked {
			out = append(out, components.WrapSpans([]components.Span{
				{Text: "    " + st.customs[st.tab], Style: th.Muted},
			}, innerW, method)...)
		}
	}
	return out
}

func (st *questionAskState) preferredAskHeight() int {
	h := 2 + 1 + 1 // tabs + blank + question
	h += st.optionCount()*2 + 1
	if h < 8 {
		h = 8
	}
	return h
}

func contains(xs []string, s string) bool {
	return slices.Contains(xs, s)
}
