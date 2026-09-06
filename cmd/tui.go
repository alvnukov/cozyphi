package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/session"
)

// tuiCmd parses TUI startup flags, resolves the session to open, and runs the
// TUI. The session resolves before the terminal is handed over, so a typo
// exits with code 3 instead of flashing a UI.
func tuiCmd(args []string) int {
	opts, err := parseTUIArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cozyphi:", err)
		printTUIUsage(os.Stderr)
		return ExitUsage
	}
	if opts.help {
		printTUIUsage(os.Stdout)
		return ExitOK
	}

	var acquired *session.Manager
	if opts.continueLast || opts.resume != "" {
		proj := project.GetDefaultProject()
		acquired, err = resolveTUIResumeSession(opts, proj.SessionDir())
		if err != nil {
			fmt.Fprintln(os.Stderr, "cozyphi:", err)
			return ExitUsage
		}
	}
	return runTUIExit(runTUI(acquired, opts.developerMode))
}

func printTUIUsage(w *os.File) {
	fmt.Fprintf(w, `usage: cozyphi [flags]   (same flags after 'cozyphi tui')

Start the interactive TUI, optionally opening an existing session.

flags:
  -c, --continue        open the newest free session, or start a new one
      --resume ID       open a session by id or unique prefix
      --developer-mode  let the model read cozyphi's own configuration (read-only harness tool)
  -h, --help            show this help

See 'cozyphi sessions list' for session ids.
`)
}

// tuiOptions holds parsed TUI startup flags (`cozyphi [flags]` / `cozyphi tui [flags]`).
type tuiOptions struct {
	continueLast bool
	resume       string
	// developerMode is read from args only. Nothing else grants it: not the
	// config file, not the environment, not a resumed session's history.
	developerMode bool
	help          bool
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
		case arg == "--developer-mode":
			o.developerMode = true
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

// resolveTUIResumeSession acquires before terminal startup. The caller must
// transfer the manager into the controller or close it on startup failure.
func resolveTUIResumeSession(opts tuiOptions, sessionDir string) (*session.Manager, error) {
	switch {
	case opts.continueLast:
		m, err := session.OpenLatestSession(sessionDir)
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return m, err
	case opts.resume != "":
		path, err := session.FindSessionFile(sessionDir, opts.resume)
		if err != nil {
			return nil, fmt.Errorf("--resume: %w", err)
		}
		m, err := session.OpenSession(path)
		if err != nil {
			return nil, fmt.Errorf("--resume: %w", err)
		}
		return m, nil
	default:
		return nil, nil
	}
}
