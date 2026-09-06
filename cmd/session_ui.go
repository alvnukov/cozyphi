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

// Runtime.Children returns creation records, whose JobID stays fixed across
// follow-up assignments. Keep those IDs after tab removal, without retaining
// closed Controller graphs: a stale creation snapshot must not resurrect a View.
func newChildSessionSync(
	children func() []controller.ChildSession, retain func(controller.ChildSession),
) func() {
	seen := make(map[string]bool)
	return func() {
		for _, child := range children() {
			if seen[child.JobID] {
				continue
			}
			seen[child.JobID] = true
			retain(child)
		}
	}
}
