// Package editor wires the TUI root widget and assembles domain panes.
package editor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	"github.com/pulseaiclub/phi/internal/tui/pathutil"
	"github.com/pulseaiclub/phi/internal/tui/submit"
	"github.com/pulseaiclub/phi/internal/tui/transcript"
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

	transcript *transcript.TranscriptPane
	composer   *composer.ComposerPane
	footer     *footer.FooterChrome
	overlays   *overlays.Overlays
	toast      toast.Toast

	ctrl *controller.Controller

	commands   *commands.CommandRegistry
	modelNames []string
	skillPath  string

	sessions  *commands.SessionCommands
	hookCmds  *commands.HookCommands
	submitter *submit.Submitter
}

// NewEditor builds the TUI panes and wires injected collaborators.
// application, bus, and ctrl must be non-nil. registry may be nil (builtins used).
func NewEditor(
	application *app.App,
	bus *controller.Bus,
	ctrl *controller.Controller,
	registry *commands.CommandRegistry,
	vx *xui.XUI,
	theme components.Theme,
	cwd, model, skillPath string,
	contextWindow int,
	modelNames []string,
) *Editor {
	if registry == nil {
		registry = commands.NewBuiltinRegistry()
	}
	e := &Editor{
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
	e.transcript = transcript.NewTranscriptPane(theme, e.footer.Spinner(), "Phi "+version.Version)
	e.transcript.SetUsageCallback(e.footer.UpdateTokenDisplay)
	e.footer.BindComposer(e.composer)
	e.footer.SetLabelContext(e.transcript.Snapshot)
	e.footer.SetLiveJobs(func() int {
		if e.ctrl != nil {
			return e.ctrl.LiveJobCount()
		}
		return 0
	})
	e.overlays = overlays.NewOverlays(
		theme,
		e.footer.Activity(),
		e.composer,
		func() {
			if e.App != nil {
				e.App.RequestFocus(e)
			}
		},
		func() {
			if e.App != nil {
				e.composer.FocusChat()
			}
		},
	)
	e.transcript.SetCopyHandlers(
		func(text string) bool {
			return e.vx != nil && e.vx.CopyToClipboard(text) == nil
		},
		func(msg string, kind toast.ToastKind, d time.Duration) {
			e.toast.Show(msg, kind, d)
		},
	)
	e.hookCmds = &commands.HookCommands{
		Registry: e.commands,
		Ctrl:     e.ctrl,
		CWD:      e.cwd,
		Composer: e.composer,
		Footer:   e.footer,
		Toast:    e.toast,
		Publish:  e.Publish,
	}
	e.sessions = commands.NewSessionCommands(
		e.ctrl,
		e.transcript,
		e.footer,
		e.toast,
		e.hookCmds.Sync,
	)

	var bridge *commandBridge
	bashRunner := submit.NewBashRunner(
		e.transcript,
		e.composer,
		func(msg string, kind toast.ToastKind, d time.Duration) {
			e.toast.Show(msg, kind, d)
		},
		e.Publish,
	)
	e.submitter = submit.NewSubmitter(
		e.ctrl,
		e.commands,
		e.transcript,
		e.footer.Activity(),
		e.composer,
		bashRunner,
		func() commands.CommandContext {
			if bridge == nil {
				return commands.CommandContext{}
			}
			return bridge.context()
		},
		e.Publish,
		e.overlays.PermissionActive,
		e.overlays.ContinueActive,
		e.overlays.ResolvePermission,
		e.overlays.ResolveContinue,
	)
	e.hookCmds.Submitter = e.submitter
	bridge = newCommandBridge(
		e.toast,
		e.composer,
		e.transcript,
		e.ctrl,
		e.submitter,
		e.sessions,
		e.reloadHooks,
		e.listHooks,
		e.setModel,
		e.applyTheme,
		e.setPermissions,
		e.setAgents,
		e.addPendingSkill,
		e.copyLastMessage,
		e.modelNames,
		e.skillPath,
	)
	e.hookCmds.CommandCtx = bridge.context
	e.composer.Wire(
		e.transcript,
		e.submitter,
		e.commands,
		e.cwd,
		e.Publish,
		e.drainBus,
		func() {
			if e.vx != nil {
				e.vx.QueueRefresh()
			}
		},
		e.overlays.BlocksComposer,
		e.overlays.HandlePermissionKey,
		e.overlays.HandleContinueKey,
		e.handleCopyKey,
		func() {
			if e.App != nil {
				e.App.RequestFocus(e)
			}
		},
		func(w components.Widget) {
			if e.App != nil {
				e.App.RequestFocus(w)
			}
		},
		func() {
			if e.ctrl != nil {
				e.ctrl.Close()
			}
		},
	)

	e.hookCmds.Sync()
	return e
}

// Publish sends a message onto the bus from any goroutine / widget callback.
func (e *Editor) Publish(m controller.Msg) {
	if e.bus == nil {
		return
	}
	e.bus.Publish(m)
}

// Update applies one message on the UI goroutine.
func (e *Editor) Update(m controller.Msg) {
	switch msg := m.(type) {
	case controller.SubmitMsg:
		e.submitter.Submit(msg.Text)
	case controller.CancelStreamMsg:
		e.submitter.Cancel()
	case controller.MentionResultsMsg:
		e.composer.ApplyMentionResults(msg)
	case controller.PermissionAskMsg, controller.PermissionDismissMsg,
		controller.ContinueAskMsg, controller.ContinueDismissMsg:
		e.overlays.Apply(m)
	case controller.SetActivityMsg, controller.ClearIfActivityMsg, controller.UpdateAvailableMsg:
		e.footer.Apply(m)
	case controller.HookSessionEffectsMsg:
		e.footer.Apply(m)
		if msg.Toast != "" {
			e.toast.Show(msg.Toast, toast.ToastSuccess, 3*time.Second)
		}
	case controller.BranchLabelMsg:
		e.composer.SetBranchLabel(msg.Text)
		if e.vx != nil {
			e.vx.QueueRefresh()
		}
	case controller.HookCommandResultMsg:
		if e.hookCmds != nil {
			e.hookCmds.Apply(msg)
		}
	case controller.JobProgressMsg:
		// Applied in drainBus so we can skip Sync when the tree is unchanged.
	case controller.RedrawMsg:
		// no state change; drain already requested redraw
	}
}

func (e *Editor) drainBus() {
	batch := e.bus.Drain()
	if len(batch) == 0 {
		return
	}
	atBottom := e.transcript.AtBottom()
	agentEvent := false
	for _, m := range batch {
		switch msg := m.(type) {
		case controller.SessionEventMsg:
			agentEvent = true
			e.transcript.ApplySession(msg.Event)
		case controller.JobProgressMsg:
			if e.transcript.ApplyJobProgress(msg.Progress) {
				agentEvent = true
			}
		default:
			e.Update(m)
		}
	}
	if agentEvent {
		e.transcript.Sync()
		e.footer.SyncFromSnap(e.transcript.Snapshot())
		if atBottom {
			e.transcript.StickToBottom()
		}
	}
}

func (e *Editor) Handle(ctx *components.EventContext, ev xui.Event) {
	e.composer.Handle(ctx, ev)
}

func (e *Editor) handleCopyKey(ctx *components.EventContext, ke xui.KeyEvent) bool {
	return e.transcript.HandleCopyKey(ctx, ke)
}

// Draw renders the editor surface for the given draw context.
func (e *Editor) Draw(ctx components.DrawContext) components.Surface {
	e.drainBus()

	if e.footer != nil {
		e.footer.AdvanceTick()
	}
	if e.footer != nil && e.footer.Activity().ShowSpinner() {
		ctx.WakeIn(spinnerInterval)
	}
	if e.toast.Visible() {
		// The frame that lands after Until removes the toast.
		ctx.WakeAt(e.toast.Until)
	}

	maxSize := ctx.Max
	root := components.Surface{Size: maxSize, Widget: e}

	footerH := 1
	var chatH int
	if askH, overlay := e.overlays.PreferredBottomHeight(maxSize.Width, ctx.Method); overlay {
		chatH = askH
		maxChatH := maxSize.Height - footerH - 3
		if chatH > maxChatH {
			chatH = maxChatH
		}
		if chatH < 8 {
			chatH = 8
		}
	} else {
		chatH = e.composer.PreferredHeight(maxSize.Width, ctx.Method)
		minChatH := 5
		if len(e.composer.Chat.PendingSkills) > 0 {
			minChatH++
		}
		if chatH < minChatH {
			chatH = minChatH
		}
		maxChatH := maxSize.Height - footerH - 3
		maxChatH = max(maxChatH, minChatH)
		if chatH > maxChatH {
			chatH = maxChatH
		}
	}
	listH := maxSize.Height - chatH - footerH
	if listH < 3 {
		listH = 3
		chatH = maxSize.Height - listH - footerH
		chatH = max(chatH, 5)
	}

	listSurf := e.transcript.Draw(ctx, maxSize.Width, listH)
	listH = e.transcript.ListHeight()

	var chatSurf components.Surface
	if surf, ok := e.overlays.DrawBottom(ctx, maxSize.Width, chatH); ok {
		chatSurf = surf
	} else {
		chatSurf = e.composer.DrawChat(ctx, maxSize.Width, chatH)
	}
	footerSurf := e.footer.Draw(ctx, maxSize.Width)

	root.Children = []components.SubSurface{
		{Origin: components.Point{X: 0, Y: 0}, Surface: listSurf},
		{Origin: components.Point{X: 0, Y: listH}, Surface: chatSurf, Z: 1},
		{Origin: components.Point{X: 0, Y: maxSize.Height - footerH}, Surface: footerSurf, Z: 2},
	}
	if !e.overlays.Active() {
		root.Children = append(root.Children, e.composer.PickerOverlays(ctx, listH, maxSize.Width)...)
	}
	if pal, ok := e.composer.PaletteOverlay(ctx); ok {
		root.Children = append(root.Children, pal)
	}
	if e.toast.Visible() {
		toastSurf := e.toast.Draw(ctx)
		root.Children = append(root.Children, components.SubSurface{
			Origin:  components.Point{X: 0, Y: 0},
			Surface: toastSurf,
			Z:       40,
		})
	}
	return root
}

func (e *Editor) requestRedraw() {
	if e.App != nil {
		e.App.RequestRedraw()
	}
}

// RequestRedraw asks the app to repaint (safe to bind onto controller.RedrawRelay / controller.Bus).
func (e *Editor) RequestRedraw() {
	e.requestRedraw()
}

func (e *Editor) addPendingSkill(name string) {
	e.composer.AddPendingSkill(name)
	if e.vx != nil {
		e.vx.QueueRefresh()
	}
}

// StartUpdateCheck queries GitHub for a newer release in the background and
// surfaces a footer hint when one is available. cacheDir is where the version
// check may store its cache (e.g. project global root); empty disables disk cache.
func (e *Editor) StartUpdateCheck(cacheDir string) {
	ch := update.CheckAsync(update.CheckOptions{
		Current:  version.Version,
		CacheDir: cacheDir,
	})
	go func() {
		info, ok := <-ch
		if !ok || !info.Available {
			return
		}
		e.Publish(controller.UpdateAvailableMsg{Latest: info.Latest, Current: info.Current})
	}()
}

// StartBranchWatch hot-reloads the git branch in the path label when the
// repo HEAD changes (checkout from another terminal, editor, …). Polling
// HEAD is a file read; the git process only runs after a real switch.
func (e *Editor) StartBranchWatch() {
	if e.cwd == "" {
		return
	}
	stop := make(chan struct{}) // lives for the process; Close is process exit
	go (&branchWatch{dir: e.cwd, interval: branchPollInterval}).run(stop, func(label string) {
		e.Publish(controller.BranchLabelMsg{Text: label})
	})
}

func (e *Editor) applyTheme(name string) {
	th, ok := components.ThemeByName(name)
	if !ok {
		return
	}
	e.theme = th
	e.composer.SetTheme(th)
	e.toast.Theme = th
	e.transcript.SetTheme(th)
	e.footer.SetTheme(th)
	e.overlays.SetTheme(th)
	e.toast.Show("Theme: "+name, toast.ToastSuccess, 2*time.Second)
	if e.vx != nil {
		e.vx.QueueRefresh()
	}
}

func (e *Editor) setModel(name string) {
	if err := e.ctrl.SetModel(name); err != nil {
		e.toast.Show(err.Error(), toast.ToastError, 3*time.Second)
		return
	}
	e.composer.SetModelLabel(name)
	e.toast.Show("Model: "+name, toast.ToastSuccess, 2*time.Second)
	if e.vx != nil {
		e.vx.QueueRefresh()
	}
}

func (e *Editor) setPermissions(bypass bool) {
	e.ctrl.SetAllowAll(bypass)
	kind := toast.ToastWarning
	msg := "Permissions: on (ask)"
	if bypass {
		kind = toast.ToastSuccess
		msg = "Permissions: off (allow all)"
	}
	e.toast.Show(msg, kind, 3*time.Second)
}

func (e *Editor) setAgents(enabled bool) {
	e.ctrl.SetAgentsEnabled(enabled)
	msg := "Sub-agents: off"
	if enabled {
		msg = "Sub-agents: on"
	}
	e.toast.Show(msg, toast.ToastSuccess, 2*time.Second)
}

func (e *Editor) reloadHooks() {
	n, warns, err := e.ctrl.ReloadHooks()
	if err != nil {
		e.toast.Show("Hooks reload: "+err.Error(), toast.ToastError, 3*time.Second)
		return
	}
	e.hookCmds.Sync()
	msg := fmt.Sprintf("Hooks: reloaded %d", n)
	if len(warns) > 0 {
		msg = fmt.Sprintf("Hooks: reloaded %d (%d warning(s))", n, len(warns))
		e.toast.Show(msg, toast.ToastWarning, 3*time.Second)
		return
	}
	e.toast.Show(msg, toast.ToastSuccess, 2*time.Second)
}

func (e *Editor) listHooks() []palette.PaletteCommand {
	found, warns, err := e.ctrl.ListHooks()
	return commands.HookListEntries(found, warns, err)
}

func (e *Editor) copyLastMessage() {
	e.transcript.CopyBlock(e.transcript.LastCopyText())
}

// SubmitPrompt publishes a user prompt onto the bus.
func (e *Editor) SubmitPrompt(text string) {
	e.Publish(controller.SubmitMsg{Text: text})
}

type commandBridge struct {
	toast      toast.Toast
	composer   *composer.ComposerPane
	transcript *transcript.TranscriptPane
	ctrl       *controller.Controller
	submitter  *submit.Submitter
	sessions   *commands.SessionCommands

	reloadHooks     func()
	listHooks       func() []palette.PaletteCommand
	setModel        func(string)
	applyTheme      func(string)
	setPermissions  func(bool)
	setAgents       func(bool)
	addSkill        func(string)
	copyLastMessage func()

	modelNames []string
	skillPath  string
}

func newCommandBridge(
	toast toast.Toast,
	composer *composer.ComposerPane,
	transcript *transcript.TranscriptPane,
	ctrl *controller.Controller,
	submitter *submit.Submitter,
	sessions *commands.SessionCommands,
	reloadHooks func(),
	listHooks func() []palette.PaletteCommand,
	setModel func(string),
	applyTheme func(string),
	setPermissions func(bool),
	setAgents func(bool),
	addSkill func(string),
	copyLastMessage func(),
	modelNames []string,
	skillPath string,
) *commandBridge {
	return &commandBridge{
		toast:           toast,
		composer:        composer,
		transcript:      transcript,
		ctrl:            ctrl,
		submitter:       submitter,
		sessions:        sessions,
		reloadHooks:     reloadHooks,
		listHooks:       listHooks,
		setModel:        setModel,
		applyTheme:      applyTheme,
		setPermissions:  setPermissions,
		setAgents:       setAgents,
		addSkill:        addSkill,
		copyLastMessage: copyLastMessage,
		modelNames:      append([]string(nil), modelNames...),
		skillPath:       skillPath,
	}
}

func (b *commandBridge) context() commands.CommandContext {
	if b == nil {
		return commands.CommandContext{}
	}
	return commands.CommandContext{
		Toast: func(msg string, kind toast.ToastKind, d time.Duration) {
			b.toast.Show(msg, kind, d)
		},
		PushSubmenu: func(title string, cmds []palette.PaletteCommand) {
			b.composer.PushPalette(title, cmds)
		},
		ShowSessions:  b.sessions.Show,
		ResumeSession: b.sessions.Resume,
		ClearSession: func() {
			if b.submitter != nil && b.submitter.StreamActive() {
				b.toast.Show("Cannot clear while a reply or command is running", toast.ToastWarning, 3*time.Second)
				return
			}
			b.sessions.Clear()
		},
		SetModel:        b.setModel,
		ApplyTheme:      b.applyTheme,
		SetPermissions:  b.setPermissions,
		SetAgents:       b.setAgents,
		ReloadHooks:     b.reloadHooks,
		ListHooks:       b.listHooks,
		AddSkill:        b.addSkill,
		CopyLastMessage: b.copyLastMessage,
		ModelNames:      b.modelNames,
		SkillPath:       b.skillPath,
	}
}

const branchPollInterval = time.Second

// spinnerInterval is the footer spinner glyph rate while an activity is in
// flight; the app loop draws only on these wakes.
const spinnerInterval = time.Second / 15

type branchWatch struct {
	dir      string
	interval time.Duration
}

func (b *branchWatch) run(stop <-chan struct{}, publish func(label string)) {
	if b.interval <= 0 {
		b.interval = branchPollInterval
	}
	last := branchState(b.dir)
	ticker := time.NewTicker(b.interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
		}
		if cur := branchState(b.dir); cur != last {
			last = cur
			publish(pathutil.PathWithBranch(b.dir))
		}
	}
}

func branchState(dir string) string {
	gitDir := resolveGitDir(dir)
	data, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return "missing"
	}
	return strings.TrimSpace(string(data))
}

func resolveGitDir(dir string) string {
	dotGit := filepath.Join(dir, ".git")
	if data, err := os.ReadFile(dotGit); err == nil {
		if target, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir:"); ok {
			target = strings.TrimSpace(target)
			if !filepath.IsAbs(target) {
				target = filepath.Join(dir, target)
			}
			return target
		}
	}
	return dotGit
}
