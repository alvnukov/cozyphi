// Package editor wires the TUI root widget and assembles domain panes.
package editor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/app"
	"github.com/alvnukov/cozyphi/internal/components/palette"
	"github.com/alvnukov/cozyphi/internal/components/slot"
	"github.com/alvnukov/cozyphi/internal/components/toast"
	"github.com/alvnukov/cozyphi/internal/harnesssettings"
	"github.com/alvnukov/cozyphi/internal/history"
	"github.com/alvnukov/cozyphi/internal/llm/skills"
	"github.com/alvnukov/cozyphi/internal/notify"
	"github.com/alvnukov/cozyphi/internal/provider"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/session/compaction"
	"github.com/alvnukov/cozyphi/internal/tools/questiontool"
	"github.com/alvnukov/cozyphi/internal/tui/commands"
	"github.com/alvnukov/cozyphi/internal/tui/composer"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/ctxpane"
	"github.com/alvnukov/cozyphi/internal/tui/footer"
	"github.com/alvnukov/cozyphi/internal/tui/helppane"
	"github.com/alvnukov/cozyphi/internal/tui/keys"
	"github.com/alvnukov/cozyphi/internal/tui/overlays"
	"github.com/alvnukov/cozyphi/internal/tui/pathutil"
	"github.com/alvnukov/cozyphi/internal/tui/planedit"
	"github.com/alvnukov/cozyphi/internal/tui/settings"
	"github.com/alvnukov/cozyphi/internal/tui/sidebar"
	"github.com/alvnukov/cozyphi/internal/tui/submit"
	"github.com/alvnukov/cozyphi/internal/tui/transcript"
	"github.com/alvnukov/cozyphi/internal/tui/usagepane"
	"github.com/alvnukov/cozyphi/internal/tui/watchpane"
	"github.com/alvnukov/cozyphi/internal/util"
	"github.com/alvnukov/cozyphi/internal/util/update"
	"github.com/alvnukov/cozyphi/internal/version"
	"github.com/alvnukov/cozyphi/internal/watch"
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
	// footerY is the screen row the footer took on the last frame, -1 before
	// the first: a click on that row is read back into the footer's watch
	// indicator.
	footerY   int
	sidebar   *sidebar.Sidebar
	overlays  *overlays.Overlays
	toast     toast.Toast
	ctxpane   *ctxpane.Pane
	watches   *watchpane.Pane
	usagepane *usagepane.Pane
	help      *helppane.Pane
	settings  *settings.Pane
	planPane  *planedit.Pane

	ctrl *controller.Controller

	commands   *commands.CommandRegistry
	modelNames []string
	skillPath  string
	// discoveredSkills caches the session's skill names; the discovery root
	// never changes mid-session, so the plan-settings tab reads names, not
	// directories.
	discoveredSkills []string
	skillsResolved   bool

	sessions  *commands.SessionCommands
	hookCmds  *commands.HookCommands
	submitter *submit.Submitter

	// notifier pings the OS when the model stops or waits for input; nil
	// (the default) disables notifications entirely.
	notifier attentionNotifier
	// watchList reads the session's watches: the footer counts the live
	// ones, the transcript marks their start rows, and a finished turn sends
	// no ping while one runs. Tests swap in a fixed list.
	watchList func() []watch.Watch

	terminalWidth int

	// lastCtrlC is when the last Ctrl+C that found nothing to interrupt was
	// pressed; a second one inside ctrlCExitWindow quits the app.
	lastCtrlC time.Time
}

// NewEditor builds the TUI panes and wires injected collaborators.
// application, bus, and ctrl must be non-nil. registry may be nil (builtins used).
// hist may be nil — the composer then works without prompt history.
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
	hist *history.Store,
	settingsStores ...settings.Store,
) *Editor {
	if ctrl != nil {
		modelNames = mergeModelNames(modelNames, ctrl.ModelNames())
	}
	if registry == nil {
		registry = commands.NewBuiltinRegistry()
	}
	if len(modelNames) > 0 {
		// /model with the configured names; two adapters make the arg
		// completer seam real (/theme is the static one).
		registry.RegisterModelCommand(modelNames)
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
		composer:   composer.NewComposerPane(theme, model, cwd, hist),
		footer:     footer.NewFooterChrome(theme, contextWindow),
		footerY:    -1,
		sidebar:    sidebar.NewSidebar(theme, contextWindow),
	}
	if len(settingsStores) > 0 && settingsStores[0] != nil {
		e.settings = settings.New(theme, settingsStores[0], func() { e.composer.FocusChat() })
		e.settings.SetSkills(e.skillNames())
		if e.ctrl != nil {
			e.settings.SetTypeInUse(e.ctrl.PlanUsesType)
			e.settings.SetAvailableTools(e.ctrl.ToolNames())
			e.applySettings(settingsStores[0].Snapshot())
			e.settings.SetOnApplied(e.applySettings)
		}
	}
	if ctrl != nil {
		e.planPane = planedit.New(theme, planStore{ctrl: ctrl}, func() { e.composer.FocusChat() })
		// The same catalog the settings pane and the plan tool see: the
		// skills picker offers it, and names outside it wear a warning.
		e.planPane.SetSkills(e.skillNames())
	}
	e.transcript = transcript.NewTranscriptPane(theme, e.footer.Spinner(), version.Version)
	// One usage flow feeds every display: the composer border label (footer)
	// and the status sidebar.
	e.transcript.SetUsageCallback(func(u session.TokenUsage) {
		e.footer.UpdateTokenDisplay(u)
		e.sidebar.UpdateUsage(u)
	})
	if e.ctrl != nil {
		// A session whose permission boundary could not be built denies every
		// tool call. Saying so once beats letting the user rediscover it in
		// each refusal.
		if reason := e.ctrl.GateFailure(); reason != "" {
			e.toast.Show(
				"Permissions unavailable, tool calls are denied: "+reason,
				toast.ToastError,
				10*time.Second,
			)
		}
		// Same once-at-startup courtesy for a session without a model: the
		// notice says how to get one (or names the automatically picked
		// fallback), so the first refused submit is not the first hint.
		if notice := e.ctrl.ModelSetupNotice(); notice != "" {
			e.toast.Show(notice, toast.ToastWarning, 10*time.Second)
		}
		e.sidebar.SetRuntime(sidebar.Runtime{
			Model:        e.ctrl.EffectiveModelName(),
			SessionModel: e.ctrl.ModelName(),
			Mode:         string(e.ctrl.Mode()),
			MCP:          e.ctrl.MCPStatuses(),
			LSP:          e.ctrl.LSPStatuses(),
		})
		e.sidebar.SetPlan(e.ctrl.Plan())
		preferences := controller.SidebarPreferences{Visible: true, ExpandEdits: true}
		loaded, err := e.ctrl.SidebarPreferences()
		if err != nil {
			e.toast.Show("Cannot load sidebar preferences: "+err.Error(), toast.ToastWarning, 4*time.Second)
		} else {
			preferences = loaded
		}
		e.sidebar.ConfigureWidth(preferences.Width, e.ctrl.SaveSidebarWidth)
		e.sidebar.ConfigureVisibility(preferences.Visible, e.ctrl.SaveSidebarVisibility)
		e.sidebar.ConfigureApprove(e.ctrl.SetPlanApproved)
		e.ctrl.SetPlanAutoApprove(e.sidebar.AutoApprove)
		e.sidebar.ConfigureClearPlan(e.ctrl.ClearPlan)
		e.sidebar.ConfigureModels(e.commands.RankModels(modelNames))
		// A step-model pick is a model choice like any other: credit it so every
		// model picker converges on one order. Clearing the override (empty
		// model) is not a choice.
		e.sidebar.ConfigureStepModel(func(stepID, model string) error {
			err := e.ctrl.SetStepModel(stepID, model)
			if err == nil {
				e.commands.RecordModel(model)
			}
			return err
		})
		e.sidebar.ConfigureSkillToggle(e.ctrl.SetStepSkill)
		setStop := func(enabled bool) error {
			if err := e.ctrl.SaveStopLimit(enabled); err != nil {
				return err
			}
			e.ctrl.SetStopOnLimit(enabled)
			return nil
		}
		e.sidebar.ConfigureStopOnLimit(preferences.StopOnLimit, setStop)
		e.ctrl.SetStopOnLimit(preferences.StopOnLimit)
		setPlan := func(enabled bool) error {
			if err := e.ctrl.SavePlanFeature(enabled); err != nil {
				return err
			}
			e.ctrl.SetPlanEnabled(enabled)
			e.applyPlanVisibility(enabled)
			return nil
		}
		e.sidebar.ConfigurePlanFeature(preferences.PlanEnabled, setPlan)
		e.ctrl.SetPlanEnabled(preferences.PlanEnabled)
		e.applyPlanVisibility(preferences.PlanEnabled)
		setEdits := func(enabled bool) error {
			if err := e.ctrl.SaveExpandEdits(enabled); err != nil {
				return err
			}
			e.transcript.SetExpandEdits(enabled)
			return nil
		}
		e.sidebar.ConfigureExpandEdits(preferences.ExpandEdits, setEdits)
		e.transcript.SetExpandEdits(preferences.ExpandEdits)
	}
	e.footer.BindComposer(e.composer)
	e.footer.SetLabelContext(e.transcript.Snapshot)
	e.footer.SetModelSource(func() string { return e.ctrl.EffectiveModelName() })
	e.footer.SetLiveJobs(func() int {
		if e.ctrl != nil {
			return e.ctrl.LiveJobCount()
		}
		return 0
	})
	e.watchList = func() []watch.Watch {
		if e.ctrl != nil {
			return e.ctrl.WatchList()
		}
		return nil
	}
	e.footer.SetLiveWatches(func() []watch.Watch { return e.watchList() })
	// The transcript tells a still-running watch's start row apart from
	// the finished ones by the same list the footer counts.
	e.transcript.SetLiveWatches(func() []transcript.WatchRef {
		var live []transcript.WatchRef
		for _, w := range e.watchList() {
			if w.Live {
				live = append(live, transcript.WatchRef{ID: w.ID, Label: w.Label})
			}
		}
		return live
	})
	e.footer.SetSessionID(func() string {
		if e.ctrl != nil {
			return e.ctrl.SessionID()
		}
		return ""
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
	// Composer copy/cut chords share the clipboard and confirm with a toast,
	// so selection copy in the input feels the same as transcript copy.
	e.composer.SetChatCopyFunc(func(text string) bool {
		if e.vx == nil {
			return false
		}
		if err := e.vx.CopyToClipboard(text); err != nil {
			// Surface the failure: a silent false would make the claimed
			// Ctrl+C a dead key with no hint why nothing was copied.
			e.toast.Show("Cannot copy: "+err.Error(), toast.ToastError, 3*time.Second)
			return false
		}
		e.toast.Show("Copied to clipboard", toast.ToastSuccess, 2*time.Second)
		return true
	})
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
		e.commandContext,
		e.Publish,
		e.overlays.PermissionActive,
		e.overlays.ContinueActive,
		e.overlays.ResolvePermission,
		e.overlays.ResolveContinue,
	)
	e.hookCmds = commands.NewHookCommands(
		e.commands,
		e.ctrl,
		e.cwd,
		e.composer,
		e.footer,
		e.submitter,
		e.toast,
		e.Publish,
		e,
	)
	e.sessions = commands.NewSessionCommands(
		e.ctrl,
		e.transcript,
		e.footer,
		e.sidebar,
		e.toast,
		e.hookCmds.Sync,
	)
	e.composer.Wire(
		e.transcript,
		e.submitter,
		e.commands,
		e.cwd,
		e,
		e,
	)

	e.ctxpane = ctxpane.New(
		theme,
		e.ctrl.ContextView,
		e.RunCompact,
		func(entryID string) error {
			if e.submitter != nil && !e.submitter.CanSubmit() {
				e.toast.Show("Cannot trim while a reply or command is running", toast.ToastWarning, 3*time.Second)
				return errors.New("busy")
			}
			if err := e.ctrl.TrimContextFrom(entryID); err != nil {
				e.toast.Show("Cannot trim context: "+err.Error(), toast.ToastError, 4*time.Second)
				return err
			}
			e.toast.Show("Context trimmed", toast.ToastSuccess, 3*time.Second)
			return nil
		},
		func(ids []string) error {
			if e.submitter != nil && !e.submitter.CanSubmit() {
				e.toast.Show("Cannot delete while a reply or command is running", toast.ToastWarning, 3*time.Second)
				return errors.New("busy")
			}
			if err := e.ctrl.DropContextEntries(ids); err != nil {
				e.toast.Show("Cannot delete context blocks: "+err.Error(), toast.ToastError, 4*time.Second)
				return err
			}
			e.toast.Show(fmt.Sprintf("Deleted %d context block(s)", len(ids)), toast.ToastSuccess, 3*time.Second)
			return nil
		},
		// Closing the browser hands the keyboard back to the composer.
		func() { e.composer.FocusChat() },
	)

	e.help = helppane.New(theme, func() { e.composer.FocusChat() })

	// The watch browser reads and stops watches through the controller's
	// watch seams — never the manager directly. Stop errors surface as a
	// toast; closing hands the keyboard back, exactly like the ctxpane.
	e.watches = watchpane.New(
		theme,
		e.ctrl.WatchList,
		e.ctrl.WatchLog,
		func(id string) error {
			if err := e.ctrl.StopWatch(id); err != nil {
				e.toast.Show("Cannot stop watch: "+err.Error(), toast.ToastError, 4*time.Second)
				return err
			}
			e.toast.Show("Watch stopped", toast.ToastSuccess, 3*time.Second)
			return nil
		},
		func() { e.composer.FocusChat() },
	)

	// The usage browser pulls session totals through the controller seam and
	// asks it for a quota fetch; closing hands the keyboard back to the
	// composer, exactly like the other full-screen panes.
	e.usagepane = usagepane.New(
		theme,
		e.ctrl.SessionStats,
		func() { e.ctrl.FetchQuota(context.Background()) },
		func() { e.composer.FocusChat() },
	)

	// Startup replay (cozyphi --continue / --resume): when the controller booted
	// on an existing session the transcript must carry the history before the
	// first frame. A fresh session has an empty snapshot — nothing to load.
	if e.ctrl != nil {
		if snap := e.ctrl.ReplaySnapshot(); len(snap.Messages) > 0 {
			e.transcript.LoadReplay(snap)
			e.transcript.Sync()
			e.transcript.StickToBottom()
		}
	}

	// Ctrl+K rebuilds the root list on every open: usage ranking and command
	// visibility must reflect current state, not the startup snapshot.
	e.composer.SetPaletteRefresh(func() []palette.PaletteCommand {
		return e.commands.BuildPalette(e.commandContext())
	})
	e.hookCmds.Sync()

	// Posture label: the controller owns the mode; the label follows it.
	if e.ctrl != nil {
		e.composer.SetMode(e.ctrl.Mode())
	}
	return e
}

// applySettings puts a committed settings snapshot into effect without a
// restart: notification mode and sound reach the live notifier, compaction
// thresholds go to the controller, and agent model pins reload from the
// project config so the next spawn resolves them.
func (e *Editor) applySettings(snap harnesssettings.Snapshot) {
	if e.notifier != nil {
		e.notifier.Reconfigure(snap.Notifications.Mode, snap.Notifications.Sound)
	}
	e.ctrl.SetTasksAccess(snap.Tasks)
	e.ctrl.SetCompactionSettings(compaction.ConfiguredSettings(snap.Compaction.ReminderTokens))
	// agents.models pins live in the project config; reload it so the
	// next spawn resolves them without a restart.
	if err := e.ctrl.RefreshProjectConfig(); err != nil {
		e.toast.Show("Agent model pins may be stale: "+err.Error(), toast.ToastWarning, 4*time.Second)
		return
	}
	if stale := e.ctrl.AgentModelWarnings(); len(stale) > 0 {
		e.toast.Show(
			"Unknown model in agents.models (inherit): "+strings.Join(stale, ", "),
			toast.ToastWarning,
			4*time.Second,
		)
	}
}

// attentionNotifier pings the user outside the terminal when the model
// finishes a turn or waits for an answer. *notify.Notifier is the production
// adapter; a fake covers editor wiring in tests.
type attentionNotifier interface {
	SetFocused(focused bool)
	SetOnFailure(handle func(error))
	TurnEnded()
	NeedsAttention(detail string)
	Reconfigure(mode notify.Mode, sound string)
}

// SetAttentionNotifier wires OS notifications for agent state changes. The
// terminal's focus reports reach the notifier through Handle, so the
// unfocused mode only pings when the user is actually elsewhere.
func (e *Editor) SetAttentionNotifier(n attentionNotifier) {
	e.notifier = n
	if n == nil {
		return
	}
	// The sender fails on its own goroutine, so the report rides the bus onto
	// the UI thread like any other background result.
	n.SetOnFailure(func(err error) {
		e.Publish(controller.NotifierFailedMsg{ErrText: err.Error()})
	})
}

// watchRunning reports whether any watch is still live.
func (e *Editor) watchRunning() bool {
	if e.watchList == nil {
		return false
	}
	for _, w := range e.watchList() {
		if w.Live {
			return true
		}
	}
	return false
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
		e.submitter.Submit(msg.Text, msg.Media...)
	case controller.ModeToggleMsg:
		if e.ctrl != nil {
			e.composer.SetMode(e.ctrl.ToggleMode())
		}
	case controller.CancelStreamMsg:
		e.submitter.Cancel()
	case controller.PlanUpdatedMsg:
		e.sidebar.SetPlan(msg.Plan)
	case controller.MentionResultsMsg:
		e.composer.ApplyMentionResults(msg)
	case controller.PermissionAskMsg:
		e.overlays.Apply(m)
		if e.notifier != nil {
			// The tool name is the context the user needs at a glance.
			e.notifier.NeedsAttention(msg.Request.Tool)
		}
	case controller.ContinueAskMsg:
		e.overlays.Apply(m)
		if e.notifier != nil {
			e.notifier.NeedsAttention(fmt.Sprintf("continue for %d more rounds?", msg.MaxRounds))
		}
	case controller.QuestionAskMsg:
		e.overlays.Apply(m)
		if e.notifier != nil {
			e.notifier.NeedsAttention(questionDetail(msg.Questions))
		}
	case controller.PermissionDismissMsg, controller.ContinueDismissMsg, controller.QuestionDismissMsg:
		e.overlays.Apply(m)
	case controller.PermissionPersistedMsg:
		// The permanent rule leaves a visible trace either way: the file
		// it landed in, or the fact that it never landed.
		if msg.ErrText != "" {
			e.toast.Show(
				"Could not write the allow-all rule to "+pathutil.ShortPath(msg.Path)+": "+msg.ErrText,
				toast.ToastError,
				6*time.Second,
			)
			break
		}
		e.toast.Show(
			"Allow-all rule written to "+pathutil.ShortPath(msg.Path),
			toast.ToastSuccess,
			5*time.Second,
		)
	case controller.NotifierFailedMsg:
		e.toast.Show(
			"Desktop notifications are off: "+msg.ErrText,
			toast.ToastWarning,
			5*time.Second,
		)
	case controller.ProviderCatalogMsg:
		e.overlays.Apply(m)
		if msg.ErrText != "" {
			e.toast.Show("Provider catalog refresh failed: "+msg.ErrText, toast.ToastWarning, 5*time.Second)
		}
	case controller.ProviderDeviceCodeMsg:
		e.overlays.Apply(m)
		if msg.ErrText != "" {
			e.toast.Show("Cannot start subscription sign-in: "+msg.ErrText, toast.ToastError, 5*time.Second)
		}
	case controller.ProviderAuthorizationMsg:
		e.overlays.Apply(m)
		if msg.ErrText != "" {
			e.toast.Show("Cannot start subscription sign-in: "+msg.ErrText, toast.ToastError, 5*time.Second)
		}
	case controller.ProviderConnectResultMsg:
		e.overlays.Apply(m)
		if msg.ErrText != "" {
			e.toast.Show("Cannot save provider credential: "+msg.ErrText, toast.ToastError, 5*time.Second)
			break
		}
		e.refreshModelCommands()
		if msg.WarningText != "" {
			e.toast.Show(msg.WarningText, toast.ToastWarning, 6*time.Second)
		} else {
			e.toast.Show("Provider credential saved: "+msg.ProviderID, toast.ToastSuccess, 3*time.Second)
		}
	case controller.ProviderModelsUpdatedMsg:
		if msg.ErrText != "" {
			e.toast.Show("Cannot refresh subscription models: "+msg.ErrText, toast.ToastWarning, 5*time.Second)
			break
		}
		e.refreshModelCommands()
	case controller.UsageQuotaMsg:
		// The fetch the pane started lands here; the pane decides what to
		// render, including the fetch-for-a-closed-pane case.
		if e.usagepane != nil {
			e.usagepane.Apply(msg)
		}
	case controller.SetActivityMsg, controller.ClearIfActivityMsg, controller.UpdateAvailableMsg:
		e.footer.Apply(m)
	case controller.RunEndedMsg:
		e.footer.Apply(m)
		// A live watch wakes the session by itself, so this turn's end is
		// not a wait for input: the ping waits for the last watch to go.
		if e.notifier != nil && !e.watchRunning() {
			e.notifier.TurnEnded()
		}
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
		if atBottom {
			e.transcript.StickToBottom()
		}
	}
}

// questionDetail picks the most recognizable line of the first question —
// the header when present, else the question text — so the notification body
// names what the model is asking about. Empty falls back to the notifier's
// default body.
func questionDetail(questions []questiontool.Question) string {
	if len(questions) == 0 {
		return ""
	}
	if q := questions[0]; q.Header != "" {
		return q.Header
	} else if q.Question != "" {
		return q.Question
	}
	return ""
}

// modalActive reports whether a full-screen modal (harness settings or the
// plan editor) covers the screen and owns keyboard input; composer overlays
// stay hidden behind it.
func (e *Editor) modalActive() bool {
	return (e.settings != nil && e.settings.Visible()) ||
		(e.planPane != nil && e.planPane.Visible())
}

// AcceptInterrupt claims Ctrl+C as an interrupt so the chord stops work
// instead of killing the session. The press cancels the innermost thing in
// flight — a modal ask, then a shell command or agent run, then an unsent
// draft. With nothing left to stop it arms the exit and says so; the next
// Ctrl+C within ctrlCExitWindow returns false and the app quits.
func (e *Editor) AcceptInterrupt() bool {
	if e.interruptWork() {
		e.lastCtrlC = time.Time{}
		return true
	}
	now := time.Now()
	if !e.lastCtrlC.IsZero() && now.Sub(e.lastCtrlC) <= ctrlCExitWindow {
		return false
	}
	e.lastCtrlC = now
	e.toast.Show("Press Ctrl+C again to exit", toast.ToastWarning, ctrlCExitWindow)
	return true
}

// interruptWork cancels one layer of in-flight work and reports whether it
// found any. Layers unwind one press at a time, the way Escape does: an ask
// is declined before the run behind it is cancelled, and the draft is cleared
// only once the session is idle.
func (e *Editor) interruptWork() bool {
	if e.overlays.CancelActive() {
		return true
	}
	if e.submitter != nil && !e.submitter.CanSubmit() {
		e.submitter.Cancel()
		return true
	}
	if e.composer != nil && strings.TrimSpace(e.composer.Chat.Value) != "" {
		e.composer.ClearInput()
		return true
	}
	return false
}

func (e *Editor) Handle(ctx *components.EventContext, ev xui.Event) {
	// Focus reports are observed, not consumed: unfocused notifications gate
	// on them even while a modal owns the keyboard. The composer still gets
	// the event below.
	if fe, ok := ev.(xui.FocusEvent); ok && e.notifier != nil {
		e.notifier.SetFocused(fe.Focused)
	}
	if e.settings != nil && e.settings.Visible() && e.settings.HandleEvent(ctx, ev) {
		return
	}
	if e.planPane != nil && e.planPane.Visible() && e.planPane.HandleEvent(ctx, ev) {
		return
	}
	if e.overlays.HandleConnectEvent(ctx, ev) {
		return
	}
	// A modal ask owns the keyboard, so its text field is the only place a paste
	// can land while it is up.
	if pe, ok := ev.(xui.PasteEvent); ok && e.overlays.HandleAskPaste(ctx, pe) {
		return
	}
	// The help screen covers everything below it, F1 included — that is what
	// closes it again.
	if e.help != nil && e.help.Visible() && e.help.HandleEvent(ctx, ev) {
		return
	}
	// The context browser covers the screen: it takes keys and mouse first.
	if e.ctxpane != nil && e.ctxpane.Visible() && e.ctxpane.HandleEvent(ctx, ev) {
		return
	}
	// So does the watch browser: while it is up, nothing underneath reacts.
	if e.watches != nil && e.watches.Visible() && e.watches.HandleEvent(ctx, ev) {
		return
	}
	// And the usage browser: it owns keys and mouse while it covers the screen.
	if e.usagepane != nil && e.usagepane.Visible() && e.usagepane.HandleEvent(ctx, ev) {
		return
	}
	if mouse, ok := ev.(xui.MouseEvent); ok {
		// A modal ask owns the mouse the way it owns the keyboard: the click
		// either lands on an option or dies, it never reaches the sidebar.
		if e.overlays.HandleAskMouse(ctx, mouse) {
			return
		}
		handled, err := e.sidebar.HandleGlobalMouse(ctx, mouse, e.terminalWidth)
		if err != nil {
			e.toast.Show("Cannot save sidebar width: "+err.Error(), toast.ToastError, 4*time.Second)
		}
		if handled {
			return
		}
		if e.handleFooterClick(ctx, mouse) {
			return
		}
	}
	if ke, ok := ev.(xui.KeyEvent); ok {
		if e.overlays.HandlePermissionKey(ctx, ke) {
			return
		}
		if e.overlays.HandleContinueKey(ctx, ke) {
			return
		}
		if e.overlays.HandleQuestionKey(ctx, ke) {
			return
		}
		// Every rebindable global chord resolves through the keys table:
		// the editor never compares a chord itself, so a config override
		// changes the behavior with the same table lookup that changes
		// the footers and the help screen.
		if cmd, ok := keys.GlobalCommand(ke); ok && e.runGlobalCommand(ctx, cmd) {
			return
		}
		if e.sidebar.HandleScrollKey(ctx, ke) {
			return
		}
		// The plan pane owns plain keys only while the editor root is the real
		// focused widget (the alt+P contract). With real focus elsewhere —
		// the composer after a click — keys it passes up must fall through,
		// so a stale planFocus is released before it can eat them.
		if e.App != nil {
			if focused := e.App.Focused(); focused != nil && focused != e {
				e.sidebar.ReleasePlanFocus()
			}
		}
		planWasFocused := e.sidebar.PlanFocused()
		handled, err := e.sidebar.HandlePlanKey(ctx, ke)
		if planWasFocused && !e.sidebar.PlanFocused() {
			// Restore actual focus, not only Sidebar's logical flag. If this key
			// was a rune and was not consumed, composer.Handle below inserts it.
			e.Focus(&e.composer.Chat)
		}
		if err != nil {
			e.toast.Show("Cannot set step model: "+err.Error(), toast.ToastError, 4*time.Second)
			return
		}
		if handled {
			return
		}
	}
	e.composer.Handle(ctx, ev)
}

// handleFooterClick folds or unfolds a live watch's transcript rows when a
// left click lands on the footer's watch indicator: a label folds that
// watch, the glyph and the count fold them all. It runs after the modal
// ask check, so an open ask keeps the mouse, and reports whether it took
// the click. A watch with no rows in view — a trimmed transcript — says so
// in a toast instead of swallowing the click silently.
func (e *Editor) handleFooterClick(ctx *components.EventContext, m xui.MouseEvent) bool {
	if m.Action != xui.MousePress || m.Button != xui.MouseLeft || e.footerY < 0 || m.Y != e.footerY {
		return false
	}
	live, ok := e.footer.WatchesAt(m.X)
	if !ok {
		return false
	}
	refs := make([]transcript.WatchRef, 0, len(live))
	ids := make([]string, 0, len(live))
	for _, w := range live {
		refs = append(refs, transcript.WatchRef{ID: w.ID, Label: w.Label})
		ids = append(ids, w.ID)
	}
	if !e.transcript.ToggleWatches(refs) {
		e.toast.Show("No transcript rows for "+strings.Join(ids, ", "), toast.ToastWarning, 3*time.Second)
	}
	ctx.ConsumeAndRedraw()
	return true
}

// runGlobalCommand executes one table-dispatched global chord. It reports
// false when the command does not apply right now — no plan to approve, no
// details to flip — so the key falls through the ladder like any unclaimed
// event instead of going dead. The palette also reports false: it lives in
// the composer's flow, and composer.Handle matches the same table entry.
func (e *Editor) runGlobalCommand(ctx *components.EventContext, cmd keys.Command) bool {
	switch cmd {
	case keys.CmdHelp:
		e.ShowHelp()
	case keys.CmdSettings:
		e.ShowSettings()
	case keys.CmdPlanEditor:
		e.ShowPlan()
	case keys.CmdPlanFocus:
		if e.sidebar.FocusPlan() {
			// ChatInput normally receives keys before the editor root. Move real
			// application focus here so the sidebar can see m/arrows/Escape.
			e.FocusEditor()
		}
	case keys.CmdSidebarToggle:
		handled, err := e.sidebar.ToggleVisibility(ctx)
		if err != nil {
			e.toast.Show("Cannot save sidebar visibility: "+err.Error(), toast.ToastError, 4*time.Second)
		}
		return handled
	case keys.CmdPlanApprove:
		handled, err := e.sidebar.TogglePlanApproved(ctx)
		if !handled {
			return false
		}
		if err != nil {
			e.toast.Show("Cannot approve plan: "+err.Error(), toast.ToastError, 4*time.Second)
		} else if e.sidebar.Approved() {
			e.toast.Show("Plan approved", toast.ToastSuccess, 3*time.Second)
		} else {
			e.toast.Show("Plan stopped", toast.ToastWarning, 3*time.Second)
		}
		return true
	case keys.CmdPlanDetails:
		return e.sidebar.TogglePlanDetails(ctx)
	case keys.CmdWatches:
		e.ShowWatches()
	case keys.CmdCopyLast:
		return e.transcript.CopySelectionOrLast(ctx)
	case keys.CmdVerbose:
		if e.transcript.ToggleVerbose() {
			e.toast.Show("Verbose transcript: every turn in full", toast.ToastSuccess, 2*time.Second)
		} else {
			e.toast.Show("Condensed transcript: older turns fold to summaries", toast.ToastSuccess, 2*time.Second)
		}
	default:
		return false
	}
	ctx.ConsumeAndRedraw()
	return true
}

// Draw renders the editor surface for the given draw context.
func (e *Editor) Draw(ctx components.DrawContext) components.Surface {
	e.drainBus()

	if e.footer != nil {
		e.footer.AdvanceTick()
		if e.footer.Activity().ShowSpinner() {
			ctx.WakeIn(spinnerInterval)
		} else if e.footer.WatchesLive() {
			// The watch glyph breathes on the wall clock, in the footer and
			// on the feed's start row, so idle frames keep coming while one
			// runs.
			ctx.WakeIn(watchPulseInterval)
		}
	}
	if e.transcript.AdvanceEdgeScroll() {
		// Drag selection held at a viewport edge keeps scrolling on ticks.
		ctx.WakeIn(edgeScrollInterval)
	}
	if e.toast.Visible() {
		// The frame that lands after Until removes the toast.
		ctx.WakeAt(e.toast.Until)
	}

	maxSize := ctx.Max
	e.terminalWidth = maxSize.Width
	if e.ctrl != nil {
		activity := e.footer.Activity().Label(e.transcript.Snapshot())
		if activity == "" {
			activity = "idle"
		}
		e.sidebar.SetRuntime(sidebar.Runtime{
			Model:        e.ctrl.EffectiveModelName(),
			SessionModel: e.ctrl.ModelName(),
			Mode:         string(e.ctrl.Mode()),
			Activity:     activity,
			MCP:          e.ctrl.MCPStatuses(),
			LSP:          e.ctrl.LSPStatuses(),
		})
	}
	root := components.Surface{Size: maxSize, Widget: e}

	// The status sidebar takes right-hand columns; everything else wraps
	// inside contentW. ReserveWidth is 0 while hidden or on narrow terminals.
	sideW := e.sidebar.ReserveWidth(maxSize.Width)
	contentW := maxSize.Width - sideW

	footerH := slot.FooterRows
	preferred, minH := e.composer.PreferredHeight(contentW, ctx.Method), e.composer.MinHeight()
	// The overlay is measured at the width it is drawn at. Measuring at the
	// full terminal width under-counts its wrapped rows, and the ask loses its
	// last options off the bottom whenever the sidebar takes columns.
	if askH, overlay := e.overlays.PreferredBottomHeight(contentW, ctx.Method); overlay {
		preferred, minH = askH, overlayFloorH
	}
	plan := slot.Arbitrate(maxSize.Height, preferred, minH)

	listSurf := e.transcript.Draw(ctx, contentW, plan.ListHeight)

	var chatSurf components.Surface
	if surf, ok := e.overlays.DrawBottom(ctx, contentW, plan.ChatHeight); ok {
		chatSurf = surf
		e.overlays.SetBottomOrigin(0, plan.ChatY)
	} else {
		chatSurf = e.composer.DrawChat(ctx, contentW, plan.ChatHeight)
	}
	footerSurf := e.footer.Draw(ctx, contentW)
	e.footerY = maxSize.Height - footerH

	root.Children = []components.SubSurface{
		{Origin: components.Point{X: 0, Y: 0}, Surface: listSurf, Z: components.ZList},
		{Origin: components.Point{X: 0, Y: plan.ChatY}, Surface: chatSurf, Z: components.ZChat},
		{Origin: components.Point{X: 0, Y: e.footerY}, Surface: footerSurf, Z: components.ZFooter},
	}
	if sideW > 0 {
		root.Children = append(root.Children, components.SubSurface{
			Origin:  components.Point{X: contentW, Y: 0},
			Surface: e.sidebar.Draw(ctx),
		})
	}
	if e.ctxpane != nil && e.ctxpane.Visible() {
		root.Children = append(root.Children, components.SubSurface{
			Origin:  components.Point{X: 0, Y: 0},
			Surface: e.ctxpane.Draw(ctx.WithConstraints(components.Size{}, maxSize)),
			Z:       components.ZOverlay,
		})
	}
	if e.watches != nil && e.watches.Visible() {
		root.Children = append(root.Children, components.SubSurface{
			Origin:  components.Point{X: 0, Y: 0},
			Surface: e.watches.Draw(ctx.WithConstraints(components.Size{}, maxSize)),
			Z:       components.ZOverlay,
		})
	}
	if e.usagepane != nil && e.usagepane.Visible() {
		root.Children = append(root.Children, components.SubSurface{
			Origin:  components.Point{X: 0, Y: 0},
			Surface: e.usagepane.Draw(ctx.WithConstraints(components.Size{}, maxSize)),
			Z:       components.ZOverlay,
		})
	}
	if e.help != nil && e.help.Visible() {
		root.Children = append(root.Children, components.SubSurface{
			Origin:  components.Point{X: 0, Y: 0},
			Surface: e.help.Draw(ctx.WithConstraints(components.Size{}, maxSize)),
			Z:       components.ZOverlay,
		})
	}
	if e.settings != nil && e.settings.Visible() {
		root.Children = append(root.Children, components.SubSurface{
			Origin:  components.Point{X: 0, Y: 0},
			Surface: e.settings.Draw(ctx.WithConstraints(components.Size{}, maxSize)),
			Z:       components.ZOverlay,
		})
	}
	if e.planPane != nil && e.planPane.Visible() {
		root.Children = append(root.Children, components.SubSurface{
			Origin:  components.Point{X: 0, Y: 0},
			Surface: e.planPane.Draw(ctx.WithConstraints(components.Size{}, maxSize)),
			Z:       components.ZOverlay,
		})
	}
	if !e.overlays.Active() && !e.modalActive() {
		root.Children = append(root.Children, e.composer.PickerOverlays(ctx, plan.ChatY, contentW)...)
	}
	if !e.modalActive() {
		if pal, ok := e.composer.PaletteOverlay(ctx); ok {
			root.Children = append(root.Children, pal)
		}
	}
	if e.toast.Visible() {
		toastSurf := e.toast.Draw(ctx)
		root.Children = append(root.Children, components.SubSurface{
			Origin:  components.Point{X: 0, Y: 0},
			Surface: toastSurf,
			Z:       components.ZToast,
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

// DrainNow applies pending bus messages immediately (submit/cancel flush path).
func (e *Editor) DrainNow() {
	e.drainBus()
}

// RequestRefresh schedules an immediate frame (composer input change path).
func (e *Editor) RequestRefresh() {
	if e.vx != nil {
		e.vx.QueueRefresh()
	}
}

// FocusEditor moves keyboard focus to the editor root.
func (e *Editor) FocusEditor() {
	if e.App != nil {
		e.App.RequestFocus(e)
	}
}

// Focus moves keyboard focus to an inner widget. While a modal overlay owns
// the keyboard the request lands on the editor root instead, so composer
// widgets hidden behind an ask dialog never take focus.
func (e *Editor) Focus(w components.Widget) {
	if e.App == nil {
		return
	}
	if e.modalActive() {
		e.App.RequestFocus(e)
		return
	}
	if e.ctxpane != nil && e.ctxpane.Visible() {
		e.App.RequestFocus(e)
		return
	}
	if e.watches != nil && e.watches.Visible() {
		e.App.RequestFocus(e)
		return
	}
	if e.usagepane != nil && e.usagepane.Visible() {
		e.App.RequestFocus(e)
		return
	}
	if e.help != nil && e.help.Visible() {
		e.App.RequestFocus(e)
		return
	}
	if e.overlays.Active() {
		e.App.RequestFocus(e)
		return
	}
	e.App.RequestFocus(w)
}

// commandContext returns the Host-bearing context passed to command Run /
// palette builders. The Editor is the single Host adapter in production.
func (e *Editor) commandContext() commands.CommandContext {
	return commands.CommandContext{Host: e}
}

// Toast surfaces a transient message.
func (e *Editor) Toast(msg string, kind toast.ToastKind, d time.Duration) {
	e.toast.Show(msg, kind, d)
}

// PushSubmenu opens or nests a palette submenu.
func (e *Editor) PushSubmenu(title string, cmds []palette.PaletteCommand) {
	e.composer.PushPalette(title, cmds)
}

// ShowSessions lists recent sessions for this directory.
func (e *Editor) ShowSessions() {
	e.sessions.Show()
}

// ShowSettings opens the global harness settings modal.
func (e *Editor) ShowSettings() {
	if e.settings == nil {
		return
	}
	e.composer.HideCompleters()
	e.composer.HidePalette()
	if e.ctrl != nil {
		e.settings.SetAvailableTools(e.ctrl.ToolNames())
	}
	e.settings.Show()
	e.FocusEditor()
}

// ShowPlan opens the durable-plan viewer/editor modal. With the plan feature
// switched off it is inert — the entry points (/plan, palette, Ctrl+P) are
// hidden, and a stale one must not resurrect the modal.
func (e *Editor) ShowPlan() {
	if e.planPane == nil || !e.sidebar.PlanEnabled() {
		return
	}
	e.composer.HideCompleters()
	e.composer.HidePalette()
	e.planPane.Show()
	e.FocusEditor()
}

// applyPlanVisibility withdraws or restores the plan feature's entry points
// (/plan, the plan-editor palette row) and refreshes the palette through
// whichever path owns it. Called at startup and on every sidebar toggle.
func (e *Editor) applyPlanVisibility(enabled bool) {
	if e.commands == nil {
		return
	}
	e.commands.SetHidden("plan", !enabled)
	e.commands.SetHidden("plan-editor", !enabled)
	if e.composer == nil {
		return
	}
	if e.hookCmds != nil {
		e.hookCmds.Sync()
		return
	}
	e.composer.SetPaletteCommands(e.commands.BuildPalette(e.commandContext()))
}

// ShowUsage opens the full-screen usage browser (/usage).
func (e *Editor) ShowUsage() {
	if e.usagepane != nil {
		e.usagepane.Show()
		e.FocusEditor()
	}
}

// ShowContext opens the full-screen context browser (/context).
func (e *Editor) ShowContext() {
	if e.ctxpane != nil {
		e.ctxpane.Show()
		// app.dispatch delivers keys to the focused widget first; the chat
		// input would swallow arrows and letters before the editor sees them.
		e.FocusEditor()
	}
}

// ShowWatches opens the full-screen watch browser (/watches, Ctrl+W).
func (e *Editor) ShowWatches() {
	if e.watches != nil {
		e.watches.Show()
		// Same reason as ShowContext: the chat input would eat the arrows
		// and letters before the editor root ever saw them.
		e.FocusEditor()
	}
}

// ShowHelp opens the full-screen keyboard help (/help, F1).
func (e *Editor) ShowHelp() {
	if e.help != nil {
		e.help.Show()
		// Same reason as ShowContext: the chat input would eat the scroll
		// keys before the editor root ever saw them.
		e.FocusEditor()
	}
}

// ResumeSession loads a prior session by id.
func (e *Editor) ResumeSession(id string) {
	e.sessions.Resume(id)
}

// ClearSession starts a new empty session when the stream is idle.
func (e *Editor) ClearSession() {
	if e.submitter != nil && !e.submitter.CanSubmit() {
		e.toast.Show("Cannot clear while a reply or command is running", toast.ToastWarning, 3*time.Second)
		return
	}
	e.sessions.Clear()
}

// ModelNames returns a detached snapshot of configured and connected models.
func (e *Editor) ModelNames() []string {
	return append([]string(nil), e.modelNames...)
}

// ConnectProvider opens the secure provider picker and refreshes its catalog
// without blocking input or drawing.
func (e *Editor) ConnectProvider() {
	if e == nil || e.ctrl == nil || e.overlays == nil {
		return
	}
	authCtx, cancelAuth := context.WithCancel(context.Background())
	e.overlays.BeginConnect(
		e.ctrl.ProviderOptions(),
		func(req provider.ConnectRequest) {
			go func() {
				err := e.ctrl.ConnectProvider(req)
				req.APIKey = ""
				msg := controller.ProviderConnectResultMsg{ProviderID: req.ProviderID}
				if err != nil {
					msg.ErrText = err.Error()
				}
				e.Publish(msg)
			}()
		},
		func(item provider.Info, method provider.AuthMethod) {
			if method.Kind == provider.AuthOAuthDevice {
				go e.authorizeProviderDevice(authCtx, item.ID)
				return
			}
			go e.authorizeProviderBrowser(authCtx, item.ID)
		},
		cancelAuth,
	)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		err := e.ctrl.RefreshProviders(ctx)
		msg := controller.ProviderCatalogMsg{Providers: e.ctrl.ProviderOptions()}
		if err != nil {
			msg.ErrText = err.Error()
		}
		e.Publish(msg)
	}()
}

// authorizeProviderBrowser runs the loopback OAuth flow: open the browser, show
// the URL in case it did not open, then wait for the callback.
func (e *Editor) authorizeProviderBrowser(ctx context.Context, providerID string) {
	flow, err := e.ctrl.BeginProviderAuthorization(ctx, providerID)
	if err != nil {
		e.Publish(controller.ProviderAuthorizationMsg{ProviderID: providerID, ErrText: err.Error()})
		return
	}
	openErrText := ""
	if openErr := util.OpenBrowser(ctx, flow.AuthorizationURL); openErr != nil {
		openErrText = openErr.Error()
	}
	e.Publish(controller.ProviderAuthorizationMsg{
		ProviderID: providerID, AuthorizationURL: flow.AuthorizationURL, BrowserErrText: openErrText,
	})
	e.publishConnectResult(providerID, e.ctrl.CompleteProviderAuthorization(ctx, flow))
}

// authorizeProviderDevice runs the headless flow, for a machine with no browser
// to hand off to: the user carries the code to another device, so nothing here
// waits on a local browser or a loopback port.
func (e *Editor) authorizeProviderDevice(ctx context.Context, providerID string) {
	flow, err := e.ctrl.BeginProviderDeviceAuthorization(ctx, providerID)
	if err != nil {
		e.Publish(controller.ProviderDeviceCodeMsg{ProviderID: providerID, ErrText: err.Error()})
		return
	}
	e.Publish(controller.ProviderDeviceCodeMsg{
		ProviderID: providerID, VerificationURL: flow.VerificationURL, UserCode: flow.UserCode,
	})
	e.publishConnectResult(providerID, e.ctrl.CompleteProviderDeviceAuthorization(ctx, flow))
}

// publishConnectResult reports a finished sign-in. A model-catalog warning is
// not a failed sign-in: the credential is stored and the provider is usable.
func (e *Editor) publishConnectResult(providerID string, err error) {
	msg := controller.ProviderConnectResultMsg{ProviderID: providerID}
	if err != nil {
		if warning, ok := errors.AsType[*provider.ModelCatalogWarning](err); ok {
			msg.WarningText = warning.Error()
		} else {
			msg.ErrText = err.Error()
		}
	}
	e.Publish(msg)
}

func (e *Editor) refreshModelCommands() {
	if e == nil || e.ctrl == nil || e.commands == nil {
		return
	}
	e.modelNames = mergeModelNames(e.ctrl.ModelNames())
	// One dataset, one ordering: rank the shared list once, then fan it out to
	// every model picker (palette submenu, sidebar, settings pane).
	e.modelNames = e.commands.RankModels(e.modelNames)
	e.commands.RegisterModelCommand(e.modelNames)
	if e.sidebar != nil {
		e.sidebar.ConfigureModels(e.modelNames)
	}
	if e.settings != nil {
		e.settings.SetModelNames(e.modelNames)
	}
	if e.hookCmds != nil {
		e.hookCmds.Sync()
	} else if e.composer != nil {
		e.composer.SetPaletteCommands(e.commands.BuildPalette(e.commandContext()))
	}
}

// StartProviderModelRefresh updates account-specific model availability in the
// background. Input and drawing remain on the UI goroutine.
func (e *Editor) StartProviderModelRefresh() {
	if e == nil || e.ctrl == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		err := e.ctrl.RefreshSubscriptionModels(ctx)
		msg := controller.ProviderModelsUpdatedMsg{}
		if err != nil {
			msg.ErrText = err.Error()
		}
		e.Publish(msg)
	}()
}

func mergeModelNames(groups ...[]string) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, group := range groups {
		for _, name := range group {
			if name == "" {
				continue
			}
			if _, exists := seen[name]; exists {
				continue
			}
			seen[name] = struct{}{}
			result = append(result, name)
		}
	}
	return result
}

// SkillPath returns the skill discovery root.
func (e *Editor) SkillPath() string {
	return e.skillPath
}

// skillNames resolves the session's skill names on first use and caches them:
// every open of the plan-settings tab reuses the same slice instead of walking
// the skill tree again.
func (e *Editor) skillNames() []string {
	if !e.skillsResolved {
		list, _ := skills.LoadSkills(e.skillPath)
		for _, skill := range list {
			e.discoveredSkills = append(e.discoveredSkills, skill.Name)
		}
		e.skillsResolved = true
	}
	return e.discoveredSkills
}

func (e *Editor) AddSkill(name string) {
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

func (e *Editor) ApplyTheme(name string) {
	th, ok := components.ThemeByName(name)
	if !ok {
		return
	}
	e.theme = th
	e.composer.SetTheme(th)
	e.toast.Theme = th
	e.transcript.SetTheme(th)
	e.footer.SetTheme(th)
	e.sidebar.SetTheme(th)
	e.overlays.SetTheme(th)
	if e.settings != nil {
		e.settings.SetTheme(th)
	}
	e.toast.Show("Theme: "+name, toast.ToastSuccess, 2*time.Second)
	if e.vx != nil {
		e.vx.QueueRefresh()
	}
}

// SetModel switches the session model. Failures surface through the caller:
// slash dispatch toasts returned errors, and the palette path wraps this with
// its own toast — toasting here too would announce every failure twice.
func (e *Editor) SetModel(name string) error {
	if err := e.ctrl.SetModel(name); err != nil {
		return err
	}
	e.composer.SetModelLabel(name)
	e.toast.Show("Model: "+name, toast.ToastSuccess, 2*time.Second)
	if e.vx != nil {
		e.vx.QueueRefresh()
	}
	return nil
}

func (e *Editor) SetPermissions(bypass bool) {
	e.ctrl.SetAllowAll(bypass)
	kind := toast.ToastWarning
	msg := "Permissions: on (ask)"
	if bypass {
		kind = toast.ToastSuccess
		msg = "Permissions: off (allow all)"
	}
	e.toast.Show(msg, kind, 3*time.Second)
}

func (e *Editor) SetAgents(enabled bool) {
	e.ctrl.SetAgentsEnabled(enabled)
	msg := "Sub-agents: off"
	if enabled {
		msg = "Sub-agents: on"
	}
	e.toast.Show(msg, toast.ToastSuccess, 2*time.Second)
}

func (e *Editor) ReloadHooks() {
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

func (e *Editor) ListHooks() []palette.PaletteCommand {
	found, warns, err := e.ctrl.ListHooks()
	return commands.HookListEntries(found, warns, err)
}

// ListToasts renders the toast history for the palette's notifications page.
func (e *Editor) ListToasts() []palette.PaletteCommand {
	return commands.ToastListEntries(e.toast.History())
}

func (e *Editor) CopyLastMessage() {
	e.transcript.CopyBlock(e.transcript.LastCopyText())
}

// ExportSession writes the current transcript as markdown. An empty path
// defaults to cozyphi-<session>.md in the working directory; relative paths
// resolve against it.
func (e *Editor) ExportSession(path string) {
	if path == "" {
		path = "cozyphi-" + session.ShortID(e.sessionID()) + ".md"
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(e.cwd, path)
	}
	if err := os.WriteFile(path, []byte(session.Markdown(e.transcript.Snapshot().Messages)), 0o600); err != nil {
		e.toast.Show("Export failed: "+err.Error(), toast.ToastError, 4*time.Second)
		return
	}
	e.toast.Show("Exported "+path, toast.ToastSuccess, 3*time.Second)
	if e.vx != nil {
		e.vx.QueueRefresh()
	}
}

func (e *Editor) sessionID() string {
	if e.ctrl != nil {
		return e.ctrl.SessionID()
	}
	return "session"
}

// RunCompact summarizes the session history on demand (/compact). Refused
// while anything is in flight; outcomes arrive as transcript events and
// the footer "Compacting…" activity.
func (e *Editor) RunCompact() {
	if e.submitter != nil && !e.submitter.CanSubmit() {
		e.toast.Show("Cannot compact while a reply or command is running", toast.ToastWarning, 3*time.Second)
		return
	}
	if e.ctrl != nil {
		e.ctrl.Compact()
	}
}

// SubmitPrompt publishes a user prompt onto the bus.
func (e *Editor) SubmitPrompt(text string) {
	e.Publish(controller.SubmitMsg{Text: text})
}

const branchPollInterval = time.Second

// spinnerInterval is the footer spinner glyph rate while an activity is in
// flight; the app loop draws only on these wakes.
const spinnerInterval = time.Second / 15

// watchPulseInterval is the frame rate of the live-watch glyph's breathing
// while no activity spinner is up: ten frames a second reads as a smooth
// pulse and costs the idle loop little.
const watchPulseInterval = time.Second / 10

// edgeScrollInterval is the drag-selection auto-scroll rate while the
// pointer is held at a transcript viewport edge.
const edgeScrollInterval = time.Second / 20

// overlayFloorH is the smallest height the bottom overlay (the permission
// ask) may shrink to on short screens.
const overlayFloorH = 8

// ctrlCExitWindow is how long an armed Ctrl+C stays armed: a second press
// inside it exits, a later one only re-arms. It also times out the hint
// toast, so the toast is visible exactly while the exit is armed.
const ctrlCExitWindow = 2 * time.Second

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
