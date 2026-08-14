package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/pulseaiclub/phi/internal/mcp"
)

func mcpCmd(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		printMCPUsage(os.Stdout)
		return ExitOK
	}
	switch args[0] {
	case "list":
		return mcpList()
	case "add":
		return mcpAdd(args[1:])
	case "remove", "rm":
		return mcpRemove(args[1:])
	case "call":
		return mcpCall(args[1:])
	case "doctor":
		return mcpDoctor()
	default:
		fmt.Fprintf(os.Stderr, "phi mcp: unknown subcommand %q\n", args[0])
		printMCPUsage(os.Stderr)
		return ExitUsage
	}
}

func printMCPUsage(w *os.File) {
	fmt.Fprintf(w, `usage: phi mcp <command>

  phi mcp list                         list configured servers
  phi mcp add <name> -- <cmd> [args…]  add a stdio server to ~/.phi/mcp.json
  phi mcp remove <name>                remove a server from user config
  phi mcp call <server> <tool> [json]  call a tool (optional JSON args object)
  phi mcp doctor                       check config + connectivity

Set PHI_MCP=off to disable MCP meta-tools in the agent.
See doc/mcp.md.
`)
}

func mcpList() int {
	cwd, _ := os.Getwd()
	servers, err := mcp.Load(cwd)
	if err != nil {
		fmt.Fprintln(os.Stderr, "phi mcp list:", err)
		return ExitError
	}
	if len(servers) == 0 {
		fmt.Println("(no servers — try: phi mcp add fetch -- npx -y @modelcontextprotocol/server-fetch)")
		return ExitOK
	}
	for _, name := range sortedKeys(servers) {
		cfg := servers[name]
		cmd, _ := cfg.CmdLine()
		fmt.Printf("%s\t%s\n", name, strings.Join(cmd, " "))
	}
	return ExitOK
}

func sortedKeys(m map[string]mcp.ServerConfig) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func mcpAdd(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: phi mcp add <name> -- <command> [args…]")
		return ExitUsage
	}
	name := args[0]
	rest := args[1:]
	// allow optional "--"
	if len(rest) > 0 && rest[0] == "--" {
		rest = rest[1:]
	}
	if len(rest) == 0 {
		fmt.Fprintln(os.Stderr, "phi mcp add: missing command after --")
		return ExitUsage
	}
	cfg := mcp.ServerConfig{
		Transport: "stdio",
		Command:   rest[:1],
		Args:      rest[1:],
	}
	if err := mcp.AddServer(name, cfg); err != nil {
		fmt.Fprintln(os.Stderr, "phi mcp add:", err)
		return ExitError
	}
	fmt.Printf("added %s\n", name)
	return ExitOK
}

func mcpRemove(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: phi mcp remove <name>")
		return ExitUsage
	}
	ok, err := mcp.RemoveServer(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "phi mcp remove:", err)
		return ExitError
	}
	if !ok {
		fmt.Fprintf(os.Stderr, "phi mcp remove: %q not in user config\n", args[0])
		return ExitError
	}
	fmt.Printf("removed %s\n", args[0])
	return ExitOK
}

func mcpCall(args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: phi mcp call <server> <tool> [json-args]")
		return ExitUsage
	}
	server, tool := args[0], args[1]
	argMap := map[string]any{}
	if len(args) >= 3 {
		raw := strings.Join(args[2:], " ")
		if err := json.Unmarshal([]byte(raw), &argMap); err != nil {
			fmt.Fprintln(os.Stderr, "phi mcp call: args must be a JSON object:", err)
			return ExitUsage
		}
	}
	cwd, _ := os.Getwd()
	pool, err := mcp.LoadPool(cwd)
	if err != nil {
		fmt.Fprintln(os.Stderr, "phi mcp call:", err)
		return ExitError
	}
	if pool == nil {
		fmt.Fprintln(os.Stderr, "phi mcp call: MCP disabled (PHI_MCP=off)")
		return ExitError
	}
	defer func() { _ = pool.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	out, err := pool.Call(ctx, server, tool, argMap)
	if err != nil {
		fmt.Fprintln(os.Stderr, "phi mcp call:", err)
		return ExitError
	}
	fmt.Println(out)
	return ExitOK
}

func mcpDoctor() int {
	cwd, _ := os.Getwd()
	if mcp.Disabled() {
		fmt.Println("PHI_MCP=off — MCP disabled")
		return ExitOK
	}
	pool, err := mcp.LoadPool(cwd)
	if err != nil {
		fmt.Fprintln(os.Stderr, "phi mcp doctor:", err)
		return ExitError
	}
	if pool == nil {
		fmt.Println("no pool")
		return ExitError
	}
	defer func() { _ = pool.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	results := pool.Doctor(ctx)
	fail := 0
	for _, r := range results {
		status := "ok"
		if !r.OK {
			status = "FAIL"
			fail++
		}
		fmt.Printf("%s\t%s\t%s\n", status, r.Name, r.Detail)
	}
	if fail > 0 {
		return ExitError
	}
	return ExitOK
}
