package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/alvnukov/cozyphi/internal/memory"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/usage"
)

const memoryUsage = `usage: cozyphi memory [list|path|show <name>|forget <name>|forgotten]

  list           what the agent remembers about this directory (default)
  path           print the memory directory
  show <name>    print one memory file
  forget <name>  move one memory to forgotten/ (reversible: it is a move)
  forgotten      what has been forgotten, newest first
`

// memoryCmd inspects the agent's memory for the current directory. Writing is
// the agent's job — this is the window into what it decided to keep.
func memoryCmd(args []string) int {
	sub := "list"
	if len(args) > 0 {
		sub = args[0]
	}
	if sub == "-h" || sub == "--help" || sub == "help" {
		fmt.Fprint(os.Stdout, memoryUsage)
		return ExitOK
	}

	proj := project.GetDefaultProject()
	history, _ := usage.Open(proj.Global().UsageFile())
	store, err := memory.Open(proj.MemoryDir(), usage.Memory{Store: history, Dir: proj.MemoryDir()})
	if err != nil {
		fmt.Fprintln(os.Stderr, "cozyphi memory:", err)
		return ExitError
	}

	switch sub {
	case "list":
		return memoryList(store)
	case "path":
		fmt.Println(store.Dir())
		return ExitOK
	case "show":
		if len(args) != 2 {
			fmt.Fprint(os.Stderr, memoryUsage)
			return ExitUsage
		}
		return memoryShow(store, args[1])
	case "forget":
		if len(args) != 2 {
			fmt.Fprint(os.Stderr, memoryUsage)
			return ExitUsage
		}
		return memoryForget(store, args[1])
	case "forgotten":
		return memoryForgotten(store)
	default:
		fmt.Fprintf(os.Stderr, "cozyphi memory: unknown subcommand %q\n", sub)
		return ExitUsage
	}
}

func memoryList(store *memory.Store) int {
	entries := store.Entries()
	if len(entries) == 0 {
		fmt.Fprintf(os.Stderr, "no memories in %s\n", store.Dir())
		return ExitOK
	}
	width := 0
	for _, entry := range entries {
		if n := len(entry.Name); n > width {
			width = n
		}
	}
	for _, entry := range entries {
		fmt.Printf("%-9s  %-*s  %s\n", entry.Kind, width, entry.Name, entry.Description)
	}
	fmt.Printf("\n%s\n", memorySummary(store.Budget()))
	if stale := store.Stale(); len(stale) > 0 {
		fmt.Printf("%d unused for months — 'cozyphi memory forget <name>' archives one\n", len(stale))
	}
	return ExitOK
}

// memorySummary says what memory costs the model. It is the one number the
// session never shows: the memory block rides in the system prompt, which the
// context browser does not itemize.
func memorySummary(budget memory.Budget) string {
	// Three runes per token, near enough for the English/Russian mix in these
	// files; the exact number is the provider's business.
	summary := fmt.Sprintf("%d memories · ~%s tokens in the system prompt · %d in force, %d listed",
		budget.Facts, thousands(budget.Runes/3), budget.Standing, budget.Listed)
	if hidden := budget.Facts - budget.Standing - budget.Listed; hidden > 0 {
		summary += fmt.Sprintf(", %d only in %s", hidden, memory.IndexFile)
	}
	return summary
}

func memoryForget(store *memory.Store, name string) int {
	entry, err := store.Forget(name)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cozyphi memory:", err)
		return ExitError
	}
	fmt.Printf("forgot %s (moved to forgotten/%s)\n", entry.Name, entry.File)
	return ExitOK
}

func memoryForgotten(store *memory.Store) int {
	entries := store.Forgotten()
	if len(entries) == 0 {
		fmt.Fprintf(os.Stderr, "nothing forgotten in %s\n", store.Dir())
		return ExitOK
	}
	for _, entry := range entries {
		fmt.Printf("%-10s  %-24s  %s\n", entry.Modified.Format("2006-01-02"), entry.Name, entry.Description)
	}
	return ExitOK
}

func thousands(n int) string {
	if n < 1000 {
		return strconv.Itoa(n)
	}
	return fmt.Sprintf("%.1fk", float64(n)/1000)
}

func memoryShow(store *memory.Store, name string) int {
	entry, ok := store.Fact(name)
	if !ok {
		fmt.Fprintf(os.Stderr, "cozyphi memory: no memory named %q (try 'cozyphi memory list')\n", name)
		return ExitError
	}
	content, err := os.ReadFile(entry.Path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cozyphi memory:", err)
		return ExitError
	}
	fmt.Printf("# %s\n\n%s", entry.File, content)
	return ExitOK
}
