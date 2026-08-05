package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/app"
	"github.com/pulseaiclub/phi/internal/components/block"
	"github.com/pulseaiclub/phi/internal/components/chat"
	"github.com/pulseaiclub/phi/internal/components/layout"
	"github.com/pulseaiclub/phi/internal/components/mention"
	"github.com/pulseaiclub/phi/internal/components/palette"
	"github.com/pulseaiclub/phi/internal/components/splash"
	"github.com/pulseaiclub/phi/internal/components/status"
	"github.com/pulseaiclub/phi/internal/components/toast"
	"github.com/pulseaiclub/phi/internal/components/transcript"
	"github.com/pulseaiclub/phi/internal/session"
	"github.com/pulseaiclub/phi/internal/util/filesearch"
	"github.com/pulseaiclub/xui"
)

// Editor is the TUI root widget: layout composition and the UI-goroutine
// message loop. Cross-component work goes through Bus — producers Publish,
// Draw drains and Update applies. Agent lifecycle lives in Controller;
// session→widget projection lives in Mapper; activity status in ActivityHandler.
type Editor struct {
	vx    *xui.XUI
	App   *app.App
	theme components.Theme
	bus   *Bus
	cwd   string

	list      transcript.MessageList
	Chat      chat.ChatInput
	palette   palette.CommandPalette
	mention   mention.Picker
	slash     mention.Picker
	toast     toast.Toast
	spin      *status.Spinner
	welcome   splash.Screen
	activity  *ActivityHandler
	startedAt time.Time
	tick      int

	listH        int
	lastListSurf components.Surface
	sel          textSel

	// Session model. Mutations happen only on the UI goroutine via Update.
	snap session.Snapshot

	mapper  *Mapper
	ctrl    *Controller
	listIDs []string // parallels list.Entries (item ids)

	mentionGen int // bumped to invalidate in-flight @-file searches

	contextWindow int
	lastUsage     session.TokenUsage
	usageStats    string // panda-style ↑↓Σ for footer

	permAsk *permAskState
}

func newChatInput(theme components.Theme, model string, cwd string) chat.ChatInput {
	return chat.ChatInput{
		MinBodyRows:    3,
		MaxBodyRows:    8,
		UseBlockCursor: false, // terminal cursor only; reverse cells ghost on CJK delete
		PaddingX:       1,
		Theme:          theme,
		BorderStyle:    theme.Border,
		TextStyle:      theme.Foreground,
		CursorStyle:    xui.Style{Reverse: true},
		TopLeftLabel: layout.BorderLabel{
			Text:  pathWithBranch(cwd),
			Style: pathLabelStyle(theme),
		},
		TopRightLabel: layout.BorderLabel{
			Text:  model,
			Style: theme.Success,
		},
		// BottomRightLabel filled by updateTokenDisplay after the first usage report.
	}
}

func NewEditor(vx *xui.XUI, theme components.Theme, cwd string, model string, skillPath string, contextWindow int) *Editor {
	editor := &Editor{
		vx:            vx,
		theme:         theme,
		cwd:           cwd,
		contextWindow: contextWindow,
		Chat:          newChatInput(theme, model, cwd),
		spin:          status.NewWaveSpinner(theme.ToolName),
		startedAt:     time.Now(),
		welcome: splash.Screen{
			Sphere: &splash.Sphere{Fast: true},
			Theme:  theme,
			Brand:  "Phi",
		},
		palette: palette.CommandPalette{
			Theme: theme,
		},
		mention: mention.Picker{
			Theme: theme,
		},
		slash: mention.Picker{
			Theme:  theme,
			Prefix: "/",
		},
		toast: toast.Toast{Theme: theme},
		list: transcript.MessageList{
			Theme:    theme,
			Selected: -1,
		},
	}
	editor.activity = NewActivityHandler(editor.spin)
	editor.bus = NewBus(editor.requestRedraw)
	editor.mapper = NewMapper(theme, editor.spin, func() {
		editor.list.InvalidateHeights()
	})
	editor.ctrl = NewController(editor.bus)
	editor.palette.FocusReturn = &editor.Chat
	editor.Chat.OnSubmit = func(text string) {
		// OnSubmit runs on the UI goroutine — publish and apply immediately
		// so the composer clears before the next frame.
		editor.Publish(SubmitMsg{Text: text})
		editor.drainBus()
	}
	editor.Chat.OnChange = func(string) {
		// CJK paste/delete can desync tty wide-glyph columns vs our damage
		// grid; force a full redraw so ghost cells cannot stick around.
		if editor.vx != nil {
			editor.vx.QueueRefresh()
		}
	}
	editor.Chat.OnMentionChange = func(active bool, query string) {
		editor.handleMentionChange(active, query)
	}
	editor.Chat.OnSlashChange = func(active bool, query string) {
		editor.handleSlashChange(active, query)
	}
	editor.mention.OnAccept = func(item mention.Item) {
		editor.acceptMention(item)
	}
	editor.slash.OnAccept = func(item mention.Item) {
		editor.acceptSlash(item)
	}
	addSkill := func(name string) {
		editor.Chat.AddPendingSkill(name)
		if editor.vx != nil {
			editor.vx.QueueRefresh()
		}
	}
	editor.palette.Commands = append(
		PaletteCommands(func(name string) {
			if err := editor.ctrl.SetModel(name); err != nil {
				editor.toast.Show(err.Error(), toast.ToastError, 3*time.Second)
				return
			}
			editor.Chat.TopRightLabel.Text = name
			editor.toast.Show("Model: "+name, toast.ToastSuccess, 2*time.Second)
			if editor.vx != nil {
				editor.vx.QueueRefresh()
			}
		}),
		ThemeCommand(editor.applyTheme),
		SkillsCommand(skillPath, addSkill),
		palette.PaletteCommand{
			ID:       "clipboard-copy-last",
			Noun:     "clipboard",
			Verb:     "copy last message",
			Keywords: []string{"yank", "selection"},
			Shortcut: "Ctrl+Shift+C",
			Run: func() {
				text := editor.list.LastCopyText()
				editor.copyBlock(text)
			},
		},
	)
	return editor
}

// applyTheme switches the live chrome + transcript widgets to a builtin theme.
func (editor *Editor) applyTheme(name string) {
	th, ok := components.ThemeByName(name)
	if !ok {
		return
	}
	editor.theme = th
	editor.Chat.Theme = th
	editor.Chat.BorderStyle = th.Border
	editor.Chat.TextStyle = th.Foreground
	editor.Chat.TopLeftLabel.Style = pathLabelStyle(th)
	editor.Chat.TopRightLabel.Style = th.Success
	editor.palette.Theme = th
	editor.mention.Theme = th
	editor.slash.Theme = th
	editor.toast.Theme = th
	editor.welcome.Theme = th
	editor.list.Theme = th
	if editor.spin != nil {
		editor.spin.Style = th.ToolName
	}
	if editor.mapper != nil {
		editor.mapper.theme = th
	}
	applyThemeToWidgets(editor.list.Entries, th)
	editor.list.InvalidateHeights()
	if editor.lastUsage.Reported() {
		editor.updateTokenDisplay(editor.lastUsage)
	}
	editor.toast.Show("Theme: "+name, toast.ToastSuccess, 2*time.Second)
	if editor.vx != nil {
		editor.vx.QueueRefresh()
	}
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
		}
	}
}

// Publish sends a message onto the bus from any goroutine / widget callback.
func (editor *Editor) Publish(m Msg) {
	if editor.bus != nil {
		editor.bus.Publish(m)
	}
}

// Update applies one message on the UI goroutine. Returns whether a redraw is useful.
func (editor *Editor) Update(m Msg) {
	switch msg := m.(type) {
	case SubmitMsg:
		editor.handleSubmit(msg.Text)
	case CancelStreamMsg:
		editor.handleCancel()
	case SessionEventMsg:
		editor.applySessionEvent(msg.Event)
	case SetActivityMsg:
		editor.activity.Apply(msg.Activity)
	case ClearIfActivityMsg:
		if editor.activity.Current == msg.If {
			editor.activity.Apply(ActivityIdle)
		}
	case MentionResultsMsg:
		editor.applyMentionResults(msg)
	case PermissionAskMsg:
		editor.beginPermissionAsk(msg)
	case PermissionDismissMsg:
		// Agent already timed out / cancelled; only clear the overlay.
		wasAsk := editor.permAsk != nil
		editor.permAsk = nil
		if wasAsk {
			if editor.activity.Current == ActivityAwaitingApproval {
				editor.activity.Apply(ActivityTools)
			}
			if editor.App != nil {
				editor.App.RequestFocus(&editor.Chat)
			}
		}
	case RedrawMsg:
		// no state change; drain already requested redraw
	}
}

func (editor *Editor) drainBus() {
	batch := editor.bus.Drain()
	if len(batch) == 0 {
		return
	}
	atBottom := editor.list.ScrollFromBottom == 0
	threadDirty := false
	for _, m := range batch {
		if _, ok := m.(SessionEventMsg); ok {
			threadDirty = true
		}
		editor.Update(m)
	}
	if threadDirty {
		editor.list.Entries, editor.listIDs = editor.mapper.Sync(editor.list.Entries, editor.listIDs, editor.snap)
		editor.list.InvalidateHeights()
		editor.activity.SyncFromSnap(editor.snap)
		if atBottom {
			editor.list.StickToBottom()
		}
	}
}

func (editor *Editor) applySessionEvent(ev session.Event) {
	editor.snap = session.Apply(editor.snap, ev)
	if upd, ok := ev.(session.AssistantMessageUpdate); ok && upd.Message.Usage.Reported() {
		editor.updateTokenDisplay(upd.Message.Usage)
	}
}

func (editor *Editor) handleSubmit(text string) {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "/") {
		if editor.handleSlash(text) {
			editor.hideCompleters()
			editor.Chat.Value = ""
			editor.Chat.Cursor = 0
			return
		}
	}
	pending := append([]string(nil), editor.Chat.PendingSkills...)
	if (text == "" && len(pending) == 0) || editor.isBusy() {
		return
	}

	editor.mention.Hide()
	editor.Chat.MentionOpen = false
	editor.mentionGen++
	editor.slash.Hide()
	editor.Chat.SlashOpen = false

	editor.activity.Apply(ActivitySubmitting)
	display := text
	if display == "" && len(pending) > 0 {
		display = "Skills: " + strings.Join(pending, ", ")
	}
	editor.applySessionEvent(session.UserAppend{Text: display})
	editor.list.Entries, editor.listIDs = editor.mapper.Sync(editor.list.Entries, editor.listIDs, editor.snap)
	editor.list.InvalidateHeights()
	editor.list.StickToBottom()
	editor.activity.Apply(ActivityWaiting)

	editor.Chat.Value = ""
	editor.Chat.Cursor = 0
	editor.Chat.ClearPendingSkills()

	editor.ctrl.StartPrompt(text, pending)
}

// handleSlash runs /sessions and /resume. Returns true when the input was consumed.
func (editor *Editor) handleSlash(text string) bool {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return false
	}
	switch fields[0] {
	case "/sessions":
		editor.showSessions()
		return true
	case "/resume":
		if len(fields) < 2 {
			editor.toast.Show("Usage: /resume <session-id>", toast.ToastWarning, 3*time.Second)
			return true
		}
		editor.resumeSession(fields[1])
		return true
	default:
		return false
	}
}

func (editor *Editor) showSessions() {
	dir := ""
	if editor.ctrl != nil {
		dir = editor.ctrl.sessionDir
	}
	list, err := session.ListSessions(dir)
	if err != nil {
		editor.toast.Show(err.Error(), toast.ToastError, 3*time.Second)
		return
	}
	const maxN = 12
	var b strings.Builder
	if len(list) == 0 {
		b.WriteString("No sessions for this directory")
	} else {
		fmt.Fprintf(&b, "Sessions in this directory (%d):\n", len(list))
		n := len(list)
		if n > maxN {
			n = maxN
		}
		for i := 0; i < n; i++ {
			m := list[i]
			short := m.ID
			if len(short) > 8 {
				short = short[:8]
			}
			preview := m.Preview
			if preview == "" {
				preview = "(no preview)"
			}
			fmt.Fprintf(&b, "  %s  %s  %s\n", short, m.Mtime.Format("01-02 15:04"), preview)
		}
		b.WriteString("Resume with /resume <id>")
	}
	editor.applySessionEvent(session.AssistantMessageUpdate{Message: session.Message{
		ID:    fmt.Sprintf("sessions-%d", time.Now().UnixNano()),
		State: session.StateComplete,
		Text:  b.String(),
		Content: []session.ContentBlock{
			{Type: session.BlockText, Text: b.String()},
		},
	}})
	editor.list.Entries, editor.listIDs = editor.mapper.Sync(editor.list.Entries, editor.listIDs, editor.snap)
	editor.list.InvalidateHeights()
	editor.list.StickToBottom()
}

func (editor *Editor) resumeSession(id string) {
	warn, err := editor.ctrl.Resume(id)
	if err != nil {
		editor.toast.Show(err.Error(), toast.ToastError, 4*time.Second)
		return
	}
	editor.snap = editor.ctrl.ReplaySnapshot()
	editor.list.Entries, editor.listIDs = editor.mapper.Sync(nil, nil, editor.snap)
	editor.list.InvalidateHeights()
	editor.list.StickToBottom()
	msg := "Resumed " + shortSessionID(editor.ctrl.SessionID())
	if warn != "" {
		editor.toast.Show(msg+": "+warn, toast.ToastWarning, 5*time.Second)
	} else {
		editor.toast.Show(msg, toast.ToastSuccess, 3*time.Second)
	}
}

func shortSessionID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func (editor *Editor) handleCancel() {
	if editor.permAsk != nil {
		editor.resolvePermission(AskReply{})
	}
	editor.ctrl.Cancel()
	editor.applySessionEvent(session.CancelStreaming{})
	editor.list.Entries, editor.listIDs = editor.mapper.Sync(editor.list.Entries, editor.listIDs, editor.snap)
	editor.list.InvalidateHeights()
	editor.activity.Apply(ActivityCancelled)
	time.AfterFunc(1200*time.Millisecond, func() {
		editor.Publish(ClearIfActivityMsg{If: ActivityCancelled})
	})
}

func (editor *Editor) Handle(ctx *components.EventContext, ev xui.Event) {
	switch e := ev.(type) {
	case xui.FocusEvent:
		if editor.permAsk != nil {
			ctx.RequestFocus(editor)
		} else if editor.palette.Open {
			ctx.RequestFocus(&editor.palette)
		} else {
			ctx.RequestFocus(&editor.Chat)
		}
	case xui.KeyEvent:
		if e.CtrlC() {
			ctx.Quit = true
			return
		}
		if editor.handlePermissionKey(ctx, e) {
			return
		}
		if editor.handleCopyKey(ctx, e) {
			return
		}
		if e.Press && e.Code == xui.KeyEscape {
			if editor.slash.Open {
				editor.slash.Cancel()
				editor.Chat.SlashOpen = false
				ctx.ConsumeAndRedraw()
				return
			}
			if editor.mention.Open {
				editor.mention.Cancel()
				editor.Chat.MentionOpen = false
				editor.mentionGen++
				ctx.ConsumeAndRedraw()
				return
			}
			if editor.isBusy() {
				editor.Publish(CancelStreamMsg{})
				editor.drainBus()
				ctx.ConsumeAndRedraw()
				return
			}
			if editor.sel.active {
				editor.sel.clear()
				ctx.ConsumeAndRedraw()
				return
			}
		}
		if e.Press && e.Mods.Has(xui.ModCtrl) && e.Code == xui.KeyRune &&
			(e.Rune == 'k' || e.Rune == 'K') {
			if editor.palette.Open {
				editor.palette.Hide()
				ctx.RequestFocus(&editor.Chat)
			} else {
				editor.hideCompleters()
				editor.palette.Show()
				ctx.RequestFocus(&editor.palette)
			}
			ctx.ConsumeAndRedraw()
			return
		}
		if editor.palette.Open {
			editor.palette.Handle(ctx, e)
			if !editor.palette.Open {
				ctx.RequestFocus(&editor.Chat)
			}
			return
		}
		if editor.slash.Open && editor.mentionNavKey(e) {
			editor.slash.Handle(ctx, e)
			if !editor.slash.Open {
				editor.Chat.SlashOpen = false
			}
			return
		}
		if editor.mention.Open && editor.mentionNavKey(e) {
			editor.mention.Handle(ctx, e)
			if !editor.mention.Open {
				editor.Chat.MentionOpen = false
			}
			return
		}
		if e.Code == xui.KeyPageUp || e.Code == xui.KeyPageDown {
			editor.list.Handle(ctx, e)
			return
		}
		// Fallback: if focus was left on a transcript widget, still type into
		// the composer (keys bubble here when the focused widget ignores them).
		editor.Chat.Handle(ctx, e)
	case xui.MouseEvent:
		if editor.palette.Open {
			editor.palette.Handle(ctx, e)
			return
		}
		editor.handleListMouse(ctx, e)
	case xui.PasteEvent:
		if editor.palette.Open {
			editor.palette.Handle(ctx, e)
			return
		}
		editor.Chat.Handle(ctx, e)
	}
}

func (editor *Editor) mentionNavKey(e xui.KeyEvent) bool {
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

func (editor *Editor) hideCompleters() {
	editor.mention.Hide()
	editor.Chat.MentionOpen = false
	editor.mentionGen++
	editor.slash.Hide()
	editor.Chat.SlashOpen = false
}

func (editor *Editor) handleMentionChange(active bool, query string) {
	if !active {
		editor.mention.Hide()
		editor.Chat.MentionOpen = false
		editor.mentionGen++
		return
	}
	// Prefer slash when both could match (leading /).
	if editor.slash.Open || editor.Chat.SlashOpen {
		return
	}
	editor.slash.Hide()
	editor.Chat.SlashOpen = false
	editor.mention.Show()
	editor.Chat.MentionOpen = true
	if len(editor.mention.Items) == 0 {
		editor.mention.Status = "Searching…"
	}
	editor.scheduleMentionSearch(query)
}

func (editor *Editor) handleSlashChange(active bool, query string) {
	if !active {
		editor.slash.Hide()
		editor.Chat.SlashOpen = false
		return
	}
	editor.mention.Hide()
	editor.Chat.MentionOpen = false
	editor.mentionGen++
	items := FilterSlashCommands(query)
	s := ""
	if len(items) == 0 {
		s = "No matching commands"
	}
	editor.slash.SetResults(items, s)
	editor.slash.Show()
	editor.Chat.SlashOpen = true
}

func (editor *Editor) scheduleMentionSearch(query string) {
	editor.mentionGen++
	gen := editor.mentionGen
	cwd := editor.cwd
	go func() {
		time.Sleep(100 * time.Millisecond)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		paths, err := filesearch.Search(ctx, cwd, query, 20)
		msg := MentionResultsMsg{Gen: gen, Query: query, Paths: paths}
		if err != nil {
			msg.ErrText = err.Error()
		}
		editor.Publish(msg)
	}()
}

func (editor *Editor) applyMentionResults(msg MentionResultsMsg) {
	if msg.Gen != editor.mentionGen || !editor.mention.Open {
		return
	}
	if msg.ErrText != "" {
		editor.mention.SetResults(nil, msg.ErrText)
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
	editor.mention.SetResults(items, status)
}

func (editor *Editor) acceptMention(item mention.Item) {
	_, start, end, ok := chat.ActiveMention(editor.Chat.Value, editor.Chat.Cursor)
	if !ok {
		start, end = editor.Chat.Cursor, editor.Chat.Cursor
	}
	editor.mentionGen++
	editor.mention.Hide()
	editor.Chat.MentionOpen = false
	// Trailing space ends the mention token so the picker stays closed.
	editor.Chat.ReplaceRange(start, end, "@"+item.Path+" ")
}

func (editor *Editor) acceptSlash(item mention.Item) {
	_, start, end, ok := chat.ActiveSlash(editor.Chat.Value, editor.Chat.Cursor)
	if !ok {
		start, end = 0, editor.Chat.Cursor
	}
	insert := LookupSlashInsert(item.Path)
	if insert == "" {
		insert = "/" + item.Path
	}
	editor.Chat.ReplaceRange(start, end, insert)
	// ReplaceRange notifies slash completer and may reopen for no-arg inserts
	// (e.g. "/sessions"); force-close after the text update.
	editor.slash.Hide()
	editor.Chat.SlashOpen = false
	// No-arg commands (no trailing space): run immediately.
	if !strings.HasSuffix(insert, " ") {
		editor.Publish(SubmitMsg{Text: strings.TrimSpace(insert)})
		editor.drainBus()
	}
}

func (editor *Editor) Draw(ctx components.DrawContext) components.Surface {
	editor.drainBus()

	editor.tick++
	if editor.activity.ShowSpinner() && editor.tick%4 == 0 {
		editor.spin.Tick()
	}
	if editor.welcome.Sphere != nil {
		editor.welcome.Sphere.Time = time.Since(editor.startedAt).Seconds()
	}
	_ = editor.toast.Visible()

	maxSize := ctx.Max
	root := components.Surface{Size: maxSize, Widget: editor}

	footerH := 1
	var chatH int
	if editor.permAsk != nil {
		chatH = editor.permAsk.preferredAskHeight(maxSize.Width, ctx.Method)
		maxChatH := maxSize.Height - footerH - 3
		if chatH > maxChatH {
			chatH = maxChatH
		}
		if chatH < 8 {
			chatH = 8
		}
	} else {
		chatH = editor.Chat.PreferredHeight(maxSize.Width, ctx.Method)
		minChatH := 5
		if len(editor.Chat.PendingSkills) > 0 {
			minChatH++
		}
		if chatH < minChatH {
			chatH = minChatH
		}
		maxChatH := maxSize.Height - footerH - 3
		if maxChatH < minChatH {
			maxChatH = minChatH
		}
		if chatH > maxChatH {
			chatH = maxChatH
		}
	}
	listH := maxSize.Height - chatH - footerH
	if listH < 3 {
		listH = 3
		chatH = maxSize.Height - listH - footerH
		if chatH < 5 {
			chatH = 5
		}
	}
	editor.listH = listH

	var listSurf components.Surface
	if len(editor.list.Entries) == 0 {
		listSurf = editor.welcome.Draw(ctx.WithConstraints(components.Size{}, components.Size{Width: maxSize.Width, Height: listH}))
	} else {
		listSurf = editor.list.Draw(ctx.WithConstraints(components.Size{}, components.Size{Width: maxSize.Width, Height: listH}))
	}
	if editor.sel.active {
		hl := editor.theme.SelectionBg
		hl.Fg = editor.theme.SelectionFg.Fg
		ax, ay, ex, ey := editor.viewSel()
		components.ApplySelectionHighlight(&listSurf, ax, ay, ex, ey, hl)
	}
	editor.lastListSurf = listSurf

	var chatSurf components.Surface
	if editor.permAsk != nil {
		// Permission confirmation replaces the chat composer.
		chatSurf = editor.drawPermissionAsk(ctx, maxSize.Width, chatH)
	} else {
		chatSurf = editor.Chat.Draw(ctx.WithConstraints(components.Size{}, components.Size{Width: maxSize.Width, Height: chatH}))
	}
	footer := editor.drawFooter(ctx, maxSize.Width)

	root.Children = []components.SubSurface{
		{Origin: components.Point{X: 0, Y: 0}, Surface: listSurf},
		{Origin: components.Point{X: 0, Y: listH}, Surface: chatSurf, Z: 1},
		{Origin: components.Point{X: 0, Y: maxSize.Height - footerH}, Surface: footer, Z: 2},
	}
	if editor.permAsk == nil {
		if editor.slash.Open {
			editor.slash.AnchorBottomY = listH
			editor.slash.AnchorX = 0
			editor.slash.AnchorWidth = maxSize.Width
			panel := editor.slash.Draw(ctx)
			root.Children = append(root.Children, components.SubSurface{
				Origin:  components.Point{X: 0, Y: 0},
				Surface: panel,
				Z:       15,
			})
		}
		if editor.mention.Open {
			editor.mention.AnchorBottomY = listH
			editor.mention.AnchorX = 0
			editor.mention.AnchorWidth = maxSize.Width
			men := editor.mention.Draw(ctx)
			root.Children = append(root.Children, components.SubSurface{
				Origin:  components.Point{X: 0, Y: 0},
				Surface: men,
				Z:       15,
			})
		}
	}
	if editor.palette.Open {
		pal := editor.palette.Draw(ctx)
		root.Children = append(root.Children, components.SubSurface{
			Origin:  components.Point{X: 0, Y: 0},
			Surface: pal,
			Z:       20,
		})
	}
	if editor.toast.Visible() {
		toastSurf := editor.toast.Draw(ctx)
		root.Children = append(root.Children, components.SubSurface{
			Origin:  components.Point{X: 0, Y: 0},
			Surface: toastSurf,
			Z:       40,
		})
	}
	return root
}

func (editor *Editor) isBusy() bool {
	return session.IsStreaming(editor.snap)
}

func (editor *Editor) requestRedraw() {
	if editor.App != nil {
		editor.App.RequestRedraw()
	}
}

// SubmitPrompt is kept for callers; it publishes onto the bus.
func (editor *Editor) SubmitPrompt(text string) {
	editor.Publish(SubmitMsg{Text: text})
}
