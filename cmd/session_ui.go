package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/app"
	"github.com/alvnukov/cozyphi/internal/components/palette"
	"github.com/alvnukov/cozyphi/internal/components/toast"
	"github.com/alvnukov/cozyphi/internal/harnesssettings"
	"github.com/alvnukov/cozyphi/internal/history"
	"github.com/alvnukov/cozyphi/internal/notify"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/commands"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/editor"
	"github.com/alvnukov/cozyphi/internal/tui/sessions"
	"github.com/alvnukov/cozyphi/internal/voice"
)

// newTUIView assembles controller-bound collaborators once. The shell selects
// this entire graph, never rebinding widgets or deferred callbacks to another
// Controller.
// settingsManager is shared by every View of the process; the View attaches
// to it and detaches on Close.
func newTUIView(
	application *app.App,
	vx *xui.XUI,
	th components.Theme,
	proj *project.Project,
	ctrl *controller.Controller,
	bus *controller.Bus,
	hist *history.Store,
	cwd string,
	captureGate *voice.CaptureGate,
	cmds *commands.CommandRegistry,
	settingsManager *harnesssettings.Manager,
) *sessions.View {
	cfg := ctrl.ModelConfig()
	view := sessions.NewView(
		application, bus, ctrl, cmds, vx, th, cwd, ctrl.ModelLabel(), cfg.SkillPath,
		cfg.ContextWindow, ctrl.ModelNames(), hist.NewCursor(), settingsManager,
	)
	view.ConfigureStatusDashboard(controller.NewStatusHistory(bus, cwd, session.HistoryStats))
	notifications := proj.Config().Notifications
	view.SetAttentionNotifier(notify.New(notifications.Mode, notify.WithSound(notifications.Sound)))
	view.ConfigureVoice(sessions.VoiceOptions{
		Config: proj.Config().Voice,
		Env: voice.ResolveEnv{
			GOOS:           runtime.GOOS,
			LookBin:        proj.Global().LookBin,
			ModelsDir:      proj.Global().VoiceModelsDir(),
			ExtraModelDirs: voice.DefaultModelDirs(),
		},
		// Pending transcriptions may overlap across Views even though microphone
		// admission is shared. Their segment files must never share a prefix.
		WAVPath:     filepath.Join(filepath.Dir(proj.Global().VoiceWAVFile()), "voice-"+rand.Text()+".wav"),
		CaptureGate: captureGate,
		PersistModel: func(name string) error {
			return settingsManager.SetVoiceModel(context.Background(), name)
		},
	})
	view.StartProviderModelRefresh()
	view.StartBranchWatch()
	return view
}

// registerSessionNavigation keeps process navigation out of commands.Host:
// all ordinary command callbacks still resolve against their original View.
func registerSessionNavigation(
	registry *commands.CommandRegistry, open func() error, jump func(int) error, closeSession func() error,
) {
	registry.Register(commands.Command{
		Name: "new", Description: "Open and select a new retained session", Slash: true,
		Run: func(ctx commands.CommandContext) error {
			if len(ctx.Args) != 0 {
				return errors.New("usage: /new (use /clear to replace this conversation)")
			}
			return open()
		},
	})
	registry.Register(commands.Command{
		Name: "switch", Description: "Select a retained session by its opening number", Slash: true,
		Run: func(ctx commands.CommandContext) error {
			if len(ctx.Args) != 1 {
				return errors.New("usage: /switch <session number>")
			}
			n, err := strconv.Atoi(ctx.Args[0])
			if err != nil || n < 1 {
				return fmt.Errorf("invalid session number %q: use /switch with a positive number", ctx.Args[0])
			}
			return jump(n)
		},
	})
	registry.Register(commands.Command{
		Name: "close", Description: "Close this retained session (keep disk history)", Slash: true,
		Run: func(ctx commands.CommandContext) error {
			if len(ctx.Args) != 0 {
				return errors.New("usage: /close (closes this tab, not disk history)")
			}
			return closeSession()
		},
		PaletteRoot: func(ctx commands.CommandContext) palette.PaletteCommand {
			return palette.PaletteCommand{
				ID: "session-close", Noun: "session", Verb: "close tab", Keywords: []string{"close"},
				Run: func() {
					if err := closeSession(); err != nil && ctx.Host != nil {
						ctx.Host.Toast(err.Error(), toast.ToastWarning, 5*time.Second)
					}
				},
			}
		},
	})
}

// childFamilies reconciles the runtime's retained sub-agents against the
// families the views on screen hold. A child never becomes a tab: it is built
// once, handed to the family of the session that spawned it, and retired when
// the runtime stops retaining it.
type childFamilies struct {
	// children is the runtime's creation snapshot. JobIDs stay fixed across
	// follow-up assignments, so a job seen once is never rebuilt.
	children func() []controller.ChildSession
	// views lists every retained view, tabs and sub-agents alike, so a
	// sub-agent that spawns one of its own is found too.
	views func() []*sessions.View
	// build assembles the UI for one child; retire disposes of it.
	build  func(controller.ChildSession) *sessions.View
	retire func(*sessions.View)
	// report surfaces a failure the user must know about.
	report func(string)
	// held maps a retained job onto the family holding it, so a job the
	// runtime released can be found again without searching every family.
	held map[string]*sessions.Family
	// seen remembers every job this shell has ever built a view for. It is
	// never forgotten: a creation snapshot taken before a release must not
	// bring a retired sub-agent back.
	seen map[string]bool
}

// newChildSessionSync installs the reconciliation the shell runs on the UI
// goroutine, once per drain.
func newChildSessionSync(c *childFamilies) func() {
	c.held = make(map[string]*sessions.Family)
	c.seen = make(map[string]bool)
	return c.sync
}

func (c *childFamilies) sync() {
	live := make(map[string]bool)
	for _, child := range c.children() {
		live[child.JobID] = true
		if c.seen[child.JobID] {
			continue
		}
		c.seen[child.JobID] = true
		c.adopt(child)
	}
	for jobID, family := range c.held {
		if live[jobID] {
			continue
		}
		delete(c.held, jobID)
		if view, ok := family.Release(jobID); ok {
			c.retire(view)
		}
	}
}

// adopt builds one child's UI and hands it to its parent's family. The child
// is told the outcome either way: its first turn waits on that acknowledgement.
func (c *childFamilies) adopt(child controller.ChildSession) {
	family, err := c.family(child)
	if err == nil {
		view := c.build(child)
		if err = family.Adopt(child.JobID, child.Title, view); err != nil {
			c.retire(view)
		}
	}
	if family != nil && err == nil {
		c.held[child.JobID] = family
	}
	child.Ready(err)
	if err != nil {
		c.report("Cannot retain sub-agent: " + err.Error())
	}
}

// family finds the family of the session that spawned this child.
func (c *childFamilies) family(child controller.ChildSession) (*sessions.Family, error) {
	for _, view := range c.views() {
		if view.SessionID() == child.ParentSessionID {
			return view.Family(), nil
		}
	}
	return nil, fmt.Errorf("session %s is no longer open", child.ParentSessionID)
}

// bindFamilyScreen tells a view's agent panel how to change the screen: a
// child row shows that sub-agent's session, the main row puts the session
// that owns it back. Neither touches the session selector.
func bindFamilyScreen(ui *editor.Editor, view *sessions.View) {
	view.Family().SetOnShow(func(child *sessions.View) {
		if child == nil {
			ui.ShowMain()
			return
		}
		ui.ShowChild(child)
	})
}
