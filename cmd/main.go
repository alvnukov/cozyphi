package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	//nolint:gosec // G108: pprof handlers on DefaultServeMux; served only when COZYPHI_PPROF is set
	_ "net/http/pprof"
	"os"
	"strings"
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/app"
	"github.com/alvnukov/cozyphi/internal/components/toast"
	"github.com/alvnukov/cozyphi/internal/harnesssettings"
	"github.com/alvnukov/cozyphi/internal/history"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/commands"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/editor"
	"github.com/alvnukov/cozyphi/internal/tui/keys"
	"github.com/alvnukov/cozyphi/internal/tui/sessions"
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
// acquired transfers an already owned history into the controller, without
// releasing and reopening it. Early startup failures release it here.
func runTUI(acquired *session.Manager) (runErr error) {
	defer func() {
		if acquired != nil {
			if err := acquired.Close(); err != nil {
				fmt.Fprintln(os.Stderr, "cozyphi: close session:", err)
			}
		}
	}()
	resumePath := ""
	if acquired != nil {
		resumePath = acquired.File()
	}
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

	application := app.NewApp(vx)

	redraw := controller.NewRedrawRelay()
	usageHistory, usageErr := usage.Open(proj.Global().UsageFile())
	if usageErr != nil {
		fmt.Fprintln(os.Stderr, "warning: could not load usage history:", usageErr)
	}
	process, err := controller.NewRuntime(proj, usageHistory)
	if err != nil {
		return &exitError{code: ExitError, err: err}
	}
	defer func() { runErr = errors.Join(runErr, process.Close()) }()
	// Bind the first engine to the interactive adapter, not the headless runner.
	process.EnableInteractiveChildren()
	workspace, err := process.Workspace(cwd)
	if err != nil {
		return &exitError{code: ExitError, err: err}
	}
	// One settings manager per process: every session sees one token for the
	// config file and receives every committed snapshot.
	settingsManager, err := harnesssettings.Open(proj.Global().ConfigFile(), process.PlanRuntime(), nil)
	if err != nil {
		return &exitError{code: ExitError, err: fmt.Errorf("initialize settings: %w", err)}
	}
	registry := sessions.NewRegistry(12, application.RequestRedraw)
	ui := editor.NewEditor(application, registry)
	redraw.Bind(ui.RequestRedraw)
	captureGate := voice.NewCaptureGate()
	// Every View gets a cursor; only the append-only history corpus is shared.
	hist := history.Open(history.DefaultPath())
	var openNew func() error
	create := func(path string, owner *session.Manager) (*sessions.View, error) {
		bus := controller.NewBus(redraw.Fire)
		ctrl, err := process.NewSession(bus, workspace, path, owner)
		if err != nil {
			return nil, err
		}
		cmds := commands.NewBuiltinRegistry(usageHistory)
		registerSessionNavigation(cmds, openNew, ui.Jump)
		view := newTUIView(application, vx, th, proj, ctrl, bus, hist, workspace.Root(), captureGate, cmds,
			settingsManager)
		view.ConfigureSessionNavigation(registry, ui.Activate)
		return view, nil
	}
	// Names count openings, not live members: once sessions can close, a new
	// one must not reuse the number of one that is still open.
	opened := 1
	openNew = func() error {
		if registry.Len() >= 12 {
			return errors.New("session limit (12) reached: close a session before opening another")
		}
		view, err := create("", nil)
		if err != nil {
			return err
		}
		opened++
		id, err := registry.Open(fmt.Sprintf("session %d", opened), view)
		if err != nil {
			return errors.Join(err, view.Close(context.Background()))
		}
		return ui.Activate(id)
	}
	transferred := acquired
	acquired = nil // Runtime.NewSession consumes ownership even on failure.
	first, err := create(resumePath, transferred)
	if err != nil {
		return &exitError{code: ExitError, err: err}
	}
	id, err := registry.Open("main", first)
	if err != nil {
		return errors.Join(err, first.Close(context.Background()))
	}
	if err := ui.Activate(id); err != nil {
		return errors.Join(err, first.Close(context.Background()))
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := ui.Close(ctx); err != nil {
			fmt.Fprintln(os.Stderr, "cozyphi: session shutdown:", err)
		}
	}()
	seenChildren := make(map[string]bool)
	ui.SetSessionSync(func() {
		for _, child := range process.Children() {
			if seenChildren[child.JobID] {
				continue
			}
			seenChildren[child.JobID] = true
			cmds := commands.NewBuiltinRegistry(usageHistory)
			registerSessionNavigation(cmds, openNew, ui.Jump)
			view := newTUIView(
				application,
				vx,
				th,
				child.Project,
				child.Controller,
				child.Bus,
				hist,
				child.Workspace.Root(),
				captureGate,
				cmds,
				settingsManager,
			)
			view.ConfigureSessionNavigation(registry, ui.Activate)
			name := child.Name
			if name == "" {
				name = child.JobID
			}
			_, err := registry.Open(name, view)
			child.Ready(err)
			if err != nil {
				child.Controller.Close()
				if view != nil {
					_ = view.Close(context.Background())
				}
				if active, ok := registry.Active(); ok {
					active.View.Toast("Cannot retain child view: "+err.Error(), toast.ToastWarning, 6*time.Second)
				}
			}
		}
	})
	first.StartUpdateCheck(proj.Global().Root())
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
