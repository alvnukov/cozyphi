package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	//nolint:gosec // G108: pprof handlers on DefaultServeMux; served only when PHI_PPROF is set
	_ "net/http/pprof"
	"os"
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/app"
	"github.com/pulseaiclub/phi/internal/project"
	"github.com/pulseaiclub/phi/internal/tui/commands"
	"github.com/pulseaiclub/phi/internal/tui/controller"
	"github.com/pulseaiclub/phi/internal/tui/editor"
)

func main() {
	startPprof()
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "run":
			os.Exit(runCmd(os.Args[2:]))
		case "sessions":
			os.Exit(sessionsCmd(os.Args[2:]))
		case "mcp":
			os.Exit(mcpCmd(os.Args[2:]))
		case "config":
			os.Exit(configCmd(os.Args[2:]))
		case "update":
			os.Exit(updateCmd(os.Args[2:]))
		case "tui":
			os.Exit(runTUIExit(runTUI()))
		case "-h", "--help", "help":
			printMainUsage(os.Stdout)
			return
		default:
			fmt.Fprintf(os.Stderr, "phi: unknown command %q (try 'phi run --help' or 'phi tui')\n", os.Args[1])
			os.Exit(ExitUsage)
		}
	}
	os.Exit(runTUIExit(runTUI()))
}

// startPprof serves /debug/pprof on PHI_PPROF (host:port) when set. Intended
// for hang diagnosis: `PHI_PPROF=127.0.0.1:6060 phi`, then curl
// http://127.0.0.1:6060/debug/pprof/goroutine?debug=2.
func startPprof() {
	addr := os.Getenv("PHI_PPROF")
	if addr == "" {
		return
	}
	go func() {
		fmt.Fprintln(os.Stderr, "phi: pprof on http://"+addr+"/debug/pprof/")
		srv := &http.Server{Addr: addr, ReadHeaderTimeout: 5 * time.Second}
		if err := srv.ListenAndServe(); err != nil {
			fmt.Fprintln(os.Stderr, "phi: pprof:", err)
		}
	}()
}

// runTUI starts the interactive terminal UI (default, unchanged behavior).
// It returns an error so main() can pick the process exit code.
func runTUI() error {
	proj := project.GetDefaultProject()
	if err := proj.LoadConfig(); err != nil {
		fmt.Fprintln(os.Stderr, "phi:", err)
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Configure a model first, then restart:")
		fmt.Fprintln(os.Stderr, "  phi config")
		fmt.Fprintln(os.Stderr, "or set PHI_MODEL and PHI_API_KEY.")
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
		fmt.Fprintln(os.Stderr, "phi: terminal UI:", err)
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
		fmt.Fprintln(os.Stderr, "phi: getwd:", err)
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
	ctrl, err := controller.NewController(bus, proj, cwd)
	if err != nil {
		fmt.Fprintln(os.Stderr, "phi:", err)
		return &exitError{code: ExitError, err: err}
	}
	// Run returns on every quit path (Ctrl+C included); Close runs
	// session_shutdown hooks and releases jobs/MCP before the process exits.
	defer ctrl.Close()
	cmds := commands.NewBuiltinRegistry()
	ui := editor.NewEditor(
		application,
		bus,
		ctrl,
		cmds,
		vx,
		th,
		cwd,
		cfg.Name,
		cfg.SkillPath,
		cfg.ContextWindow,
		modelNames,
	)
	redraw.Bind(ui.RequestRedraw)
	ui.StartUpdateCheck(proj.Global().Root())
	ui.StartBranchWatch()
	if err := application.Run(ui); err != nil {
		fmt.Fprintln(os.Stderr, "phi:", err)
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
	fmt.Fprintf(w, `usage: phi [COMMAND]

  phi                start the interactive TUI
  phi tui            start the interactive TUI
  phi config         open the HTML config editor (local web server)
  phi update         install the latest release (see 'phi update --help')
  phi run -p "..."   run one agent loop headlessly (see 'phi run --help')
  phi sessions list  list persisted sessions for this directory
  phi mcp …          manage MCP servers (see 'phi mcp --help')
`)
}
