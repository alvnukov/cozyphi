package main

import (
	"fmt"
	"os"

	"github.com/pulseaiclub/phi/internal/project"
	"github.com/pulseaiclub/phi/internal/session"
)

// sessionsCmd lists persisted sessions for the current directory
// (reuses task-001's session.ListSessions).
func sessionsCmd(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "list":
			// ok
		case "-h", "--help":
			fmt.Fprintln(os.Stdout, "usage: phi sessions list")
			return ExitOK
		default:
			fmt.Fprintf(os.Stderr, "phi sessions: unknown subcommand %q\n", args[0])
			return ExitUsage
		}
	}

	proj := project.GetDefaultProject()
	dir := proj.SessionDir()
	list, err := session.ListSessions(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "phi sessions:", err)
		return ExitError
	}
	if len(list) == 0 {
		fmt.Fprintf(os.Stderr, "no sessions in %s\n", dir)
		return ExitOK
	}
	for _, s := range list {
		fmt.Printf("%s  %s  %s\n", s.ID, s.Mtime.Format("2006-01-02 15:04:05"), s.Preview)
	}
	return ExitOK
}
