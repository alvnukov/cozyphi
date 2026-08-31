package overlays

import (
	"fmt"
	"slices"
	"strings"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/input"
	"github.com/alvnukov/cozyphi/internal/tools/questiontool"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/keys"
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
	customs  []input.Line
	editing  bool // editing the custom-answer text
}

func newQuestionAskState(qs []questiontool.Question, reply chan controller.QuestionReply) *questionAskState {
	return &questionAskState{
		questions: qs,
		reply:     reply,
		answers:   make([][]string, len(qs)),
		customs:   make([]input.Line, len(qs)),
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
	o.beginAsk()
	o.question = newQuestionAskState(msg.Questions, msg.Reply)
}

func (o *Overlays) dismissQuestion() {
	st := o.question
	o.question = nil
	o.endAsk(st != nil)
}

func (o *Overlays) resolveQuestion(r controller.QuestionReply) {
	st := o.question
	o.question = nil
	o.endAsk(st != nil)
	if st != nil {
		sendReply(st.reply, r)
	}
}

func (o *Overlays) submitQuestion(st *questionAskState) {
	answers := make([]questiontool.Answer, len(st.questions))
	for i := range st.questions {
		answers[i] = append([]string(nil), st.answers[i]...)
	}
	o.resolveQuestion(controller.QuestionReply{Answers: answers})
}

// handleQuestionEditKey handles text entry for the custom-answer row: the
// overlay owns Esc and Enter, the shared line editor owns the editing keys, and
// what neither claims (Tab, Up, Down) falls through to option navigation.
func (o *Overlays) handleQuestionEditKey(ctx *components.EventContext, e xui.KeyEvent) bool {
	st := o.question
	if st == nil {
		return false
	}
	line := &st.customs[st.tab]
	switch e.Code {
	case xui.KeyEscape:
		st.editing = false
	case xui.KeyEnter:
		if answer := line.Trimmed(); answer != "" {
			if st.multi() {
				line.Set(answer)
				st.toggle(answer)
			} else {
				st.answers[st.tab] = []string{answer}
			}
		}
		st.editing = false
	default:
		if !line.Key(e) {
			return false
		}
	}
	ctx.ConsumeAndRedraw()
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
		if st.multi() && !st.customs[st.tab].Empty() {
			st.toggle(st.customs[st.tab].Trimmed())
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
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = st.preferredAskHeight(o.theme, width, ctx.Method)
	}
	body := st.askRows(o.theme, askInnerWidth(width), ctx.Method)
	return paintAskPanel(body, width, height, o.theme.Warning, ctx.Method)
}

func questionTabLine(
	st *questionAskState,
	th components.Theme,
	primary xui.Style,
	innerW int,
	method xui.WidthMethod,
) []components.RichLine {
	// All question headers plus the corner action render on one horizontal
	// row, wrapping only when the panel is too narrow for all of them.
	var spans []components.Span
	for i, q := range st.questions {
		active := i == st.tab
		answered := len(st.answers[i]) > 0
		fg := th.Muted.Fg
		if active {
			fg = primary.Fg
		} else if answered {
			fg = th.Foreground.Fg
		}
		spans = append(spans, components.Span{
			Text:  q.Header,
			Style: xui.Style{Bold: active, Fg: fg},
		})
		if i < len(st.questions)-1 {
			spans = append(spans, components.Span{Text: "   ", Style: th.Muted})
		}
	}

	spans = append(spans, components.Span{Text: "      ", Style: th.Muted})
	confirmFg := th.Muted.Fg
	if st.submitTab() {
		confirmFg = primary.Fg
	}
	spans = append(spans, components.Span{
		Text:  "Confirm",
		Style: xui.Style{Bold: st.submitTab(), Fg: confirmFg},
	})
	return components.WrapSpans(spans, innerW, method)
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
		if picked {
			if st.multi() {
				marker = " ✓ "
			} else {
				marker = " ● "
			}
			labelSt = th.Success
		}
		if active {
			if !picked {
				marker = " ▸ "
			}
			labelSt = xui.Style{Bold: true, Fg: primary.Fg}
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
		custom := &st.customs[st.tab]
		customPicked := !custom.Empty()
		marker := " ○ "
		labelSt := th.Foreground
		if customPicked {
			if st.multi() {
				marker = " ✓ "
			} else {
				marker = " ● "
			}
			labelSt = th.Success
		}
		if active {
			if !customPicked {
				marker = " ▸ "
			}
			labelSt = xui.Style{Bold: true, Fg: primary.Fg}
		}
		out = append(out, components.WrapSpans([]components.Span{
			{Text: fmt.Sprintf("%s%d. Type your own answer", marker, idx+1), Style: labelSt},
		}, innerW, method)...)
		if st.editing {
			// The field scrolls on one row instead of wrapping, so a long answer
			// cannot push the rows below it out of an already measured panel.
			out = append(out, components.RichLine{
				{Text: "    › " + custom.Display(innerW-6, method), Style: th.Foreground},
			})
		} else if customPicked {
			out = append(out, components.WrapSpans([]components.Span{
				{Text: "    " + custom.Trimmed(), Style: th.Muted},
			}, innerW, method)...)
		}
	}
	return out
}

// preferredAskHeight counts the rows the current tab actually renders — the
// tab strip, question, every option and its optional description row — plus
// the border, so descriptions and the custom-answer row are never truncated
// out of reach.
func (st *questionAskState) preferredAskHeight(th components.Theme, width int, method xui.WidthMethod) int {
	return max(len(st.askRows(th, askInnerWidth(width), method))+2, 8)
}

func (st *questionAskState) askRows(th components.Theme, innerW int, method xui.WidthMethod) []components.RichLine {
	primary := askPrimary(th)
	var body []components.RichLine
	body = append(body, questionTabLine(st, th, primary, innerW, method)...)
	body = append(body, components.RichLine{})

	if st.submitTab() {
		body = append(body, components.WrapSpans([]components.Span{
			{Text: "Review answers, then submit:", Style: th.Foreground},
		}, innerW, method)...)
		for i, q := range st.questions {
			value := strings.Join(st.answers[i], ", ")
			if value == "" {
				value = "(not answered)"
			}
			body = append(body, components.WrapSpans([]components.Span{
				{Text: q.Header + ": ", Style: th.Muted},
				{Text: value, Style: th.Foreground},
			}, innerW, method)...)
		}
	} else {
		body = append(body, components.WrapSpans([]components.Span{
			{Text: st.question().Question, Style: th.Foreground},
		}, innerW, method)...)
		body = append(body, questionOptionLines(st, th, primary, innerW, method)...)
	}

	body = append(body, components.WrapSpans([]components.Span{
		{Text: keys.Hints(keys.ScopeQuestion), Style: th.Muted},
	}, innerW, method)...)
	return body
}

func contains(xs []string, s string) bool {
	return slices.Contains(xs, s)
}
