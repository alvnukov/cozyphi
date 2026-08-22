package shell

import (
	"fmt"
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/app"
	"github.com/pulseaiclub/phi/internal/components/palette"
	"github.com/pulseaiclub/phi/internal/components/toast"
	"github.com/pulseaiclub/phi/internal/tui/commands"
	"github.com/pulseaiclub/phi/internal/tui/composer"
	"github.com/pulseaiclub/phi/internal/tui/controller"
	"github.com/pulseaiclub/phi/internal/tui/footer"
	"github.com/pulseaiclub/phi/internal/tui/overlays"
	"github.com/pulseaiclub/phi/internal/tui/submit"
	"github.com/pulseaiclub/phi/internal/tui/transcript"
	"github.com/pulseaiclub/phi/internal/util/update"
	"github.com/pulseaiclub/phi/internal/version"
)

// Shell is the TUI root widget: layout composition and the UI-goroutine
// message loop. Cross-component work goes through controller.Bus — producers Publish,
// Draw drains and Update applies. Agent lifecycle lives in controller.Controller;
// session→widget projection lives in TranscriptPane (Mapper/SubagentStore).
//
// Construction: cmd assembles App, controller.Bus, controller.Controller, CommandRegistry and passes
// them into NewShell. Shell does not create controller.Controller or fetch the project singleton.
type Shell struct {
	vx    *xui.XUI
	App   *app.App
	theme components.Theme
	bus   *controller.Bus
	cwd   string

	transcript *transcript.TranscriptPane
	composer   *composer.ComposerPane
	footer     *footer.FooterChrome
	overlays   *overlays.Overlays
	toast      toast.Toast

	ctrl *controller.Controller

	commands   *commands.CommandRegistry
	modelNames []string
	skillPath  string

	layout    *ShellLayout
	sessions  *commands.SessionCommands
	hookCmds  *commands.HookCommands
	submitter *submit.Submitter
}

// NewShell builds the TUI panes and wires injected collaborators.
// application, bus, and ctrl must be non-nil. commands may be nil (builtins used).
func NewShell(
	application *app.App,
	bus *controller.Bus,
	ctrl *controller.Controller,
	registry *commands.CommandRegistry,
	vx *xui.XUI,
	theme components.Theme,
	cwd, model, skillPath string,
	contextWindow int,
	modelNames []string,
) *Shell {
	if registry == nil {
		registry = commands.NewBuiltinRegistry()
	}
	sh := &Shell{
		vx:         vx,
		App:        application,
		theme:      theme,
		cwd:        cwd,
		bus:        bus,
		ctrl:       ctrl,
		modelNames: append([]string(nil), modelNames...),
		skillPath:  skillPath,
		commands:   registry,
		toast:      toast.Toast{Theme: theme},
		composer:   composer.NewComposerPane(theme, model, cwd),
		footer:     footer.NewFooterChrome(theme, contextWindow),
	}
	sh.transcript = transcript.NewTranscriptPane(theme, sh.footer.Spinner(), "Phi "+version.Version)
	sh.transcript.SetUsageCallback(sh.footer.UpdateTokenDisplay)
	sh.footer.BindComposer(sh.composer)
	sh.footer.SetLabelContext(sh.transcript.Snapshot)
	sh.footer.SetLiveJobs(func() int {
		if sh.ctrl != nil {
			return sh.ctrl.LiveJobCount()
		}
		return 0
	})
	sh.overlays = overlays.NewOverlays(
		theme,
		sh.footer.Activity(),
		sh.composer,
		func() {
			if sh.App != nil {
				sh.App.RequestFocus(sh)
			}
		},
		func() {
			if sh.App != nil {
				sh.composer.FocusChat()
			}
		},
	)
	sh.transcript.SetCopyHandlers(
		func(text string) bool {
			return sh.vx != nil && sh.vx.CopyToClipboard(text) == nil
		},
		func(msg string, kind toast.ToastKind, d time.Duration) {
			sh.toast.Show(msg, kind, d)
		},
	)
	sh.layout = &ShellLayout{s: sh}
	sh.hookCmds = &commands.HookCommands{
		Registry: sh.commands,
		Ctrl:     sh.ctrl,
		CWD:      sh.cwd,
		Composer: sh.composer,
		Footer:   sh.footer,
		Toast:    sh.toast,
		Publish:  sh.Publish,
	}
	sh.sessions = commands.NewSessionCommands(commands.SessionCommandsDeps{
		Ctrl:       sh.ctrl,
		Transcript: sh.transcript,
		Footer:     sh.footer,
		Toast:      sh.toast,
		SyncHooks:  sh.hookCmds.Sync,
	})

	var bridge *CommandBridge
	bashRunner := submit.NewBashRunner(submit.BashRunnerDeps{
		Transcript: sh.transcript,
		Composer:   sh.composer,
		Toast: func(msg string, kind toast.ToastKind, d time.Duration) {
			sh.toast.Show(msg, kind, d)
		},
		Publish: sh.Publish,
	})
	sh.submitter = submit.NewSubmitter(submit.SubmitterDeps{
		Ctrl:       sh.ctrl,
		Bus:        sh.bus,
		Commands:   sh.commands,
		Transcript: sh.transcript,
		Activity:   sh.footer.Activity(),
		Composer:   sh.composer,
		Bash:       bashRunner,
		CommandContext: func() commands.CommandContext {
			if bridge == nil {
				return commands.CommandContext{}
			}
			return bridge.Context()
		},
		Toast: func(msg string, kind toast.ToastKind, d time.Duration) {
			sh.toast.Show(msg, kind, d)
		},
		Publish:           sh.Publish,
		PermissionActive:  sh.overlays.PermissionActive,
		ContinueActive:    sh.overlays.ContinueActive,
		ResolvePermission: sh.overlays.ResolvePermission,
		ResolveContinue:   sh.overlays.ResolveContinue,
	})
	sh.hookCmds.Submitter = sh.submitter
	bridge = NewCommandBridge(CommandBridgeDeps{
		Toast:           sh.toast,
		Composer:        sh.composer,
		Transcript:      sh.transcript,
		Ctrl:            sh.ctrl,
		Submitter:       sh.submitter,
		Sessions:        sh.sessions,
		ReloadHooks:     sh.reloadHooks,
		ListHooks:       sh.listHooks,
		SetModel:        sh.setModel,
		ApplyTheme:      sh.applyTheme,
		SetPermissions:  sh.setPermissions,
		SetAgents:       sh.setAgents,
		AddSkill:        sh.addPendingSkill,
		CopyLastMessage: sh.copyLastMessage,
		ModelNames:      sh.modelNames,
		SkillPath:       sh.skillPath,
	})
	sh.hookCmds.CommandCtx = bridge.Context
	sh.composer.Wire(composer.ComposerWire{
		Transcript: sh.transcript,
		Submitter:  sh.submitter,
		Commands:   sh.commands,
		CWD:        sh.cwd,
		Publish:    sh.Publish,
		DrainBus:   sh.drainBus,
		OnRedraw: func() {
			if sh.vx != nil {
				sh.vx.QueueRefresh()
			}
		},
		OverlayBlocksComposer: sh.overlays.BlocksComposer,
		HandlePermissionKey:   sh.overlays.HandlePermissionKey,
		HandleContinueKey:     sh.overlays.HandleContinueKey,
		HandleCopyKey:         sh.handleCopyKey,
		RequestFocusEditor: func() {
			if sh.App != nil {
				sh.App.RequestFocus(sh)
			}
		},
		RequestFocus: func(w components.Widget) {
			if sh.App != nil {
				sh.App.RequestFocus(w)
			}
		},
		CtrlClose: func() {
			if sh.ctrl != nil {
				sh.ctrl.Close()
			}
		},
	})

	sh.hookCmds.Sync()
	return sh
}

func (sh *Shell) addPendingSkill(name string) {
	sh.composer.AddPendingSkill(name)
	if sh.vx != nil {
		sh.vx.QueueRefresh()
	}
}

// StartUpdateCheck queries GitHub for a newer release in the background and
// surfaces a footer hint when one is available. cacheDir is where the version
// check may store its cache (e.g. project global root); empty disables disk cache.
func (sh *Shell) StartUpdateCheck(cacheDir string) {
	ch := update.CheckAsync(update.CheckOptions{
		Current:  version.Version,
		CacheDir: cacheDir,
	})
	go func() {
		info, ok := <-ch
		if !ok || !info.Available {
			return
		}
		sh.Publish(controller.UpdateAvailableMsg{Latest: info.Latest, Current: info.Current})
	}()
}

// StartBranchWatch hot-reloads the git branch in the path label when the
// repo HEAD changes (checkout from another terminal, editor, …). Polling
// HEAD is a file read; the git process only runs after a real switch.
func (sh *Shell) StartBranchWatch() {
	if sh.cwd == "" {
		return
	}
	stop := make(chan struct{}) // lives for the process; Close is process exit
	go (&branchWatch{dir: sh.cwd, interval: branchPollInterval}).run(stop, func(label string) {
		sh.Publish(controller.BranchLabelMsg{Text: label})
	})
}

func (sh *Shell) applyTheme(name string) {
	th, ok := components.ThemeByName(name)
	if !ok {
		return
	}
	sh.theme = th
	sh.composer.SetTheme(th)
	sh.toast.Theme = th
	sh.transcript.SetTheme(th)
	sh.footer.SetTheme(th)
	sh.overlays.SetTheme(th)
	sh.toast.Show("Theme: "+name, toast.ToastSuccess, 2*time.Second)
	if sh.vx != nil {
		sh.vx.QueueRefresh()
	}
}

func (sh *Shell) setModel(name string) {
	if err := sh.ctrl.SetModel(name); err != nil {
		sh.toast.Show(err.Error(), toast.ToastError, 3*time.Second)
		return
	}
	sh.composer.SetModelLabel(name)
	sh.toast.Show("Model: "+name, toast.ToastSuccess, 2*time.Second)
	if sh.vx != nil {
		sh.vx.QueueRefresh()
	}
}

func (sh *Shell) setPermissions(bypass bool) {
	sh.ctrl.SetAllowAll(bypass)
	kind := toast.ToastWarning
	msg := "Permissions: on (ask)"
	if bypass {
		kind = toast.ToastSuccess
		msg = "Permissions: off (allow all)"
	}
	sh.toast.Show(msg, kind, 3*time.Second)
}

func (sh *Shell) setAgents(enabled bool) {
	sh.ctrl.SetAgentsEnabled(enabled)
	msg := "Sub-agents: off"
	if enabled {
		msg = "Sub-agents: on"
	}
	sh.toast.Show(msg, toast.ToastSuccess, 2*time.Second)
}

func (sh *Shell) reloadHooks() {
	n, warns, err := sh.ctrl.ReloadHooks()
	if err != nil {
		sh.toast.Show("Hooks reload: "+err.Error(), toast.ToastError, 3*time.Second)
		return
	}
	sh.hookCmds.Sync()
	msg := fmt.Sprintf("Hooks: reloaded %d", n)
	if len(warns) > 0 {
		msg = fmt.Sprintf("Hooks: reloaded %d (%d warning(s))", n, len(warns))
		sh.toast.Show(msg, toast.ToastWarning, 3*time.Second)
		return
	}
	sh.toast.Show(msg, toast.ToastSuccess, 2*time.Second)
}

func (sh *Shell) listHooks() []palette.PaletteCommand {
	found, warns, err := sh.ctrl.ListHooks()
	return commands.HookListEntries(found, warns, err)
}

func (sh *Shell) copyLastMessage() {
	sh.transcript.CopyBlock(sh.transcript.LastCopyText())
}

// SubmitPrompt publishes a user prompt onto the bus.
func (sh *Shell) SubmitPrompt(text string) {
	sh.Publish(controller.SubmitMsg{Text: text})
}
