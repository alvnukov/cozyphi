package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/app"
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
// Controller. The caller closes ctrl if assembly fails before a View exists.
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
) (*sessions.View, error) {
	settingsManager, err := harnesssettings.Open(proj.Global().ConfigFile(), ctrl.PlanRuntime(), ctrl)
	if err != nil {
		return nil, fmt.Errorf("initialize session settings: %w", err)
	}
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
	return view, nil
}

// registerSessionNavigation keeps process navigation out of commands.Host:
// all ordinary command callbacks still resolve against their original View.
func registerSessionNavigation(registry *commands.CommandRegistry, open func() error, jump func(int) error) {
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
}
