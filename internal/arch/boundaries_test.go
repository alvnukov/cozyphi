package arch

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	// modulePath is this module. An import that does not start with it comes
	// from the standard library or a dependency, and only the rule that names
	// one cares about it.
	modulePath = "github.com/alvnukov/cozyphi"

	// xuiModule is the vendored terminal framework. It is a module of its own,
	// so it never shows up in this module's package list — only in imports.
	xuiModule = "github.com/pulseaiclub/xui"
)

// pkg is the slice of `go list -json` output these rules read.
type pkg struct {
	ImportPath   string
	Imports      []string
	TestImports  []string
	XTestImports []string
}

// path is the import path with the module prefix stripped — "internal/util"
// rather than the whole thing, because that is how the rules below read.
func (p pkg) path() string { return short(p.ImportPath) }

// deps are the module-internal packages p imports. Test files count: a test
// that reaches across a boundary drags the same dependency into the build
// graph, and it is usually where the first crossing is tried.
func (p pkg) deps() []string {
	seen := map[string]bool{}
	var out []string
	for _, group := range [][]string{p.Imports, p.TestImports, p.XTestImports} {
		for _, imp := range group {
			if !strings.HasPrefix(imp, modulePath) || seen[imp] {
				continue
			}
			seen[imp] = true
			out = append(out, short(imp))
		}
	}
	slices.Sort(out)
	return out
}

// buildDeps are the module-internal packages p imports outside its tests —
// what the shipped binary actually carries, which is what fan-out measures.
func (p pkg) buildDeps() []string {
	var out []string
	for _, imp := range p.Imports {
		if strings.HasPrefix(imp, modulePath) {
			out = append(out, short(imp))
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func short(path string) string {
	return strings.TrimPrefix(strings.TrimPrefix(path, modulePath), "/")
}

// under reports whether path is prefix itself or a package inside it.
func under(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

func underAny(path string, prefixes []string) bool {
	return slices.ContainsFunc(prefixes, func(prefix string) bool { return under(path, prefix) })
}

// load lists every package in the module with its imports. It shells out to
// `go list` rather than taking a dependency on go/packages, which runs this
// exact command underneath: these rules need names and nothing else, and a
// check that guards the import graph should not widen it.
func load(t *testing.T) []pkg {
	t.Helper()
	out, err := exec.CommandContext(
		t.Context(), "go", "list", "-json=ImportPath,Imports,TestImports,XTestImports", modulePath+"/...",
	).Output()
	if err != nil {
		if exit, ok := errors.AsType[*exec.ExitError](err); ok {
			t.Fatalf("go list: %v\n%s", err, exit.Stderr)
		}
		t.Fatalf("go list: %v", err)
	}
	dec := json.NewDecoder(bytes.NewReader(out))
	var pkgs []pkg
	for {
		var p pkg
		if err := dec.Decode(&p); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatalf("decode go list output: %v", err)
		}
		pkgs = append(pkgs, p)
	}
	require.NotEmpty(t, pkgs, "go list returned no packages")
	return pkgs
}

// TestUtilStaysALeaf pins the one property that makes internal/util usable as
// the home for shared helpers.
func TestUtilStaysALeaf(t *testing.T) {
	for _, p := range load(t) {
		if p.path() != "internal/util" {
			continue
		}
		require.Empty(t, p.deps(),
			"internal/util is the shared-helper leaf, so every package may reach it — which means "+
				"anything it reaches back can only return as an import cycle, and the next helper "+
				"that belongs here will not be able to move. Move the dependency out, or keep the "+
				"helper in the package that needs it.")
		return
	}
	t.Fatal("internal/util is missing; the rule above has nothing left to protect")
}

// uiPrefixes own the terminal: what is drawn, what a key does, what a pane
// holds.
var uiPrefixes = []string{"internal/tui", "internal/components"}

// uiConsumers are allowed to import the UI because showing it is their job.
var uiConsumers = []string{"cmd"}

// recordedUIExceptions are the edges that cross the boundary today. Each entry
// is debt with a name on it, not permission: it gets deleted when the edge
// does, and a new crossing adds a failure rather than a line here.
var recordedUIExceptions = map[string][]string{
	// project validates the keybind names it finds in a config file, and the
	// names belong to the UI that binds them. Lowering the name table below
	// the UI would settle it.
	"internal/project": {"internal/tui/keys"},
}

// TestTheTerminalLayerIsASink keeps the UI at the edge of the program. Domain
// code that imports a pane can only be exercised by drawing one, and the
// headless entry points — cozyphi run, the sub-agent, the diagnostics — stop
// being able to do their work without a terminal.
func TestTheTerminalLayerIsASink(t *testing.T) {
	for _, p := range load(t) {
		from := p.path()
		if underAny(from, uiPrefixes) || underAny(from, uiConsumers) {
			continue
		}
		for _, dep := range p.deps() {
			if !underAny(dep, uiPrefixes) {
				continue
			}
			require.Contains(t, recordedUIExceptions[from], dep,
				"%s imports %s: the terminal layer is a sink, so nothing below it may depend on "+
					"what is drawn. Pass the value the UI needs, or move the shared type down.",
				from, dep)
		}
	}
}

// TestTheTerminalFrameworkStaysInTheTerminalLayer keeps the vendored xui fork
// where it can be replaced. Every package that names one of its types is a
// package that has to be rewritten if the framework is ever swapped, so the
// count of them is worth holding at the layer that already draws.
func TestTheTerminalFrameworkStaysInTheTerminalLayer(t *testing.T) {
	for _, p := range load(t) {
		from := p.path()
		if underAny(from, uiPrefixes) || underAny(from, uiConsumers) {
			continue
		}
		for _, group := range [][]string{p.Imports, p.TestImports, p.XTestImports} {
			for _, imp := range group {
				require.False(t, under(imp, xuiModule),
					"%s imports %s: the terminal framework is a detail of the terminal layer, and a "+
						"domain package that knows about cells and key events cannot be tested, or "+
						"reused, without one.", from, imp)
			}
		}
	}
}

// layerRules are the upward imports that would invert the program's layering.
// Each says what the rule buys, in the voice the failure needs.
var layerRules = []struct {
	why   string
	from  string
	deny  []string
	allow []string
}{
	{
		why: "the model transport sits below the loop that drives it: a provider client " +
			"that reaches up into the agent cannot be exercised without one",
		from: "internal/llm",
		deny: []string{"internal/agent", "internal/session", "internal/tools"},
	},
	{
		why: "a plugin is data found on disk; discovery that reaches into the loop " +
			"or the tools it feeds cannot be tested without them",
		from: "internal/plugin",
		deny: []string{"internal/agent", "internal/session", "internal/tools"},
	},
	{
		why: "a transcript is a record, not a participant: session code that calls the engine " +
			"turns replaying history into running it",
		from: "internal/session",
		deny: []string{"internal/agent"},
	},
	{
		why: "the gate decides about a tool call without being able to make one; it knows the " +
			"shape of a request and nothing about who serves it",
		from:  "internal/permission",
		deny:  []string{"internal/tools"},
		allow: []string{"internal/tools/tooldef"},
	},
	{
		why: "a tool is called by the engine and does not call it back; the dependency that " +
			"way round is what lets a tool be tested on its own",
		from: "internal/tools",
		deny: []string{"internal/agent"},
	},
	{
		why: "a provider knows how to talk to one vendor and nothing about the conversation " +
			"being had",
		from: "internal/provider",
		deny: []string{"internal/agent", "internal/session", "internal/tools"},
	},
	{
		why: "diagnostics observe the program; a reporter that imports what it reports on can " +
			"be the reason a build is broken",
		from: "internal/diag",
		deny: []string{"internal/agent", "internal/session", "internal/tools"},
	},
	{
		why:  "memory is a corpus of files, below everything that reads one",
		from: "internal/memory",
		deny: []string{"internal/agent", "internal/session", "internal/tools"},
	},
	{
		why: "an MCP server is a transport; it is spoken to by the tool layer, not the other " +
			"way round",
		from: "internal/mcp",
		deny: []string{"internal/agent", "internal/session"},
	},
}

// TestLayersDoNotReachUpwards checks the rules above against the real graph. It
// reads what the binary carries and not what the tests do, because a contract
// test that reads both sides of a boundary — internal/tools against the prompt
// text that describes its tools — is the thing the rules exist to make
// possible, not a breach of them. A rule may be violated only by deleting it
// and saying why.
func TestLayersDoNotReachUpwards(t *testing.T) {
	pkgs := load(t)
	for _, rule := range layerRules {
		t.Run(rule.from, func(t *testing.T) {
			for _, p := range pkgs {
				from := p.path()
				if !under(from, rule.from) {
					continue
				}
				for _, dep := range p.buildDeps() {
					if under(dep, rule.from) || underAny(dep, rule.allow) {
						continue
					}
					for _, denied := range rule.deny {
						require.False(t, under(dep, denied),
							"%s imports %s, and it must not: %s.", from, dep, rule.why)
					}
				}
			}
		})
	}
}

// fanOutCeiling is the most module-internal packages any one package imports
// today: internal/tui/sessions, which is the view, the lifecycle, the editor
// queue and the ownership rules in one directory. It is a ceiling and not a
// target — a package that reaches this far has stopped having a subject — so
// it may be lowered whenever a package is split, and never raised.
const fanOutCeiling = 47

func TestFanOutStaysUnderItsCeiling(t *testing.T) {
	for _, p := range load(t) {
		deps := p.buildDeps()
		require.LessOrEqual(t, len(deps), fanOutCeiling,
			"%s imports %d packages of this module. The ceiling is the widest package there was "+
				"when the rule was written, so a package past it is new breadth: give the new work "+
				"its own package, or lower the ceiling once %s has been split.",
			p.path(), len(deps), p.path())
	}
}
