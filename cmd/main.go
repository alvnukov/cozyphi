package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	//nolint:gosec // G108: pprof handlers on DefaultServeMux; served only when COZYPHI_PPROF is set
	_ "net/http/pprof"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/app"
	"github.com/alvnukov/cozyphi/internal/harnesssettings"
	"github.com/alvnukov/cozyphi/internal/history"
	"github.com/alvnukov/cozyphi/internal/notify"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/tui/commands"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/editor"
	"github.com/alvnukov/cozyphi/internal/tui/keys"
	"github.com/alvnukov/cozyphi/internal/usage"
	"github.com/alvnukov/cozyphi/internal/voice"
)

func main() {
	startPprof()
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "run":
			os.Exit(runCmd(os.Args[2:]))
		case "sessions":
			os.Exit(sessionsCmd(os.Args[2:]))
		case "memory":
			os.Exit(memoryCmd(os.Args[2:]))
		case "mcp":
			os.Exit(mcpCmd(os.Args[2:]))
		case "config":
			os.Exit(configCmd(os.Args[2:]))
		case "update":
			os.Exit(updateCmd(os.Args[2:]))
		case "tui":
			os.Exit(tuiCmd(os.Args[2:]))
		case "-h", "--help", "help":
			printMainUsage(os.Stdout)
			return
		default:
			// TUI flags (`cozyphi -c`, `cozyphi --resume <id>`, …) instead of a
			// subcommand; anything else stays an unknown command.
			if strings.HasPrefix(os.Args[1], "-") {
				os.Exit(tuiCmd(os.Args[1:]))
			}
			fmt.Fprintf(
				os.Stderr,
				"cozyphi: unknown command %q (try 'cozyphi run --help' or 'cozyphi tui')\n",
				os.Args[1],
			)
			os.Exit(ExitUsage)
		}
	}
	os.Exit(tuiCmd(nil))
}

// startPprof serves /debug/pprof on COZYPHI_PPROF (host:port) when set. Intended
// for hang diagnosis: `COZYPHI_PPROF=127.0.0.1:6060 cozyphi`, then curl
// http://127.0.0.1:6060/debug/pprof/goroutine?debug=2.
func startPprof() {
	addr := os.Getenv("COZYPHI_PPROF")
	if addr == "" {
		return
	}
	go func() {
		fmt.Fprintln(os.Stderr, "cozyphi: pprof on http://"+addr+"/debug/pprof/")
		srv := &http.Server{Addr: addr, ReadHeaderTimeout: 5 * time.Second}
		if err := srv.ListenAndServe(); err != nil {
			fmt.Fprintln(os.Stderr, "cozyphi: pprof:", err)
		}
	}()
}

// runTUI starts the interactive terminal UI (default, unchanged behavior).
// resumePath opens an existing session jsonl instead of a new session
// (cozyphi --continue / --resume). It returns an error so main() can pick the
// process exit code.
func runTUI(resumePath string) error {
	proj := project.GetDefaultProject()
	if err := proj.LoadConfig(); err != nil {
		// A missing model is no longer a load error (the TUI starts and says
		// how to get one), so what is left here is a malformed config file.
		fmt.Fprintln(os.Stderr, "cozyphi:", err)
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Fix the config file, then restart:")
		fmt.Fprintln(os.Stderr, "  cozyphi config")
		return &exitError{code: ExitUsage, err: err}
	}
	printConfigWarnings(proj.Config())
	// The keybinds section was validated at load; applying it before any
	// pane exists means every footer, help row and palette shortcut is
	// born with the overridden spellings.
	if err := keys.Rebind(proj.Config().Keybinds); err != nil {
		fmt.Fprintln(os.Stderr, "cozyphi:", err)
		return &exitError{code: ExitUsage, err: err}
	}
	cfg := proj.Config().Model()

	// Download fd/rg in the background so a cold install does not block the
	// first TUI frame. Failures stay non-fatal (tools fall back to PATH).
	go func() {
		if err := EnsureSearchTools(context.Background(), proj); err != nil {
			fmt.Fprintln(os.Stderr, "warning: could not install search tools:", err)
		}
	}()

	vx, err := xui.New(xui.Options{Mouse: true, BracketedPaste: true})
	if err != nil {
		fmt.Fprintln(os.Stderr, "cozyphi: terminal UI:", err)
		return &exitError{code: ExitError, err: err}
	}
	defer func(vx *xui.XUI) {
		err := vx.Close()
		if err != nil {
			panic(err)
		}
	}(vx)

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "cozyphi: getwd:", err)
		return &exitError{code: ExitError, err: err}
	}
	th := components.DefaultTheme()
	models := proj.Config().AllModels()
	modelNames := make([]string, 0, len(models))
	for _, m := range models {
		modelNames = append(modelNames, m.Name)
	}

	application := app.NewApp(vx)

	redraw := controller.NewRedrawRelay()
	bus := controller.NewBus(redraw.Fire)
	usageHistory, usageErr := usage.Open(proj.Global().UsageFile())
	if usageErr != nil {
		fmt.Fprintln(os.Stderr, "warning: could not load usage history:", usageErr)
	}
	ctrl, err := controller.NewController(bus, proj, cwd, resumePath, usageHistory)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cozyphi:", err)
		return &exitError{code: ExitError, err: err}
	}
	// Run returns on every quit path (Ctrl+C included); Close runs
	// session_shutdown hooks and releases jobs/MCP before the process exits.
	defer ctrl.Close()
	settingsManager, err := harnesssettings.Open(proj.Global().ConfigFile(), ctrl.PlanRuntime(), ctrl)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cozyphi: initialize harness settings:", err)
		return &exitError{code: ExitError, err: err}
	}
	cmds := commands.NewBuiltinRegistry(usageHistory)
	// Prompt history degrades to in-memory when the file cannot be read.
	hist := history.Open(history.DefaultPath())
	// Resumed sessions may run a different model than the config default; a
	// session with no model at all shows the placeholder instead of an empty
	// composer label. The label names the selected reasoning effort too.
	cfg = ctrl.ModelConfig()
	modelName := ctrl.ModelLabel()
	ui := editor.NewEditor(
		application,
		bus,
		ctrl,
		cmds,
		vx,
		th,
		cwd,
		modelName,
		cfg.SkillPath,
		cfg.ContextWindow,
		modelNames,
		hist,
		settingsManager,
	)
	// Desktop notifications follow the configured mode (off/always/unfocused)
	// and sound, and stay inert when the OS has no sender for this platform.
	notifications := proj.Config().Notifications
	ui.SetAttentionNotifier(notify.New(notifications.Mode, notify.WithSound(notifications.Sound)))
	// Voice input records through an external command, so it resolves against
	// the same binary lookup the rest of the app uses. CloseVoice runs on every
	// quit path so no capture process outlives the TUI.
	ui.ConfigureVoice(editor.VoiceOptions{
		Config: proj.Config().Voice,
		Env: voice.ResolveEnv{
			GOOS:           runtime.GOOS,
			LookBin:        proj.Global().LookBin,
			ModelsDir:      proj.Global().VoiceModelsDir(),
			ExtraModelDirs: voice.DefaultModelDirs(),
		},
		WAVPath: proj.Global().VoiceWAVFile(),
		// Key releases arrive only under the kitty keyboard protocol, and
		// they are what hold-to-pause and push-to-talk are built on.
		HoldKeys: vx.Caps().KittyKeyboard,
	})
	defer ui.CloseVoice()
	redraw.Bind(ui.RequestRedraw)
	ui.StartUpdateCheck(proj.Global().Root())
	ui.StartProviderModelRefresh()
	ui.StartBranchWatch()
	if err := application.Run(ui); err != nil {
		fmt.Fprintln(os.Stderr, "cozyphi:", err)
		return &exitError{code: ExitError, err: err}
	}
	return nil
}

// exitError carries a process exit code so helpers can fail without calling os.Exit.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string { return e.err.Error() }

func (e *exitError) Unwrap() error { return e.err }

// runTUIExit maps the error returned by runTUI to the process exit code.
func runTUIExit(err error) int {
	if err == nil {
		return ExitOK
	}
	if ee, ok := errors.AsType[*exitError](err); ok {
		return ee.code
	}
	return ExitError
}

func printMainUsage(w *os.File) {
	fmt.Fprintf(w, `usage: cozyphi [COMMAND]

  cozyphi                start the interactive TUI
  cozyphi -c             start the TUI on the newest session for this directory
  cozyphi --resume ID    start the TUI on a session by id or unique prefix
  cozyphi tui            start the interactive TUI (same flags as above)
  cozyphi config         open the HTML config editor (local web server)
  cozyphi update         install the latest release (see 'cozyphi update --help')
  cozyphi run -p "..."   run one agent loop headlessly (see 'cozyphi run --help')
  cozyphi sessions list  list persisted sessions for this directory
  cozyphi memory         show what the agent remembers here (see 'cozyphi memory --help')
  cozyphi mcp …          manage MCP servers (see 'cozyphi mcp --help')
`)
}
