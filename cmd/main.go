package main

import (
	"context"
	"fmt"
	"os"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/app"
	"github.com/pulseaiclub/phi/internal/project"
	"github.com/pulseaiclub/phi/internal/tui"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "run":
			os.Exit(runCmd(os.Args[2:]))
		case "sessions":
			os.Exit(sessionsCmd(os.Args[2:]))
		case "config":
			os.Exit(configCmd(os.Args[2:]))
		case "update":
			os.Exit(updateCmd(os.Args[2:]))
		case "tui":
			runTUI()
			return
		case "-h", "--help", "help":
			printMainUsage(os.Stdout)
			return
		default:
			fmt.Fprintf(os.Stderr, "phi: unknown command %q (try 'phi run --help' or 'phi tui')\n", os.Args[1])
			os.Exit(ExitUsage)
		}
	}
	runTUI()
}

// runTUI starts the interactive terminal UI (default, unchanged behavior).
func runTUI() {
	proj := project.GetDefaultProject()
	if err := proj.LoadConfig(); err != nil {
		fmt.Fprintln(os.Stderr, "phi:", err)
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Configure a model first, then restart:")
		fmt.Fprintln(os.Stderr, "  phi config")
		fmt.Fprintln(os.Stderr, "or set PHI_MODEL and PHI_API_KEY.")
		os.Exit(ExitUsage)
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
		os.Exit(ExitError)
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
		os.Exit(ExitError)
	}
	th := components.DefaultTheme()
	models := proj.Config().AllModels()
	modelNames := make([]string, 0, len(models))
	for _, m := range models {
		modelNames = append(modelNames, m.Name)
	}
	m := tui.NewEditor(vx, th, cwd, cfg.Name, cfg.SkillPath, cfg.ContextWindow, modelNames)

	app := app.NewApp(vx)
	app.Anim = true
	m.App = app
	m.StartUpdateCheck()
	if err := app.Run(m); err != nil {
		fmt.Fprintln(os.Stderr, "phi:", err)
		os.Exit(ExitError)
	}
}

func printMainUsage(w *os.File) {
	fmt.Fprintf(w, `usage: phi [COMMAND]

  phi                start the interactive TUI
  phi tui            start the interactive TUI
  phi config         open the HTML config editor (local web server)
  phi update         install the latest release (see 'phi update --help')
  phi run -p "..."   run one agent loop headlessly (see 'phi run --help')
  phi sessions list  list persisted sessions for this directory
`)
}
