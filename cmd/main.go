package main

import (
	"context"
	"fmt"
	"os"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/app"
	"github.com/pulseaiclub/phi/internal/project"
	"github.com/pulseaiclub/phi/internal/tui"
	"github.com/pulseaiclub/xui"
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
		fmt.Fprintln(os.Stderr, err)
		return
	}
	cfg := proj.Config().Model()

	if err := EnsureSearchTools(context.Background(), proj); err != nil {
		fmt.Fprintln(os.Stderr, "warning: could not install search tools:", err)
	}

	vx, err := xui.New(xui.Options{Mouse: true, BracketedPaste: true})
	if err != nil {
		panic(err)
	}
	defer func(vx *xui.XUI) {
		err := vx.Close()
		if err != nil {
			panic(err)
		}
	}(vx)

	cwd, _ := os.Getwd()
	th := components.DefaultTheme()
	m := tui.NewEditor(vx, th, cwd, cfg.Name, cfg.SkillPath, cfg.ContextWindow)

	app := app.NewApp(vx)
	app.Anim = true
	m.App = app
	m.StartUpdateCheck()
	if err := app.Run(m); err != nil {
		panic(err)
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
