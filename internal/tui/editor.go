package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/app"
	"github.com/pulseaiclub/phi/internal/components/palette"
	"github.com/pulseaiclub/phi/internal/components/status"
	"github.com/pulseaiclub/phi/internal/components/toast"
	"github.com/pulseaiclub/phi/internal/session"
	"github.com/pulseaiclub/phi/internal/tui/controller"
	"github.com/pulseaiclub/phi/internal/util/update"
	"github.com/pulseaiclub/phi/internal/version"
)

// Editor is the TUI root widget: layout composition and the UI-goroutine
// message loop. Cross-component work goes through controller.Bus — producers Publish,
// Draw drains and Update applies. Agent lifecycle lives in controller.Controller;
// session→widget projection lives in TranscriptPane (Mapper/SubagentStore).
//
// Construction: cmd assembles App, controller.Bus, controller.Controller, CommandRegistry and passes
// them into NewEditor. Editor does not create controller.Controller or fetch the project singleton.
type Editor struct {
	vx    *xui.XUI
	App   *app.App
	theme components.Theme
	bus   *controller.Bus
	cwd   string

	transcript *TranscriptPane
	composer   *ComposerPane
	toast      toast.Toast
	spin       *status.Spinner
	activity   *controller.ActivityHandler
	tick       int

	// Session→widget projection lives in transcript. Mapper/subagents are owned there.
	ctrl *controller.Controller

	contextWindow int
	lastUsage     session.TokenUsage

	updateHint string // footer right: "vX.Y.Z available · phi update"
	hookStatus string // footer left override from command/session hooks

	permAsk     *permAskState
	continueAsk *continueAskState

	commands   *CommandRegistry
	modelNames []string
	skillPath  string

	layout    *EditorLayout
	sessions  *SessionActions
	hookCmds  *HookCommands
	submitter *Submitter
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
		toast:         toast.Toast{Theme: theme},
		composer:      NewComposerPane(theme, model, cwd),
		spin:          status.NewSpinner(theme.ToolName),
	}
	editor.transcript = NewTranscriptPane(theme, editor.spin, "Phi "+version.Version)
	editor.transcript.SetUsageCallback(editor.updateTokenDisplay)
	editor.transcript.SetCopyHandlers(
		func(text string) bool {
			return editor.vx != nil && editor.vx.CopyToClipboard(text) == nil
		},
		func(msg string, kind toast.ToastKind, d time.Duration) {
			editor.toast.Show(msg, kind, d)
		},
	)
	editor.activity = controller.NewActivityHandler(editor.spin)
	editor.layout = &EditorLayout{e: editor}
	editor.sessions = &SessionActions{e: editor}
	editor.hookCmds = &HookCommands{e: editor}

	bashRunner := NewBashRunner(BashRunnerDeps{
		Transcript: editor.transcript,
		Composer:   editor.composer,
		Toast: func(msg string, kind toast.ToastKind, d time.Duration) {
			editor.toast.Show(msg, kind, d)
		},
		Publish: editor.Publish,
	})
	editor.submitter = NewSubmitter(SubmitterDeps{
		Ctrl:       editor.ctrl,
		Bus:        editor.bus,
		Commands:   editor.commands,
		Transcript: editor.transcript,
		Activity:   editor.activity,
		Composer:   editor.composer,
		Bash:       bashRunner,
		CommandContext: func() CommandContext {
			return editor.commandContext()
		},
		Toast: func(msg string, kind toast.ToastKind, d time.Duration) {
			editor.toast.Show(msg, kind, d)
		},
		Publish: editor.Publish,
		PermissionActive: func() bool {
			return editor.permAsk != nil
		},
		ContinueActive: func() bool {
			return editor.continueAsk != nil
		},
		ResolvePermission: editor.resolvePermission,
		ResolveContinue:   editor.resolveContinue,
	})
	editor.composer.Wire(ComposerWire{
		Transcript: editor.transcript,
		Submitter:  editor.submitter,
		Commands:   editor.commands,
		CWD:        editor.cwd,
		Publish:    editor.Publish,
		DrainBus:   editor.drainBus,
		OnRedraw: func() {
			if editor.vx != nil {
				editor.vx.QueueRefresh()
			}
		},
		OverlayBlocksComposer: func() bool {
			return editor.permAsk != nil || editor.continueAsk != nil
		},
		HandlePermissionKey: editor.handlePermissionKey,
		HandleContinueKey:   editor.handleContinueKey,
		HandleCopyKey:       editor.handleCopyKey,
		RequestFocusEditor: func() {
			if editor.App != nil {
				editor.App.RequestFocus(editor)
			}
		},
		RequestFocus: func(w components.Widget) {
			if editor.App != nil {
				editor.App.RequestFocus(w)
			}
		},
		CtrlClose: func() {
			if editor.ctrl != nil {
				editor.ctrl.Close()
			}
		},
	})

	editor.sessions.Register(editor.commands)
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
			editor.composer.PushPalette(title, cmds)
		},
		ShowSessions:  editor.sessions.Show,
		ResumeSession: editor.sessions.Resume,
		ClearSession: func() {
			if editor.submitter.StreamActive() {
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
	editor.composer.AddPendingSkill(name)
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
	editor.composer.SetTheme(th)
	editor.toast.Theme = th
	editor.transcript.SetTheme(th)
	if editor.spin != nil {
		editor.spin.Style = th.ToolName
	}
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
	editor.composer.SetModelLabel(name)
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
	editor.transcript.CopyBlock(editor.transcript.LastCopyText())
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
		editor.submitter.Submit(msg.Text)
	case controller.CancelStreamMsg:
		editor.submitter.Cancel()
	case controller.SetActivityMsg:
		editor.activity.Apply(msg.Activity)
	case controller.ClearIfActivityMsg:
		if editor.activity.Current == msg.If {
			editor.activity.Apply(controller.ActivityIdle)
		}
	case controller.MentionResultsMsg:
		editor.composer.ApplyMentionResults(msg)
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
				editor.composer.FocusChat()
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
				editor.composer.FocusChat()
			}
		}
	case controller.UpdateAvailableMsg:
		latest := strings.TrimPrefix(msg.Latest, "v")
		editor.updateHint = latest + " available · phi update"
	case controller.BranchLabelMsg:
		editor.composer.SetBranchLabel(msg.Text)
		if editor.vx != nil {
			editor.vx.QueueRefresh()
		}
	case controller.HookCommandResultMsg:
		if editor.hookCmds != nil {
			editor.hookCmds.Apply(msg)
		}
	case controller.HookSessionEffectsMsg:
		if msg.StatusSet {
			editor.hookStatus = msg.Status
		}
		if msg.Toast != "" {
			editor.toast.Show(msg.Toast, toast.ToastSuccess, 3*time.Second)
		}
	case controller.JobProgressMsg:
		// Applied in drainBus so we can skip Sync when the tree is unchanged.
	case controller.RedrawMsg:
		// no state change; drain already requested redraw
	}
}

func (editor *Editor) drainBus() {
	batch := editor.bus.Drain()
	if len(batch) == 0 {
		return
	}
	atBottom := editor.transcript.AtBottom()
	threadDirty := false
	for _, m := range batch {
		switch msg := m.(type) {
		case controller.SessionEventMsg:
			threadDirty = true
			editor.transcript.ApplySession(msg.Event)
		case controller.JobProgressMsg:
			if editor.transcript.ApplyJobProgress(msg.Progress) {
				threadDirty = true
			}
		default:
			editor.Update(m)
		}
	}
	if threadDirty {
		editor.transcript.Sync()
		editor.activity.SyncFromSnap(editor.transcript.Snapshot())
		if atBottom {
			editor.transcript.StickToBottom()
		}
	}
}

func (editor *Editor) Handle(ctx *components.EventContext, ev xui.Event) {
	editor.composer.Handle(ctx, ev)
}

func (editor *Editor) handleCopyKey(ctx *components.EventContext, e xui.KeyEvent) bool {
	return editor.transcript.HandleCopyKey(ctx, e)
}

// Draw renders via EditorLayout.
func (editor *Editor) Draw(ctx components.DrawContext) components.Surface {
	return editor.layout.Draw(ctx)
}

func (editor *Editor) applySessionEvent(ev session.Event) {
	editor.transcript.ApplySession(ev)
}

// syncThread rebuilds transcript widgets (delegates to TranscriptPane).
func (editor *Editor) syncThread() {
	editor.transcript.Sync()
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
