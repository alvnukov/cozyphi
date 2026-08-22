package tui

import (
	"context"
	"strings"
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/chat"
	"github.com/pulseaiclub/phi/internal/components/mention"
	"github.com/pulseaiclub/phi/internal/tui/controller"
	"github.com/pulseaiclub/phi/internal/util/filesearch"
)

// InputRouter owns keyboard/mouse routing and mention/slash completers.
type InputRouter struct {
	e *Editor
}

// Handle dispatches an xui event to the focused sub-components.
func (r *InputRouter) Handle(ctx *components.EventContext, ev xui.Event) {
	e := r.e
	switch ev := ev.(type) {
	case xui.FocusEvent:
		if e.permAsk != nil || e.continueAsk != nil {
			ctx.RequestFocus(e)
		} else if e.palette.Open {
			ctx.RequestFocus(&e.palette)
		} else {
			ctx.RequestFocus(&e.Chat)
		}
	case xui.KeyEvent:
		if ev.CtrlC() {
			if e.ctrl != nil {
				e.ctrl.Close()
			}
			ctx.Quit = true
			return
		}
		if e.handlePermissionKey(ctx, ev) {
			return
		}
		if e.handleContinueKey(ctx, ev) {
			return
		}
		if e.handleCopyKey(ctx, ev) {
			return
		}
		if ev.Press && ev.Code == xui.KeyEscape {
			if e.slash.Open {
				e.slash.Cancel()
				e.Chat.SlashOpen = false
				ctx.ConsumeAndRedraw()
				return
			}
			if e.mention.Open {
				e.mention.Cancel()
				e.Chat.MentionOpen = false
				e.mentionGen++
				ctx.ConsumeAndRedraw()
				return
			}
			if e.bash.Running() || e.isBusy() {
				e.Publish(controller.CancelStreamMsg{})
				e.drainBus()
				ctx.ConsumeAndRedraw()
				return
			}
			if e.transcript.SelectionActive() {
				e.transcript.ClearSelection()
				ctx.ConsumeAndRedraw()
				return
			}
		}
		if ev.Press && ev.Mods.Has(xui.ModCtrl) && ev.Code == xui.KeyRune &&
			(ev.Rune == 'k' || ev.Rune == 'K') {
			if e.palette.Open {
				e.palette.Hide()
				ctx.RequestFocus(&e.Chat)
			} else {
				r.HideCompleters()
				e.palette.Show()
				ctx.RequestFocus(&e.palette)
			}
			ctx.ConsumeAndRedraw()
			return
		}
		if e.palette.Open {
			e.palette.Handle(ctx, ev)
			if !e.palette.Open {
				ctx.RequestFocus(&e.Chat)
			}
			return
		}
		if e.slash.Open && mentionNavKey(ev) {
			e.slash.Handle(ctx, ev)
			if !e.slash.Open {
				e.Chat.SlashOpen = false
			}
			return
		}
		if e.mention.Open && mentionNavKey(ev) {
			e.mention.Handle(ctx, ev)
			if !e.mention.Open {
				e.Chat.MentionOpen = false
			}
			return
		}
		if ev.Code == xui.KeyPageUp || ev.Code == xui.KeyPageDown {
			e.transcript.HandlePageKey(ctx, ev)
			return
		}
		e.Chat.Handle(ctx, ev)
	case xui.MouseEvent:
		if e.palette.Open {
			e.palette.Handle(ctx, ev)
			return
		}
		e.transcript.HandleMouse(ctx, ev, func() {
			ctx.RequestFocus(&e.Chat)
		})
	case xui.PasteEvent:
		if e.palette.Open {
			e.palette.Handle(ctx, ev)
			return
		}
		e.Chat.Handle(ctx, ev)
	}
}

func mentionNavKey(e xui.KeyEvent) bool {
	if !e.Press {
		return false
	}
	switch e.Code {
	case xui.KeyUp, xui.KeyDown, xui.KeyTab, xui.KeyEnter, xui.KeyEscape:
		return true
	case xui.KeyRune:
		if e.Mods.Has(xui.ModCtrl) && (e.Rune == 'n' || e.Rune == 'N' || e.Rune == 'p' || e.Rune == 'P') {
			return true
		}
	}
	return false
}

// HideCompleters closes mention and slash pickers.
func (r *InputRouter) HideCompleters() {
	if r == nil || r.e == nil {
		return
	}
	e := r.e
	e.mention.Hide()
	e.Chat.MentionOpen = false
	e.mentionGen++
	e.slash.Hide()
	e.Chat.SlashOpen = false
}

// OnMentionChange reacts to composer @-mention state.
func (r *InputRouter) OnMentionChange(active bool, query string) {
	e := r.e
	if !active {
		e.mention.Hide()
		e.Chat.MentionOpen = false
		e.mentionGen++
		return
	}
	if e.slash.Open || e.Chat.SlashOpen {
		return
	}
	e.slash.Hide()
	e.Chat.SlashOpen = false
	e.mention.Show()
	e.Chat.MentionOpen = true
	if len(e.mention.Items) == 0 {
		e.mention.Status = "Searching…"
	}
	r.scheduleMentionSearch(query)
}

// OnSlashChange reacts to composer / command state.
func (r *InputRouter) OnSlashChange(active bool, query string) {
	e := r.e
	if !active {
		e.slash.Hide()
		e.Chat.SlashOpen = false
		return
	}
	e.mention.Hide()
	e.Chat.MentionOpen = false
	e.mentionGen++
	items := []mention.Item{}
	if e.commands != nil {
		items = e.commands.FilterSlash(query)
	}
	status := ""
	if len(items) == 0 {
		status = "No matching commands"
	}
	e.slash.SetResults(items, status)
	e.slash.Show()
	e.Chat.SlashOpen = true
}

func (r *InputRouter) scheduleMentionSearch(query string) {
	e := r.e
	e.mentionGen++
	gen := e.mentionGen
	cwd := e.cwd
	go func() {
		time.Sleep(100 * time.Millisecond)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		paths, err := filesearch.Search(ctx, cwd, query, 20)
		msg := controller.MentionResultsMsg{Gen: gen, Query: query, Paths: paths}
		if err != nil {
			msg.ErrText = err.Error()
		}
		e.Publish(msg)
	}()
}

// ApplyMentionResults updates the mention picker from a search result msg.
func (r *InputRouter) ApplyMentionResults(msg controller.MentionResultsMsg) {
	e := r.e
	if msg.Gen != e.mentionGen || !e.mention.Open {
		return
	}
	if msg.ErrText != "" {
		e.mention.SetResults(nil, msg.ErrText)
		return
	}
	items := make([]mention.Item, 0, len(msg.Paths))
	for _, p := range msg.Paths {
		items = append(items, mention.Item{Path: p})
	}
	status := ""
	if len(items) == 0 {
		status = "No matching files"
	}
	e.mention.SetResults(items, status)
}

// AcceptMention inserts an @path into the composer.
func (r *InputRouter) AcceptMention(item mention.Item) {
	e := r.e
	_, start, end, ok := chat.ActiveMention(e.Chat.Value, e.Chat.Cursor)
	if !ok {
		start, end = e.Chat.Cursor, e.Chat.Cursor
	}
	e.mentionGen++
	e.mention.Hide()
	e.Chat.MentionOpen = false
	e.Chat.ReplaceRange(start, end, "@"+item.Path+" ")
}

// AcceptSlash inserts a slash command (and may auto-submit no-arg commands).
func (r *InputRouter) AcceptSlash(item mention.Item) {
	e := r.e
	_, start, end, ok := chat.ActiveSlash(e.Chat.Value, e.Chat.Cursor)
	if !ok {
		start, end = 0, e.Chat.Cursor
	}
	insert := ""
	if e.commands != nil {
		insert = e.commands.LookupInsert(item.Path)
	}
	if insert == "" {
		insert = "/" + item.Path
	}
	e.Chat.ReplaceRange(start, end, insert)
	e.slash.Hide()
	e.Chat.SlashOpen = false
	if !strings.HasSuffix(insert, " ") {
		e.Publish(controller.SubmitMsg{Text: strings.TrimSpace(insert)})
		e.drainBus()
	}
}
