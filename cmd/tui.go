package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/pulseaiclub/phi/internal/project"
	"github.com/pulseaiclub/phi/internal/session"
)

// tuiCmd parses TUI startup flags, resolves the session to open, and runs the
// TUI. The session resolves before the terminal is handed over, so a typo
// exits with code 3 instead of flashing a UI.
func tuiCmd(args []string) int {
	opts, err := parseTUIArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "phi:", err)
		printTUIUsage(os.Stderr)
		return ExitUsage
	}
	if opts.help {
		printTUIUsage(os.Stdout)
		return ExitOK
	}

	resumePath := ""
	if opts.continueLast || opts.resume != "" {
		proj := project.GetDefaultProject()
		resumePath, err = resolveTUIResumePath(opts, proj.SessionDir())
		if err != nil {
			fmt.Fprintln(os.Stderr, "phi:", err)
			return ExitUsage
		}
	}
	return runTUIExit(runTUI(resumePath))
}

func printTUIUsage(w *os.File) {
	fmt.Fprintf(w, `usage: phi [flags]   (same flags after 'phi tui')

Start the interactive TUI, optionally opening an existing session.

flags:
  -c, --continue     open the newest session for this directory
      --resume ID    open a session by id or unique prefix
  -h, --help         show this help

See 'phi sessions list' for session ids.
`)
}

// tuiOptions holds parsed TUI startup flags (`phi [flags]` / `phi tui [flags]`).
type tuiOptions struct {
	continueLast bool
	resume       string
	help         bool
}

// parseTUIArgs parses TUI startup flags. --continue/-c and --resume are
// mutually exclusive: both select the session to open, in different ways.
// Anything else (including positional words) is a usage error — the TUI takes
// its prompt from the composer, not the command line.
func parseTUIArgs(args []string) (tuiOptions, error) {
	var o tuiOptions
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help":
			o.help = true
		case arg == "-c" || arg == "--continue":
			o.continueLast = true
		case arg == "--resume":
			var value string
			if i+1 < len(args) {
				value = args[i+1]
			}
			if value == "" || strings.HasPrefix(value, "-") {
				return o, errors.New("--resume requires a session id")
			}
			i++
			o.resume = value
		case strings.HasPrefix(arg, "--resume="):
			o.resume = strings.TrimPrefix(arg, "--resume=")
		default:
			return o, fmt.Errorf("unknown flag or argument %q", arg)
		}
	}
	if o.continueLast && o.resume != "" {
		return o, errors.New("--continue and --resume are mutually exclusive")
	}
	return o, nil
}

// resolveTUIResumePath maps TUI startup flags to a session file path. It runs
// before the terminal is handed to the TUI so typos fail fast with exit code 3
// instead of flashing a UI. Empty result means "start a new session".
func resolveTUIResumePath(opts tuiOptions, sessionDir string) (string, error) {
	switch {
	case opts.continueLast:
		list, err := session.ListSessions(sessionDir)
		if err != nil {
			return "", err
		}
		if len(list) == 0 {
			return "", fmt.Errorf("--continue: no sessions in %s yet — start one with plain `phi` first", sessionDir)
		}
		return list[0].File, nil
	case opts.resume != "":
		if _, statErr := os.Stat(sessionDir); statErr != nil {
			return "", fmt.Errorf("--resume: no sessions in %s yet — start one with plain `phi` first", sessionDir)
		}
		path, err := session.FindSessionFile(sessionDir, opts.resume)
		if err != nil {
			return "", fmt.Errorf("--resume: %w", err)
		}
		return path, nil
	default:
		return "", nil
	}
}
