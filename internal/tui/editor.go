package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/pulseaiclub/xui"

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
	"github.com/pulseaiclub/phi/internal/tools"
	"github.com/pulseaiclub/phi/internal/tui/controller"
	uitranscript "github.com/pulseaiclub/phi/internal/tui/transcript"
	"github.com/pulseaiclub/phi/internal/util/update"
	"github.com/pulseaiclub/phi/internal/version"
)

// Editor is the TUI root widget: layout composition and the UI-goroutine
// message loop. Cross-component work goes through controller.Bus — producers Publish,
// Draw drains and Update applies. Agent lifecycle lives in controller.Controller;
// session→widget projection lives in transcript.Mapper; activity status in controller.ActivityHandler.
//
// Construction: cmd assembles App, controller.Bus, controller.Controller, CommandRegistry and passes
// them into NewEditor. Editor does not create controller.Controller or fetch the project singleton.
type Editor struct {
	vx    *xui.XUI
	App   *app.App
	theme components.Theme
	bus   *controller.Bus
	cwd   string

	list      transcript.MessageList
	Chat      chat.ChatInput
	palette   palette.CommandPalette
	mention   mention.Picker
	slash     mention.Picker
	toast     toast.Toast
	spin      *status.Spinner
	welcome   splash.Screen
	activity  *controller.ActivityHandler
	startedAt time.Time
	tick      int

	listH        int
	lastListSurf components.Surface
	sel          textSel

	// Session model. Mutations happen only on the UI goroutine via Update.
	snap session.Snapshot

	mapper  *uitranscript.Mapper
	ctrl    *controller.Controller
	listIDs []string // parallels list.Entries (item ids)

	subagents *uitranscript.SubagentStore

	mentionGen int // bumped to invalidate in-flight @-file searches

	contextWindow int
	lastUsage     session.TokenUsage

	updateHint string // footer right: "vX.Y.Z available · phi update"

	permAsk     *permAskState
	continueAsk *continueAskState

	commands   *CommandRegistry
	modelNames []string
	skillPath  string

	layout   *EditorLayout
	input    *InputRouter
	sessions *SessionActions
	hookCmds *HookCommands
	bash     *BashMode
}

func newChatInput(theme components.Theme, model, cwd string) chat.ChatInput {
	return chat.ChatInput{
		MinBodyRows:    3,
		MaxBodyRows:    8,
		UseBlockCursor: false, // terminal cursor only; reverse cells ghost on CJK delete
		PaddingX:       1,
		Theme:          theme,
		BorderStyle:    theme.Border,
		TextStyle:      theme.Foreground,
		CursorStyle:    xui.Style{Reverse: true},
		TopRightLabel: layout.BorderLabel{
			Text:  model,
			Style: theme.Success,
		},
		BottomRightLabel: layout.BorderLabel{
			Text:  pathWithBranch(cwd),
			Style: pathLabelStyle(theme),
		},
		// BottomLeftLabel (context + token stats) filled by updateTokenDisplay.
	}
}

// NewEditor builds the editor widgets and wires injected collaborators.
// application, bus, and ctrl must be non-nil. commands may be nil (builtins used).
func NewEditor(
	application *app.App,
	bus *controller.Bus,
	ctrl *controller.Controller,
	commands *CommandRegistry,
	vx *xui.XUI,
	theme components.Theme,
	cwd, model, skillPath string,
	contextWindow int,
	modelNames []string,
) *Editor {
	if commands == nil {
		commands = NewBuiltinRegistry()
	}
	editor := &Editor{
		vx:            vx,
		App:           application,
		theme:         theme,
		cwd:           cwd,
		bus:           bus,
		ctrl:          ctrl,
		contextWindow: contextWindow,
		modelNames:    append([]string(nil), modelNames...),
		skillPath:     skillPath,
		commands:      commands,
		Chat:          newChatInput(theme, model, cwd),
		spin:          status.NewSpinner(theme.ToolName),
		startedAt:     time.Now(),
		welcome: splash.Screen{
			Sphere: &splash.Sphere{Fast: true},
			Theme:  theme,
			Brand:  "Phi " + version.Version,
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
	editor.activity = controller.NewActivityHandler(editor.spin)
	editor.mapper = uitranscript.NewMapper(theme, editor.spin, func() {
		editor.list.InvalidateHeights()
	})
	editor.subagents = uitranscript.NewSubagentStore()
	editor.mapper.Children = editor.subagents.Children
	editor.mapper.ChildrenByJob = editor.subagents.ChildrenByJob
	editor.palette.FocusReturn = &editor.Chat
	editor.Chat.OnSubmit = func(text string) {
		// OnSubmit runs on the UI goroutine — publish and apply immediately
		// so the composer clears before the next frame.
		editor.Publish(controller.SubmitMsg{Text: text})
		editor.drainBus()
	}
	editor.layout = &EditorLayout{e: editor}
	editor.input = &InputRouter{e: editor}
	editor.sessions = &SessionActions{e: editor}
	editor.hookCmds = &HookCommands{e: editor}
	editor.bash = &BashMode{e: editor}
	editor.sessions.Register(editor.commands)
	editor.Chat.OnChange = func(text string) {
		editor.bash.SyncBorder(text)
		// CJK paste/delete can desync tty wide-glyph columns vs our damage
		// grid; force a full redraw so ghost cells cannot stick around.
		if editor.vx != nil {
			editor.vx.QueueRefresh()
		}
	}
	editor.Chat.OnMentionChange = editor.input.OnMentionChange
	editor.Chat.OnSlashChange = editor.input.OnSlashChange
	editor.mention.OnAccept = editor.input.AcceptMention
	editor.slash.OnAccept = editor.input.AcceptSlash
	editor.hookCmds.Sync()
	return editor
}

// commandContext builds the capability surface for slash/palette commands.
func (editor *Editor) commandContext() CommandContext {
	return CommandContext{
		Toast: func(msg string, kind toast.ToastKind, d time.Duration) {
			editor.toast.Show(msg, kind, d)
		},
		PushSubmenu: func(title string, cmds []palette.PaletteCommand) {
			editor.palette.Push(title, cmds)
		},
		ShowSessions:  editor.sessions.Show,
		ResumeSession: editor.sessions.Resume,
		ClearSession: func() {
			if editor.streamActive() {
				editor.toast.Show("Cannot clear while a reply or command is running", toast.ToastWarning, 3*time.Second)
				return
			}
			editor.sessions.Clear()
		},
		SetModel:        editor.setModel,
		ApplyTheme:      editor.applyTheme,
		SetPermissions:  editor.setPermissions,
		SetAgents:       editor.setAgents,
		ReloadHooks:     editor.reloadHooks,
		ListHooks:       editor.listHooks,
		AddSkill:        editor.addPendingSkill,
		CopyLastMessage: editor.copyLastMessage,
		ModelNames:      editor.modelNames,
		SkillPath:       editor.skillPath,
	}
}

func (editor *Editor) addPendingSkill(name string) {
	editor.Chat.AddPendingSkill(name)
	if editor.vx != nil {
		editor.vx.QueueRefresh()
	}
}

// StartUpdateCheck queries GitHub for a newer release in the background and
// surfaces a footer hint when one is available. cacheDir is where the version
// check may store its cache (e.g. project global root); empty disables disk cache.
func (editor *Editor) StartUpdateCheck(cacheDir string) {
	ch := update.CheckAsync(update.CheckOptions{
		Current:  version.Version,
		CacheDir: cacheDir,
	})
	go func() {
		info, ok := <-ch
		if !ok || !info.Available {
			return
		}
		editor.Publish(controller.UpdateAvailableMsg{Latest: info.Latest, Current: info.Current})
	}()
}

// StartBranchWatch hot-reloads the git branch in the path label when the
// repo HEAD changes (checkout from another terminal, editor, …). Polling
// HEAD is a file read; the git process only runs after a real switch.
func (editor *Editor) StartBranchWatch() {
	if editor.cwd == "" {
		return
	}
	stop := make(chan struct{}) // lives for the process; Close is process exit
	go (&branchWatch{dir: editor.cwd, interval: branchPollInterval}).run(stop, func(label string) {
		editor.Publish(controller.BranchLabelMsg{Text: label})
	})
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
	editor.Chat.BottomRightLabel.Style = pathLabelStyle(th)
	editor.Chat.TopRightLabel.Style = th.Success
	editor.palette.Theme = th
	editor.mention.Theme = th
	editor.slash.Theme = th
	editor.toast.Theme = th
	editor.welcome.Theme = th
	editor.list.Theme = th
	editor.bash.SyncBorder(editor.Chat.Value)
	if editor.spin != nil {
		editor.spin.Style = th.ToolName
	}
	if editor.mapper != nil {
		editor.mapper.SetTheme(th)
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

// setModel handles the model-switch palette command.
func (editor *Editor) setModel(name string) {
	if err := editor.ctrl.SetModel(name); err != nil {
		editor.toast.Show(err.Error(), toast.ToastError, 3*time.Second)
		return
	}
	editor.Chat.TopRightLabel.Text = name
	editor.toast.Show("Model: "+name, toast.ToastSuccess, 2*time.Second)
	if editor.vx != nil {
		editor.vx.QueueRefresh()
	}
}

// setPermissions handles the permissions-toggle palette command.
// bypass=true means no permission prompts (allow all).
func (editor *Editor) setPermissions(bypass bool) {
	editor.ctrl.SetAllowAll(bypass)
	kind := toast.ToastWarning
	msg := "Permissions: on (ask)"
	if bypass {
		kind = toast.ToastSuccess
		msg = "Permissions: off (allow all)"
	}
	editor.toast.Show(msg, kind, 3*time.Second)
}

// setAgents handles the agents-toggle palette command.
func (editor *Editor) setAgents(enabled bool) {
	editor.ctrl.SetAgentsEnabled(enabled)
	msg := "Sub-agents: off"
	if enabled {
		msg = "Sub-agents: on"
	}
	editor.toast.Show(msg, toast.ToastSuccess, 2*time.Second)
}

// reloadHooks handles the hooks reload palette command.
func (editor *Editor) reloadHooks() {
	n, warns, err := editor.ctrl.ReloadHooks()
	if err != nil {
		editor.toast.Show("Hooks reload: "+err.Error(), toast.ToastError, 3*time.Second)
		return
	}
	editor.hookCmds.Sync()
	msg := fmt.Sprintf("Hooks: reloaded %d", n)
	if len(warns) > 0 {
		msg = fmt.Sprintf("Hooks: reloaded %d (%d warning(s))", n, len(warns))
		editor.toast.Show(msg, toast.ToastWarning, 3*time.Second)
		return
	}
	editor.toast.Show(msg, toast.ToastSuccess, 2*time.Second)
}

// listHooks builds the hooks list page for the command palette.
func (editor *Editor) listHooks() []palette.PaletteCommand {
	found, warns, err := editor.ctrl.ListHooks()
	return HookListEntries(found, warns, err)
}

// copyLastMessage copies the last transcript message to the clipboard.
func (editor *Editor) copyLastMessage() {
	editor.copyBlock(editor.list.LastCopyText())
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
		case *block.AgentBlock:
			b.Theme = th
		}
	}
}

// Publish sends a message onto the bus from any goroutine / widget callback.
func (editor *Editor) Publish(m controller.Msg) {
	if editor.bus == nil {
		return
	}
	editor.bus.Publish(m)
}

// Update applies one message on the UI goroutine. Returns whether a redraw is useful.
func (editor *Editor) Update(m controller.Msg) {
	switch msg := m.(type) {
	case controller.SubmitMsg:
		editor.handleSubmit(msg.Text)
	case controller.CancelStreamMsg:
		editor.handleCancel()
	case controller.SessionEventMsg:
		editor.applySessionEvent(msg.Event)
	case controller.SetActivityMsg:
		editor.activity.Apply(msg.Activity)
	case controller.ClearIfActivityMsg:
		if editor.activity.Current == msg.If {
			editor.activity.Apply(controller.ActivityIdle)
		}
	case controller.MentionResultsMsg:
		editor.input.ApplyMentionResults(msg)
	case controller.PermissionAskMsg:
		editor.beginPermissionAsk(msg)
	case controller.PermissionDismissMsg:
		// Agent already timed out / cancelled; only clear the overlay.
		wasAsk := editor.permAsk != nil
		editor.permAsk = nil
		if wasAsk {
			if editor.activity.Current == controller.ActivityAwaitingApproval {
				editor.activity.Apply(controller.ActivityTools)
			}
			if editor.App != nil {
				editor.App.RequestFocus(&editor.Chat)
			}
		}
	case controller.ContinueAskMsg:
		editor.beginContinueAsk(msg)
	case controller.ContinueDismissMsg:
		wasAsk := editor.continueAsk != nil
		editor.continueAsk = nil
		if wasAsk {
			if editor.activity.Current == controller.ActivityAwaitingApproval {
				editor.activity.Apply(controller.ActivityTools)
			}
			if editor.App != nil {
				editor.App.RequestFocus(&editor.Chat)
			}
		}
	case controller.UpdateAvailableMsg:
		latest := strings.TrimPrefix(msg.Latest, "v")
		editor.updateHint = latest + " available · phi update"
	case controller.BranchLabelMsg:
		editor.Chat.BottomRightLabel.Text = msg.Text
		if editor.vx != nil {
			editor.vx.QueueRefresh()
		}
	case controller.HookCommandResultMsg:
		if editor.hookCmds != nil {
			editor.hookCmds.Apply(msg)
		}
	case controller.JobProgressMsg:
		// Applied in drainBus so we can skip Sync when the tree is unchanged.
	case controller.RedrawMsg:
		// no state change; drain already requested redraw
	}
}

// syncThread rebuilds transcript widgets and invalidates only rows whose
// height-relevant content changed (heights are remapped by entry id first).
func (editor *Editor) syncThread() {
	if editor.mapper == nil {
		return
	}
	oldIDs := editor.listIDs
	entries, ids, dirty := editor.mapper.Sync(editor.list.Entries, editor.listIDs, editor.snap)
	editor.list.ReindexHeights(oldIDs, ids)
	editor.list.Entries = entries
	editor.listIDs = ids
	editor.list.InvalidateHeightsAt(dirty...)
}

func (editor *Editor) drainBus() {
	batch := editor.bus.Drain()
	if len(batch) == 0 {
		return
	}
	atBottom := editor.list.ScrollFromBottom == 0
	threadDirty := false
	for _, m := range batch {
		switch msg := m.(type) {
		case controller.SessionEventMsg:
			threadDirty = true
			editor.Update(m)
		case controller.JobProgressMsg:
			if editor.subagents.ApplyProgress(msg.Progress) {
				threadDirty = true
			}
		default:
			editor.Update(m)
		}
	}
	if threadDirty {
		editor.syncThread()
		editor.activity.SyncFromSnap(editor.snap)
		// Follow mode only when the user is pinned to the bottom; scrolling
		// up must not jump back on stream/progress ticks.
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
	if td, ok := ev.(session.ToolData); ok {
		editor.applyAgentToolData(td)
	}
}

func (editor *Editor) applyAgentToolData(td session.ToolData) {
	name := strings.ToLower(td.Run.Name)
	switch name {
	case "agent_spawn", "agent_wait":
	default:
		return
	}
	parsed := tools.ParseAgentResult(td.Run.Output)
	if !parsed.OK {
		return
	}
	editor.subagents.Bind(parsed.JobID, td.Run.ToolUseID)
	editor.subagents.ApplyResult(td.Run.ToolUseID, parsed)
}

func (editor *Editor) handleSubmit(text string) {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "!") {
		if editor.bash.HandleSubmit(text) {
			return
		}
	}
	if strings.HasPrefix(text, "/") {
		if editor.handleSlash(text) {
			editor.input.HideCompleters()
			editor.Chat.Value = ""
			editor.Chat.Cursor = 0
			editor.bash.SyncBorder("")
			return
		}
	}
	pendingSkills := make([]string, 0, len(editor.Chat.PendingSkills))
	pendingSkills = append(pendingSkills, editor.Chat.PendingSkills...)
	if (text == "" && len(pendingSkills) == 0) || editor.isBusy() {
		return
	}

	editor.mention.Hide()
	editor.Chat.MentionOpen = false
	editor.mentionGen++
	editor.slash.Hide()
	editor.Chat.SlashOpen = false

	editor.activity.Apply(controller.ActivitySubmitting)
	display := text
	if display == "" && len(pendingSkills) > 0 {
		display = "Skills: " + strings.Join(pendingSkills, ", ")
	}
	editor.applySessionEvent(session.UserAppend{Text: display})
	editor.syncThread()
	editor.list.StickToBottom()
	editor.activity.Apply(controller.ActivityWaiting)

	editor.Chat.Value = ""
	editor.Chat.Cursor = 0
	editor.Chat.ClearPendingSkills()

	editor.ctrl.StartPrompt(text, pendingSkills)
}

// handleSlash runs registered `/` commands. Returns true when the input was consumed.
func (editor *Editor) handleSlash(text string) bool {
	if editor.commands == nil {
		return false
	}
	return editor.commands.DispatchSlash(text, editor.commandContext())
}

func (editor *Editor) streamActive() bool {
	if editor.isBusy() || editor.permAsk != nil || editor.continueAsk != nil {
		return true
	}
	switch editor.activity.Current {
	case controller.ActivitySubmitting,
		controller.ActivityWaiting,
		controller.ActivityStreaming,
		controller.ActivityTools,
		controller.ActivityCompacting,
		controller.ActivityAwaitingApproval,
		controller.ActivityRetrying:
		return true
	default:
		return false
	}
}

func (editor *Editor) handleCancel() {
	if editor.permAsk != nil {
		editor.resolvePermission(controller.AskReply{})
	}
	if editor.continueAsk != nil {
		editor.resolveContinue(controller.ContinueReply{})
	}
	if editor.bash.Cancel() {
		return
	}
	editor.ctrl.Cancel()
	editor.applySessionEvent(session.CancelStreaming{})
	editor.syncThread()
	editor.activity.Apply(controller.ActivityCancelled)
	time.AfterFunc(1200*time.Millisecond, func() {
		editor.Publish(controller.ClearIfActivityMsg{If: controller.ActivityCancelled})
	})
}

func (editor *Editor) Handle(ctx *components.EventContext, ev xui.Event) {
	editor.input.Handle(ctx, ev)
}

// Draw renders via EditorLayout.
func (editor *Editor) Draw(ctx components.DrawContext) components.Surface {
	return editor.layout.Draw(ctx)
}

func (editor *Editor) isBusy() bool {
	return session.IsStreaming(editor.snap) || editor.bash.Running()
}

func (editor *Editor) requestRedraw() {
	if editor.App != nil {
		editor.App.RequestRedraw()
	}
}

// RequestRedraw asks the app to repaint (safe to bind onto controller.RedrawRelay / controller.Bus).
func (editor *Editor) RequestRedraw() {
	editor.requestRedraw()
}

// SubmitPrompt is kept for callers; it publishes onto the bus.
func (editor *Editor) SubmitPrompt(text string) {
	editor.Publish(controller.SubmitMsg{Text: text})
}
