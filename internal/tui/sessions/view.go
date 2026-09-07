// Package sessions wires the session view and assembles domain panes.
package sessions

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/clipboard"
	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/app"
	"github.com/alvnukov/cozyphi/internal/components/palette"
	"github.com/alvnukov/cozyphi/internal/components/slot"
	"github.com/alvnukov/cozyphi/internal/components/toast"
	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/harnesssettings"
	"github.com/alvnukov/cozyphi/internal/history"
	"github.com/alvnukov/cozyphi/internal/llm/skills"
	"github.com/alvnukov/cozyphi/internal/mcp"
	"github.com/alvnukov/cozyphi/internal/notify"
	"github.com/alvnukov/cozyphi/internal/provider"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools/questiontool"
	"github.com/alvnukov/cozyphi/internal/tui/agentlist"
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
	"github.com/alvnukov/cozyphi/internal/tui/statuspane"
	"github.com/alvnukov/cozyphi/internal/tui/submit"
	"github.com/alvnukov/cozyphi/internal/tui/transcript"
	"github.com/alvnukov/cozyphi/internal/tui/usagepane"
	"github.com/alvnukov/cozyphi/internal/tui/watchpane"
	"github.com/alvnukov/cozyphi/internal/util"
	"github.com/alvnukov/cozyphi/internal/util/update"
	"github.com/alvnukov/cozyphi/internal/version"
	"github.com/alvnukov/cozyphi/internal/voice"
	"github.com/alvnukov/cozyphi/internal/watch"
)

// View is the TUI root widget: layout composition and the UI-goroutine
// message loop. Cross-component work goes through controller.Bus — producers Publish,
// Draw drains and Update applies. Agent lifecycle lives in controller.Controller;
// session→widget projection lives in TranscriptPane (Mapper/SubagentStore).
//
// Construction: cmd assembles App, controller.Bus, controller.Controller, CommandRegistry and passes
// them into NewView. View does not create controller.Controller or fetch the project singleton.
type View struct {
	vx    *xui.XUI
	App   *app.App
	theme components.Theme
	// bootTheme is the palette this View was built with. /theme replaces
	// theme above and persists nothing, so the pair is the only way to see
	// that the look of a session is a choice somebody made in it.
	bootTheme components.Theme
	bus       *controller.Bus
	cwd       string

	lifetime   viewLifetime
	bashRunner *submit.BashRunner
	// composerOrigin is the last painted origin, used to translate local clicks.
	composerOrigin components.Point

	transcript *transcript.TranscriptPane
	composer   *composer.ComposerPane
	footer     *footer.FooterChrome
	// footerY is the screen row the footer took on the last frame, -1 before
	// the first: a click on that row is read back into the footer's watch
	// indicator.
	footerY int
	// family is the agent panel this view draws: a parent session owns its
	// own, a child draws the one its parent owns. panelY and panelH are the
	// band's last painted placement, so a click can be read back into it.
	family     *Family
	panelY     int
	panelH     int
	childJobID string
	childTitle string
	sidebar    *sidebar.Sidebar
	overlays   *overlays.Overlays
	toast      toast.Toast
	ctxpane    *ctxpane.Pane
	watches    *watchpane.Pane
	agents     *agentlist.Pane
	usagepane  *usagepane.Pane
	status     *statuspane.Pane
	// quotaFetchedAt stamps the last subscription fetch this view asked for,
	// so Draw paces the next one instead of asking on every frame.
	quotaFetchedAt time.Time

	statusStore   settings.Store
	statusHistory *controller.StatusHistory
	help          *helppane.Pane
	settings      *settings.Pane
	// settingsDetach unregisters this view from the process-wide settings
	// manager; nil when the store is not shared.
	settingsDetach func()
	planPane       *planedit.Pane

	ctrl *controller.Controller

	commands   *commands.CommandRegistry
	modelNames []string
	skillPath  string
	// discoveredSkills caches the session's skill names; the discovery root
	// never changes mid-session, so the plan-settings tab reads names, not
	// directories.
	discoveredSkills []string
	skillsResolved   bool

	sessions   *commands.SessionCommands
	navigation *sessionNavigation
	hookCmds   *commands.HookCommands
	submitter  *submit.Submitter

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

	// voiceSession owns the microphone; nil until ConfigureVoice runs, which
	// is the case in every test that does not ask for voice.
	voiceSession *voice.Session
	// voiceGate is the process-wide microphone admission this View shares
	// with every other retained one. It is kept only to be observed: the
	// capture it wraps was handed to the session at ConfigureVoice.
	voiceGate *voice.CaptureGate
	// voiceEnv is what /voice devices probes with — the same lookup the
	// session resolved its capture command from.
	voiceEnv voice.ResolveEnv
	// voiceLifetime bounds every recording and transcription; CloseVoice
	// cancels it so no capture process outlives the TUI.
	voiceLifetime context.Context
	voiceCancel   context.CancelFunc
	// voiceConfig is the configuration the session runs with. Installing a
	// model rewrites voice.stt.model in it and re-resolves from there, so a
	// model that arrives mid-session needs no restart.
	voiceConfig voice.Config
	// voicePersist pins the selected model in config.yaml so it survives a
	// restart; nil when cmd wired no settings manager.
	voicePersist func(name string) error
	// voiceDownload is the speech-model download in flight, nil when none is.
	voiceDownload *voiceDownload
	// voiceOfferActivity is the footer activity from before the download
	// offer opened: every ask hands the footer back as "Calling tools…", and
	// this offer is not a tool call.
	voiceOfferActivity controller.Activity
}

// NewView builds inactive TUI panes. The shell must call SetActive(true) to select it.
// application, bus, and ctrl must be non-nil. registry may be nil (builtins used).
// hist may be nil — the composer then works without prompt history.
func NewView(
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
) *View {
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
	e := &View{
		vx:         vx,
		App:        application,
		theme:      theme,
		bootTheme:  theme,
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
	e.panelY = -1
	e.family = newFamily(e, time.Now)
	e.bindFamily()
	e.lifetime.ctx, e.lifetime.cancel = context.WithCancel(context.Background())
	if len(settingsStores) > 0 && settingsStores[0] != nil {
		e.settings = settings.New(theme, settingsStores[0], func() { e.composer.FocusChat() })
		e.settings.SetSkills(e.skillNames())
		if e.ctrl != nil {
			e.settings.SetTypeInUse(e.ctrl.PlanUsesType)
			e.settings.SetAvailableTools(e.ctrl.ToolNames())
			e.applySettings(settingsStores[0].Snapshot())
			if shared, ok := settingsStores[0].(sharedSettings); ok {
				// One manager per process: a commit from any session reaches
				// this one, and this session's plan takes part in migrations.
				e.settingsDetach = shared.Attach(e.ctrl, e.applySettings)
			} else {
				e.settings.SetOnApplied(e.applySettings)
			}
		}
	}
	if ctrl != nil {
		e.planPane = planedit.New(theme, planStore{ctrl: ctrl, commands: registry}, func() { e.composer.FocusChat() })
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
			ModelLabel:   e.ctrl.ModelLabel(),
			SessionModel: e.ctrl.ModelRef(),
			Mode:         string(e.ctrl.Mode()),
			MCP:          e.ctrl.MCPStatuses(),
			LSP:          e.ctrl.LSPStatuses(),
		})
		// The status tab opens on a real subscription rather than a placeholder:
		// the fetch is coalesced, so asking at startup costs one request.
		e.refreshQuota()
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
		// Session-only context steppers: chip clicks hand the next value to the
		// controller and the view pushes the authoritative effective value back —
		// nothing is persisted, and a fresh session starts from the General
		// values (window-derived compact default / unlimited agents) again.
		e.sidebar.ConfigureContext(
			e.ctrl.ReminderThreshold(), e.ctrl.AgentWindowLimit(),
			func(tokens int) {
				e.ctrl.SetSessionReminderThreshold(tokens)
				e.sidebar.SetReminderThreshold(e.ctrl.ReminderThreshold())
			},
			func(tokens int) {
				e.ctrl.SetSessionAgentContext(tokens)
				e.sidebar.SetAgentsContext(e.ctrl.AgentWindowLimit())
			},
		)
		e.sidebar.ConfigureModels(e.commands.RankModels(modelNames))
		e.sidebar.ConfigureModelEfforts(e.ModelEfforts)
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
	// The footer label shows the effort too; name-comparing consumers keep
	// reading EffectiveModelName.
	e.footer.SetModelSource(func() string { return e.ctrl.ModelLabel() })
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
		overlayComposer{view: e},
		e.focusOverlay,
		e.restoreOverlayFocus,
	)
	// An ask answered here belongs to whichever session the family routed it
	// from; the family is read late, because a child joins one after it is
	// built.
	e.overlays.SetAskResolved(func(owner string) {
		if e.family != nil {
			e.family.askResolved(owner)
		}
	})
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
	e.bindBashLifetime(submit.NewBashRunner(
		e.transcript,
		e.composer,
		func(msg string, kind toast.ToastKind, d time.Duration) {
			e.toast.Show(msg, kind, d)
		},
		e.Publish,
		e.cwd,
	))
	e.submitter = submit.NewSubmitter(
		e.ctrl,
		e.commands,
		e.transcript,
		e.footer.Activity(),
		e.composer,
		e.bashRunner,
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

	// The agent browser lists this session's whole sub-agent history and opens
	// or stops one through the family — the same two paths the band under the
	// composer takes, so a row cannot mean one thing here and another there.
	// The family is read through the field on every call, never captured: a
	// child view is adopted after it is built, and from then on the browser
	// must list its parent's children rather than its own empty family.
	e.agents = agentlist.New(
		theme,
		func() []agentlist.Agent { return e.family.Agents() },
		func(id string) { e.family.OpenAgent(id) },
		func(id string) error {
			if err := e.family.StopAgent(id); err != nil {
				e.toast.Show("Cannot stop sub-agent: "+err.Error(), toast.ToastError, 4*time.Second)
				return err
			}
			e.toast.Show("Sub-agent stopped", toast.ToastSuccess, 3*time.Second)
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
		e.refreshQuota,
		func(target *provider.QuotaResetTarget) { e.ctrl.ResetQuota(e.lifetime.ctx, target) },
		func() { e.composer.FocusChat() },
	)

	e.status = statuspane.New(theme, e.ctrl.SessionStats, e.refreshQuota, func() { e.composer.FocusChat() })
	if len(settingsStores) > 0 && settingsStores[0] != nil {
		e.statusStore = settingsStores[0]
	}
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
	e.configureEditing()
	e.composer.Chat.OnModelPick = func(at components.Point) {
		e.OpenModelPicker()
		e.composer.AnchorPalette(components.Point{X: e.composerOrigin.X + at.X, Y: e.composerOrigin.Y + at.Y})
	}
	e.composer.Chat.OnEffortPick = func(at components.Point) {
		e.openCurrentEffortPicker()
		e.composer.AnchorPalette(components.Point{X: e.composerOrigin.X + at.X, Y: e.composerOrigin.Y + at.Y})
	}
	e.syncModelControls()
	e.publishUIStatus()
	return e
}

// sharedSettings is the optional store seam a process-wide settings manager
// implements: sessions attach for plan migration and snapshot broadcast.
type sharedSettings interface {
	Attach(harnesssettings.PlanMigrator, func(harnesssettings.Snapshot)) func()
}

// applySettings puts a committed settings snapshot into effect without a
// restart: notification mode and sound reach the live notifier, compaction
// thresholds go to the controller, and agent model pins reload from the
// project config so the next spawn resolves them.
func (e *View) applySettings(snap harnesssettings.Snapshot) {
	if e.notifier != nil {
		e.notifier.Reconfigure(snap.Notifications.Mode, snap.Notifications.Sound)
		e.publishUIStatus()
	}
	e.ctrl.SetTasksAccess(snap.Tasks)
	// The General values become this session's fallbacks: a live apply must
	// not clobber an active session override, so the controller keeps the
	// override on top and the sidebar shows the effective pair.
	e.ctrl.SetReminderThreshold(snap.Compaction.ReminderTokens)
	e.ctrl.SetAgentContextLimit(snap.AgentContextLimit)
	e.sidebar.SetReminderThreshold(e.ctrl.ReminderThreshold())
	e.sidebar.SetAgentsContext(e.ctrl.AgentWindowLimit())
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
	// Observe hands over what the notifier would do, for the harness view to
	// report. It sends nothing and reconfigures nothing.
	Observe() diag.NotifierFacts
}

// SetAttentionNotifier wires OS notifications for agent state changes. The
// terminal's focus reports reach the notifier through Handle, so the
// unfocused mode only pings when the user is actually elsewhere.
func (e *View) SetAttentionNotifier(n attentionNotifier) {
	e.notifier = n
	defer e.publishUIStatus()
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
func (e *View) watchRunning() bool {
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
func (e *View) Publish(m controller.Msg) {
	if e.bus == nil {
		return
	}
	e.bus.Publish(m)
}

// Update applies one message on the UI goroutine.
func (e *View) Update(m controller.Msg) {
	e.recordStatus(m)
	switch msg := m.(type) {
	case controller.StatusHistoryMsg:
		if e.status.Visible() && e.statusHistory.Accept(msg) {
			e.ApplyStatusHistory(statusHistorySnapshot(msg))
		}
	case controller.SubmitMsg:
		if !e.lifetime.closed {
			e.submitter.Submit(msg.Text, msg.Media...)
		}
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
	case controller.VoiceStateMsg:
		e.applyVoiceState(msg)
		e.publishUIStatus()
	case controller.VoiceResultMsg:
		// The mode stays on after a segment lands, so the footer is left
		// alone: the session's own state events own it.
		e.composer.ApplyVoiceResult(msg)
	case controller.VoiceErrorMsg:
		e.composer.ApplyVoiceError(msg)
		e.toast.Show(voiceToastText(msg.Text, msg.Hint), toast.ToastError, 6*time.Second)
	case controller.VoiceNoticeMsg:
		e.toast.Show(voiceToastText(msg.Text, ""), toast.ToastWarning, 4*time.Second)
	case controller.VoiceOfferReplyMsg:
		e.applyVoiceOfferReply(msg)
	case controller.VoiceInstallProgressMsg:
		e.applyVoiceInstallProgress(msg)
	case controller.VoiceInstallDoneMsg:
		e.applyVoiceInstallDone(msg)
	case controller.PermissionAskMsg:
		// The tool name is the context the user needs at a glance.
		e.showAsk(m, msg.Request.Tool)
	case controller.ContinueAskMsg:
		e.showAsk(m, fmt.Sprintf("continue for %d more rounds?", msg.MaxRounds))
	case controller.QuestionAskMsg:
		e.showAsk(m, questionDetail(msg.Questions))
	case controller.PermissionDismissMsg, controller.ContinueDismissMsg, controller.QuestionDismissMsg:
		e.withdrawAsk(m)
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
		e.publishUIStatus()
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
		e.ctrl.InvalidateQuota()
		if e.usagepane != nil {
			e.usagepane.InvalidateReset()
		}
		// A new credential can mean a new plan: show nothing until the fetch
		// for it lands, never the previous account's numbers.
		if e.sidebar != nil {
			e.sidebar.ClearQuota()
		}
		e.refreshQuota()
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
		if !e.ctrl.AcceptQuota(msg) {
			break
		}
		if e.status != nil {
			e.status.ApplyQuota(msg, e.ctrl.SessionStats().ProviderID)
		}
		// The fetch the pane started lands here; the pane decides what to
		// render, including the fetch-for-a-closed-pane case.
		if e.usagepane != nil {
			e.usagepane.Apply(msg)
		}
		if e.sidebar != nil {
			quota := sidebar.Quota{Loaded: true, Unsupported: msg.Unsupported, Snapshot: msg.Snapshot}
			if msg.Err != nil {
				quota.Err = msg.Err.Error()
			}
			e.sidebar.SetQuota(quota)
		}
	case controller.UsageResetMsg:
		if e.usagepane != nil {
			e.usagepane.ApplyReset(msg)
		}
		if !msg.InFlight {
			// Reconcile even an ambiguous outcome with a read, never a retry.
			// Starting reset invalidated old fetches, so this cannot be coalesced away.
			e.refreshQuota()
		}
	case controller.SetActivityMsg, controller.ClearIfActivityMsg, controller.UpdateAvailableMsg:
		e.footer.Apply(m)
	case controller.RunEndedMsg:
		e.footer.Apply(m)
		// The turn that just ended spent quota; refresh what the status tab
		// shows right away instead of waiting out the refresh interval.
		e.refreshQuota()
		// A live watch wakes the session by itself, so this turn's end is
		// not a wait for input: the ping waits for the last watch to go. A
		// sub-agent finishing is not a wait for input either — the parent's
		// own turn end is what the user is waiting on, and it keeps its ping.
		if e.notifier != nil && !e.watchRunning() && !e.isChild() {
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
	case controller.JobProgressMsg, controller.ChildOutcomeMsg:
		// Applied in drainBus so we can skip Sync when nothing visible changed.
	case controller.RedrawMsg:
		// no state change; drain already requested redraw
	}
}

// refreshQuota asks the controller for fresh subscription numbers and stamps
// when it asked, so Draw can pace the next ask. The controller coalesces a
// fetch already in flight, so an eager caller costs nothing.
func (e *View) refreshQuota() {
	if e == nil || e.ctrl == nil {
		return
	}
	e.quotaFetchedAt = time.Now()
	e.ctrl.FetchQuota(e.lifetime.ctx)
}

func (e *View) drainBus() {
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
			e.recordStatus(msg)
			e.transcript.ApplySession(msg.Event)
			if data, ok := msg.Event.(session.ToolData); ok && data.Run.Name == "session" &&
				data.Run.Status == session.ToolDone && data.Run.Error == "" {
				e.toast.Show("Session named: "+data.Run.Detail, toast.ToastSuccess, 3*time.Second)
			}
		case controller.JobProgressMsg:
			if e.transcript.ApplyJobProgress(msg.Progress) {
				agentEvent = true
			}
		case controller.ChildOutcomeMsg:
			if e.transcript.ApplyChildOutcome(msg.Outcome) {
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
func (e *View) modalActive() bool {
	return e.status.Visible() || (e.settings != nil && e.settings.Visible()) ||
		(e.planPane != nil && e.planPane.Visible())
}

// AcceptInterrupt claims Ctrl+C as an interrupt so the chord stops work
// instead of killing the session. The press cancels the innermost thing in
// flight — a modal ask, then a shell command or agent run, then an unsent
// draft. With nothing left to stop it arms the exit and says so; the next
// Ctrl+C within ctrlCExitWindow returns false and the app quits.
//
// A sub-agent's screen never arms that exit: the chord there means "stop this
// agent", and a second press repeats the interrupt rather than taking down a
// cozyphi the user only opened a child of. With nothing running it says where
// the way out is instead.
func (e *View) AcceptInterrupt() bool {
	if e.interruptWork() {
		e.lastCtrlC = time.Time{}
		return true
	}
	if e.childJobID != "" {
		e.lastCtrlC = time.Time{}
		e.toast.Show("Nothing running · Esc returns to main", toast.ToastWarning, ctrlCExitWindow)
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

// RefuseExit withdraws an exit this view armed because the shell found work in
// another session. The armed toast is replaced so the screen does not promise
// an exit that will not happen, and the next Ctrl+C arms again instead of quitting.
func (e *View) RefuseExit(reason string) {
	e.lastCtrlC = time.Time{}
	e.toast.Clear()
	e.toast.Show(reason, toast.ToastWarning, 4*time.Second)
}

// interruptWork cancels one layer of in-flight work and reports whether it
// found any. Layers unwind one press at a time, the way Escape does: an ask
// is declined before the run behind it is cancelled, and the draft is cleared
// only once the session is idle.
func (e *View) interruptWork() bool {
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

func (e *View) Handle(ctx *components.EventContext, ev xui.Event) {
	// Focus reports are observed, not consumed: unfocused notifications gate
	// on them even while a modal owns the keyboard. The composer still gets
	// the event below.
	if fe, ok := ev.(xui.FocusEvent); ok && e.notifier != nil {
		e.notifier.SetFocused(fe.Focused)
		e.publishUIStatus()
	}
	if e.status.Visible() && e.status.HandleEvent(ctx, ev) {
		return
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
	// And the agent browser, for the same reason.
	if e.agents != nil && e.agents.Visible() && e.agents.HandleEvent(ctx, ev) {
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
		if e.handlePanelMouse(ctx, mouse) {
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
		// While the band holds the keyboard it owns every plain key: its own
		// motions, Enter, x and Esc. The global chords above it still work,
		// and Ctrl+C never reaches here at all — the application claims it.
		if e.family.Focused() && e.family.HandleEvent(ctx, ke) {
			return
		}
		if e.sidebar.HandleScrollKey(ctx, ke) {
			return
		}
		// The plan pane owns plain keys only while no inner widget is the real
		// focused widget (the alt+P contract). With real focus elsewhere —
		// the composer after a click — keys it passes up must fall through,
		// so a stale planFocus is released before it can eat them. Focus on
		// this view or on the application root (a click on a non-focusable
		// row under the shell) routes keys here unclaimed, so it keeps the plan.
		if e.App != nil {
			if focused := e.App.Focused(); focused != nil && focused != e && focused != e.App.Root() {
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
func (e *View) handleFooterClick(ctx *components.EventContext, m xui.MouseEvent) bool {
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
func (e *View) runGlobalCommand(ctx *components.EventContext, cmd keys.Command) bool {
	switch cmd {
	case keys.CmdHelp:
		e.ShowHelp()
	case keys.CmdSettings:
		e.ShowSettings()
	case keys.CmdEffort:
		e.openCurrentEffortPicker()
	case keys.CmdKeymap:
		if err := e.cycleEditingMode(); err != nil {
			e.Toast(err.Error(), toast.ToastError, 6*time.Second)
		}
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
func (e *View) Draw(ctx components.DrawContext) components.Surface {
	e.drainBus()
	e.syncModelControls()

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
	if e.ctrl != nil && e.sidebar.Visible() && e.sidebar.QuotaPolls() {
		// A subscription ages on the wall clock: while the panel is up, the
		// block refetches once a minute, and the same wake re-renders the
		// relative "resets in …" text.
		if time.Since(e.quotaFetchedAt) >= quotaRefreshInterval {
			e.refreshQuota()
		}
		ctx.WakeAt(e.quotaFetchedAt.Add(quotaRefreshInterval))
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
			ModelLabel:   e.ctrl.ModelLabel(),
			SessionModel: e.ctrl.ModelRef(),
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
	// The band is painted first because that is what measures it: the panel
	// builds its rows and asks for the frames its windows need inside Draw,
	// so a frame it is denied is a deadline it never meets. An empty band
	// draws zero rows and costs the composer nothing.
	panelSurf := e.family.Draw(ctx, contentW)
	e.panelH = panelSurf.Size.Height
	preferred, minH := e.composer.PreferredHeight(contentW, ctx.Method), e.composer.MinHeight()
	// The overlay is measured at the width it is drawn at. Measuring at the
	// full terminal width under-counts its wrapped rows, and the ask loses its
	// last options off the bottom whenever the sidebar takes columns.
	if askH, overlay := e.overlays.PreferredBottomHeight(contentW, ctx.Method); overlay {
		preferred, minH = askH, overlayFloorH
	}
	plan := slot.Arbitrate(maxSize.Height-e.panelH, preferred, minH)

	listSurf := e.transcript.Draw(ctx, contentW, plan.ListHeight)
	if plan.ListHeight > 0 {
		e.acknowledgeViewed()
	}

	var chatSurf components.Surface
	if surf, ok := e.overlays.DrawBottom(ctx, contentW, plan.ChatHeight); ok {
		chatSurf = surf
		e.overlays.SetBottomOrigin(0, plan.ChatY)
	} else {
		e.composerOrigin = components.Point{X: 0, Y: plan.ChatY}
		chatSurf = e.composer.DrawChat(ctx, contentW, plan.ChatHeight)
	}
	footerSurf := e.footer.Draw(ctx, contentW)
	e.footerY = maxSize.Height - footerH
	e.panelY = e.footerY - e.panelH

	root.Children = []components.SubSurface{
		{Origin: components.Point{X: 0, Y: 0}, Surface: listSurf, Z: components.ZList},
		{Origin: components.Point{X: 0, Y: plan.ChatY}, Surface: chatSurf, Z: components.ZChat},
		{Origin: components.Point{X: 0, Y: e.footerY}, Surface: footerSurf, Z: components.ZFooter},
	}
	if e.panelH > 0 {
		root.Children = append(root.Children, components.SubSurface{
			Origin:  components.Point{X: 0, Y: e.panelY},
			Surface: panelSurf,
			Z:       components.ZChat,
		})
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
	if e.agents != nil && e.agents.Visible() {
		root.Children = append(root.Children, components.SubSurface{
			Origin:  components.Point{X: 0, Y: 0},
			Surface: e.agents.Draw(ctx.WithConstraints(components.Size{}, maxSize)),
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
	if e.status.Visible() {
		root.Children = append(root.Children, components.SubSurface{
			Surface: e.status.Draw(ctx.WithConstraints(components.Size{}, maxSize)),
			Z:       components.ZOverlay,
		})
	}
	if !e.status.Visible() && e.settings != nil && e.settings.Visible() {
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

func (e *View) requestRedraw() {
	if e.App != nil {
		e.App.RequestRedraw()
	}
}

// RequestRedraw asks the app to repaint (safe to bind onto controller.RedrawRelay / controller.Bus).
func (e *View) RequestRedraw() {
	e.requestRedraw()
}

// DrainNow applies pending bus messages immediately (submit/cancel flush path).
func (e *View) DrainNow() {
	e.drainBus()
}

// RequestRefresh schedules an immediate frame (composer input change path).
func (e *View) RequestRefresh() {
	if e.vx != nil {
		e.vx.QueueRefresh()
	}
}

// SetClipboardReader replaces the composer's system clipboard image read so a
// pasted text event is not preempted by whatever image the host clipboard holds.
func (e *View) SetClipboardReader(read func() (clipboard.Image, bool, error)) {
	if e == nil || e.composer == nil {
		return
	}
	e.composer.SetClipboardReader(read)
}

// FocusEditor moves keyboard focus to the editor root.
func (e *View) FocusEditor() {
	e.requestFocus(e)
}

// Focus moves keyboard focus to an inner widget. While a modal overlay owns
// the keyboard the request lands on the editor root instead, so composer
// widgets hidden behind an ask dialog never take focus.
func (e *View) Focus(w components.Widget) {
	if e.modalActive() || (e.ctxpane != nil && e.ctxpane.Visible()) ||
		(e.watches != nil && e.watches.Visible()) || (e.agents != nil && e.agents.Visible()) ||
		(e.usagepane != nil && e.usagepane.Visible()) ||
		(e.help != nil && e.help.Visible()) || e.overlays.Active() {
		w = e
	}
	e.requestFocus(w)
}

// commandContext returns the Host-bearing context passed to command Run /
// palette builders. The View is the single Host adapter in production.
func (e *View) commandContext() commands.CommandContext {
	return commands.CommandContext{Host: e}
}

// Toast surfaces a transient message.
func (e *View) Toast(msg string, kind toast.ToastKind, d time.Duration) {
	e.toast.Show(msg, kind, d)
}

// PushSubmenu opens or nests a palette submenu.
func (e *View) PushSubmenu(title string, cmds []palette.PaletteCommand) {
	e.composer.PushPalette(title, cmds)
}

// ShowSessions lists recent sessions for this directory.
func (e *View) ShowSessions() {
	e.sessions.Show()
}

// ShowSettings opens the global harness settings modal.
func (e *View) ShowSettings() {
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
func (e *View) ShowPlan() {
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
func (e *View) applyPlanVisibility(enabled bool) {
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
func (e *View) ShowUsage() {
	if e.usagepane != nil {
		e.usagepane.Show()
		e.FocusEditor()
	}
}

// ShowContext opens the full-screen context browser (/context).
func (e *View) ShowContext() {
	if e.ctxpane != nil {
		e.ctxpane.Show()
		// app.dispatch delivers keys to the focused widget first; the chat
		// input would swallow arrows and letters before the editor sees them.
		e.FocusEditor()
	}
}

// ShowWatches opens the full-screen watch browser (/watches, Ctrl+W).
func (e *View) ShowWatches() {
	if e.watches != nil {
		e.watches.Show()
		// Same reason as ShowContext: the chat input would eat the arrows
		// and letters before the editor root ever saw them.
		e.FocusEditor()
	}
}

// ShowAgents opens the full-screen sub-agent browser (/agents): this session's
// children, the working ones on top and its whole history below.
func (e *View) ShowAgents() {
	if e.agents != nil {
		e.agents.Show()
		// Same reason as ShowContext: the chat input would eat the arrows
		// and letters before the editor root ever saw them.
		e.FocusEditor()
	}
}

// ShowHelp opens the full-screen keyboard help (/help, F1).
func (e *View) ShowHelp() {
	if e.help != nil {
		e.help.Show()
		// Same reason as ShowContext: the chat input would eat the scroll
		// keys before the editor root ever saw them.
		e.FocusEditor()
	}
}

// ResumeSession selects a retained session, or loads prior history into this view.
func (e *View) ResumeSession(id string) {
	if selected, err := e.selectRetainedSession(id); selected || err != nil {
		if err != nil {
			e.toast.Show(err.Error(), toast.ToastError, 4*time.Second)
		}
		return
	}
	e.sessions.Resume(id)
}

// ClearSession starts a new empty session when the stream is idle.
func (e *View) ClearSession() {
	if e.submitter != nil && !e.submitter.CanSubmit() {
		e.toast.Show("Cannot clear while a reply or command is running", toast.ToastWarning, 3*time.Second)
		return
	}
	e.sessions.Clear()
}

// ModelNames returns a detached snapshot of configured and connected models.
func (e *View) ModelNames() []string {
	return append([]string(nil), e.modelNames...)
}

// ConnectProvider opens the secure provider picker and refreshes its catalog
// without blocking input or drawing.
func (e *View) ConnectProvider() {
	if e == nil || e.ctrl == nil || e.overlays == nil {
		return
	}
	authCtx, cancelAuth := context.WithCancel(e.lifetime.ctx)
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
		ctx, cancel := context.WithTimeout(e.lifetime.ctx, 15*time.Second)
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
func (e *View) authorizeProviderBrowser(ctx context.Context, providerID string) {
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
func (e *View) authorizeProviderDevice(ctx context.Context, providerID string) {
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
func (e *View) publishConnectResult(providerID string, err error) {
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

func (e *View) refreshModelCommands() {
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
		e.settings.SetModelEfforts(e.ModelEfforts)
	}
	if e.hookCmds != nil {
		e.hookCmds.Sync()
	} else if e.composer != nil {
		e.composer.SetPaletteCommands(e.commands.BuildPalette(e.commandContext()))
	}
}

// StartProviderModelRefresh updates account-specific model availability in the
// background. Input and drawing remain on the UI goroutine.
func (e *View) StartProviderModelRefresh() {
	if e == nil || e.ctrl == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(e.lifetime.ctx, 6*time.Second)
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
func (e *View) SkillPath() string {
	return e.skillPath
}

// skillNames resolves the session's skill names on first use and caches them:
// every open of the plan-settings tab reuses the same slice instead of walking
// the skill tree again.
func (e *View) skillNames() []string {
	if !e.skillsResolved {
		list, _ := skills.LoadSkills(e.skillPath)
		for _, skill := range list {
			e.discoveredSkills = append(e.discoveredSkills, skill.Name)
		}
		e.skillsResolved = true
	}
	return e.discoveredSkills
}

func (e *View) AddSkill(name string) {
	e.composer.AddPendingSkill(name)
	if e.vx != nil {
		e.vx.QueueRefresh()
	}
}

// StartUpdateCheck queries GitHub for a newer release in the background and
// surfaces a footer hint when one is available. cacheDir is where the version
// check may store its cache (e.g. project global root); empty disables disk cache.
func (e *View) StartUpdateCheck(cacheDir string) {
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
func (e *View) StartBranchWatch() {
	if e.cwd == "" || e.lifetime.closed {
		return
	}
	e.lifetime.branchOnce.Do(func() {
		e.lifetime.branchStop = make(chan struct{})
		e.lifetime.branchDone = make(chan struct{})
		go func() {
			defer close(e.lifetime.branchDone)
			(&branchWatch{dir: e.cwd, interval: branchPollInterval}).run(e.lifetime.branchStop, func(label string) {
				e.Publish(controller.BranchLabelMsg{Text: label})
			})
		}()
	})
}

func (e *View) ApplyTheme(name string) {
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
	e.family.SetTheme(th)
	if e.settings != nil {
		e.settings.SetTheme(th)
	}
	e.toast.Show("Theme: "+name, toast.ToastSuccess, 2*time.Second)
	e.publishUIStatus()
	if e.vx != nil {
		e.vx.QueueRefresh()
	}
}

// SetModel switches the session model. Failures surface through the caller:
// slash dispatch toasts returned errors, and the palette path wraps this with
// its own toast — toasting here too would announce every failure twice.
func (e *View) SetModel(name string) error {
	if err := e.ctrl.SetModel(name); err != nil {
		return err
	}
	e.syncModelControls()
	e.toast.Show("Model: "+name, toast.ToastSuccess, 2*time.Second)
	if e.vx != nil {
		e.vx.QueueRefresh()
	}
	return nil
}

// SetModelEffort applies one pick from the shared model picker: the named
// model together with its effort ("default" arrives as ""). Errors come
// back to the caller — the palette wrapper or the slash dispatcher is the
// one toast surface for them.
func (e *View) SetModelEffort(name, effort string) error {
	if err := e.ctrl.SetModelEffort(name, effort); err != nil {
		return err
	}
	e.syncModelControls()
	e.toast.Show("Model: "+e.ctrl.ModelLabel(), toast.ToastSuccess, 2*time.Second)
	if e.vx != nil {
		e.vx.QueueRefresh()
	}
	return nil
}

// OpenModelPicker opens the shared two-step model picker as a palette
// submenu: the model list first, the effort step only for models that
// offer levels.
func (e *View) OpenModelPicker() {
	if e == nil || e.commands == nil {
		return
	}
	page := e.commands.ModelPickerPage(e.pickModelEffort, e.modelNames, e.ModelEfforts)
	e.PushSubmenu(page.SubmenuTitle, page.Submenu)
}

// OpenModelEffortPicker opens the effort step for one chosen model — the
// fast path of "/model <name>" when the model offers levels.
func (e *View) OpenModelEffortPicker(model string) {
	if e == nil || e.commands == nil {
		return
	}
	page := e.commands.ModelEffortPage(model, e.ModelEfforts(model), e.pickModelEffort)
	e.PushSubmenu(page.SubmenuTitle, page.Submenu)
}

// ModelEfforts lists the named model's reasoning effort levels; empty
// means the model has none and pickers skip the effort step.
func (e *View) ModelEfforts(model string) []string {
	if e == nil || e.ctrl == nil {
		return nil
	}
	return e.ctrl.ModelEfforts(model)
}

func (e *View) SetPermissions(bypass bool) {
	e.ctrl.SetAllowAll(bypass)
	kind := toast.ToastWarning
	msg := "Permissions: on (ask)"
	if bypass {
		kind = toast.ToastSuccess
		msg = "Permissions: off (allow all)"
	}
	e.toast.Show(msg, kind, 3*time.Second)
}

func (e *View) SetAgents(enabled bool) {
	e.ctrl.SetAgentsEnabled(enabled)
	msg := "Sub-agents: off"
	if enabled {
		msg = "Sub-agents: on"
	}
	e.toast.Show(msg, toast.ToastSuccess, 2*time.Second)
}

// MCPStatuses feeds the /mcp dialog: configured servers, live state.
func (e *View) MCPStatuses() []mcp.ServerStatus {
	return e.ctrl.MCPStatuses()
}

// ToggleMCPServer switches a configured MCP server on or off; the sidebar
// picks the new state up on its next draw.
func (e *View) ToggleMCPServer(name string, enabled bool) error {
	return e.ctrl.ToggleMCPServer(name, enabled)
}

func (e *View) ReloadHooks() {
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

func (e *View) ListHooks() []palette.PaletteCommand {
	found, warns, err := e.ctrl.ListHooks()
	return commands.HookListEntries(found, warns, err)
}

// ListToasts renders the toast history for the palette's notifications page.
func (e *View) ListToasts() []palette.PaletteCommand {
	return commands.ToastListEntries(e.toast.History())
}

func (e *View) CopyLastMessage() {
	e.transcript.CopyBlock(e.transcript.LastCopyText())
}

// ExportSession writes the current transcript as markdown. An empty path
// defaults to cozyphi-<session>.md in the working directory; relative paths
// resolve against it.
func (e *View) ExportSession(path string) {
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

func (e *View) sessionID() string {
	if e.ctrl != nil {
		return e.ctrl.SessionID()
	}
	return "session"
}

// RunCompact summarizes the session history on demand (/compact). Refused
// while anything is in flight; outcomes arrive as transcript events and
// the footer "Compacting…" activity.
func (e *View) RunCompact() {
	if e.submitter != nil && !e.submitter.CanSubmit() {
		e.toast.Show("Cannot compact while a reply or command is running", toast.ToastWarning, 3*time.Second)
		return
	}
	if e.ctrl != nil {
		e.ctrl.Compact()
	}
}

// SubmitPrompt publishes a user prompt onto the bus.
func (e *View) SubmitPrompt(text string) {
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

// quotaRefreshInterval is how often a visible sidebar refetches the provider
// subscription; the draw loop wakes on it, no goroutine ticks.
const quotaRefreshInterval = time.Minute

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

// bindFamily points this view's composer and footer at the agent panel it
// draws. A parent binds its own family; a child rebinds to its parent's, so
// both screens leave down into the same band and show the same hint.
func (e *View) bindFamily() {
	if e == nil || e.family == nil {
		return
	}
	if e.composer != nil {
		e.composer.SetLeaveDownFunc(e.family.enter)
		// A child's exhausted Escape leaves for the parent's screen; a parent
		// has nowhere to leave to, so its ladder ends where it always did.
		e.composer.SetLeaveOnEscapeFunc(func() bool { return e.family.backFrom(e.childJobID) })
	}
	if e.footer != nil {
		e.footer.SetPaneHint(e.family.footerHint)
	}
}

// handlePanelMouse routes a click or a wheel inside the band to the panel,
// with coordinates the panel can read as its own rows.
func (e *View) handlePanelMouse(ctx *components.EventContext, m xui.MouseEvent) bool {
	if e.panelH <= 0 || e.panelY < 0 || m.Y < e.panelY || m.Y >= e.panelY+e.panelH {
		return false
	}
	if m.X < 0 || m.X >= e.terminalWidth-e.sidebar.ReserveWidth(e.terminalWidth) {
		return false
	}
	m.Y -= e.panelY
	return e.family.HandleEvent(ctx, m)
}
