package main

import (
	"fmt"
	"os"

	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/session"
)

// sessionsCmd lists persisted sessions for the current directory
// (reuses task-001's session.ListSessions).
func sessionsCmd(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "list":
			// ok
		case "-h", "--help":
			fmt.Fprintln(os.Stdout, "usage: cozyphi sessions list")
			return ExitOK
		default:
			fmt.Fprintf(os.Stderr, "cozyphi sessions: unknown subcommand %q\n", args[0])
			return ExitUsage
		}
	}

	proj := project.GetDefaultProject()
	dir := proj.SessionDir()
	list, err := session.ListSessions(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cozyphi sessions:", err)
		return ExitError
	}
	if len(list) == 0 {
		fmt.Fprintf(os.Stderr, "no sessions in %s\n", dir)
		return ExitOK
	}
	for _, s := range list {
		fmt.Println(sessionListLine(s))
	}
	return ExitOK
}

func sessionListLine(s session.SessionMeta) string {
	active := ""
	if s.Active {
		active = " [active]"
	}
	return fmt.Sprintf("%s%s  %s  %s", s.ID, active, s.Mtime.Format("2006-01-02 15:04:05"), s.Preview)
}
