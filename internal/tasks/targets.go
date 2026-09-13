package tasks

import (
	"context"
	"fmt"
	"maps"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// Target is one registry a session may work: the label the model addresses
// it by, the checkout it belongs to, and the registry itself.
type Target struct {
	Label string
	Root  string
	Reg   *Registry
}

// Targets is the set of task registries one session may work: the checkout
// it started in by default, plus the main checkout, the repository's other
// live worktrees, and external roots the user configured. Worktrees are
// re-listed on every resolve, so one created mid-session is addressable
// without a restart.
//
// Every root here is vouched for by where it came from — the launch
// checkout, Git's own worktree list, or the user's config. A target is a
// choice among those, never a path of the model's own, and each write stays
// inside that root's registry directory.
type Targets struct {
	launchRoot string
	mainRoot   string
	extra      map[string]string
	// defRoot is the root reads default to: the launch checkout, or the
	// main checkout when the launch checkout fell back to it.
	defRoot string
	defReg  *Registry
}

// DiscoverTargets resolves the targets of a session launched in launchRoot
// of a repository whose main checkout is mainRoot (both absolute; either
// may be empty, and non-Git directories pass the same root twice). extra
// names external roots by label, from the user's config.
//
// The launch checkout decides the default, and its discovery errors surface
// as Discover's always have. A launch checkout without a registry falls
// back to the main checkout when that has one, so a session in a worktree
// on a branch that predates the registry directory still sees a registry.
// Neither root has one: no targets, and the tool is not offered — the same
// contract as Discover.
func DiscoverTargets(launchRoot, mainRoot string, extra map[string]string) (*Targets, error) {
	launchRoot, mainRoot = canonical(launchRoot), canonical(mainRoot)
	reg, err := Discover(launchRoot)
	if err != nil {
		return nil, err
	}
	defRoot := launchRoot
	if reg == nil && mainRoot != "" && mainRoot != launchRoot {
		reg, err = Discover(mainRoot)
		if err != nil {
			return nil, err
		}
		defRoot = mainRoot
	}
	if reg == nil {
		return nil, nil
	}
	canonicalExtra := make(map[string]string, len(extra))
	for label, root := range extra {
		canonicalExtra[label] = canonical(root)
	}
	t := &Targets{
		launchRoot: launchRoot,
		mainRoot:   mainRoot,
		extra:      canonicalExtra,
		defRoot:    defRoot,
		defReg:     reg,
	}
	return t, nil
}

// canonical is a root in the spelling Git prints: symlinks resolved, so the
// paths a session hands over and the paths `git worktree list` answers meet
// as the same directory instead of two spellings of one. A path that cannot
// be resolved is used as given — the failure mode is a duplicate label,
// never a missing registry.
func canonical(root string) string {
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		return filepath.Clean(resolved)
	}
	return filepath.Clean(root)
}

// Default is the registry a call without a target works: the launch
// checkout's, or the main checkout's when it fell back.
func (t *Targets) Default() *Registry {
	if t == nil {
		return nil
	}
	return t.defReg
}

// DefaultLabel is the label the default root goes by, the same label a
// write would name to reach it.
func (t *Targets) DefaultLabel() string {
	if t == nil {
		return ""
	}
	if t.mainRoot != "" && t.defRoot == t.mainRoot {
		return "main"
	}
	return labelFor(t.defRoot, t.mainRoot)
}

// Snapshot lists the targets addressable right now, default first. A root
// whose registry cannot be found is not a target: the launch checkout's
// errors surfaced at discovery, and one broken worktree or stale config
// entry must not take the tool down with it, so the rest are skipped here.
func (t *Targets) Snapshot() []Target {
	if t == nil {
		return nil
	}
	var (
		out  []Target
		seen = map[string]bool{}
		take = map[string]bool{}
	)
	add := func(label, root string, reg *Registry) {
		if label == "" || root == "" || reg == nil || seen[root] || take[label] {
			return
		}
		seen[root], take[label] = true, true
		out = append(out, Target{Label: label, Root: root, Reg: reg})
	}
	add(t.DefaultLabel(), t.defRoot, t.defReg)
	if t.mainRoot != "" {
		if reg, err := Discover(t.mainRoot); err == nil {
			add("main", t.mainRoot, reg)
		}
	}
	// Configured labels are the user's words for those roots, so they claim
	// their names first; a worktree that would share a name falls back to
	// its path below.
	for _, label := range slices.Sorted(maps.Keys(t.extra)) {
		root := filepath.Clean(t.extra[label])
		if reg, err := Discover(root); err == nil {
			add(label, root, reg)
		}
	}
	for _, root := range listWorktrees(t.mainRoot) {
		if reg, err := Discover(root); err == nil {
			add(freeLabel(root, t.mainRoot, take), root, reg)
		}
	}
	return out
}

// Resolve maps a label to its registry. An empty label is the default; an
// unknown one names the labels that could have been said.
func (t *Targets) Resolve(label string) (*Registry, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return t.Default(), nil
	}
	for _, tgt := range t.Snapshot() {
		if tgt.Label == label {
			return tgt.Reg, nil
		}
	}
	return nil, fmt.Errorf("unknown registry root %q (known: %s)", label, Labels(t.Snapshot()))
}

// Labels joins a snapshot's labels the way error messages quote them.
func Labels(targets []Target) string {
	labels := make([]string, 0, len(targets))
	for _, tgt := range targets {
		labels = append(labels, tgt.Label)
	}
	return strings.Join(labels, ", ")
}

// labelFor names a root the model can say and a human can recognize: the
// main checkout is "main", and everything else goes by its base name — a
// task worktree's base name is its task id, which is what `start` printed
// when it made the worktree.
func labelFor(root, mainRoot string) string {
	if mainRoot != "" && root == mainRoot {
		return "main"
	}
	return filepath.Base(root)
}

// relUnderMain is a root's repository-relative spelling, or "" when the
// root is not inside the main checkout at all.
func relUnderMain(root, mainRoot string) string {
	rel, err := filepath.Rel(mainRoot, root)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	return filepath.ToSlash(rel)
}

// freeLabel is labelFor with a claim on taken names: a base name another
// target already goes by, or the reserved "main", steps up to the longer
// spelling so a label never names two registries.
func freeLabel(root, mainRoot string, taken map[string]bool) string {
	label := labelFor(root, mainRoot)
	if !taken[label] && label != "main" {
		return label
	}
	if rel := relUnderMain(root, mainRoot); rel != "" && !taken[rel] {
		return rel
	}
	return root
}

// listWorktrees is Git's own answer for which checkouts of the repository
// are live, in list order. A repository Git cannot name — or a Git that
// cannot run — has no worktree targets, which is a smaller registry, not a
// broken session.
func listWorktrees(mainRoot string) []string {
	if mainRoot == "" {
		return nil
	}
	cmd := exec.CommandContext(
		context.Background(),
		"git",
		"-C",
		mainRoot,
		"worktree",
		"list",
		"--porcelain",
	)
	output, err := cmd.Output()
	if err != nil {
		return nil
	}
	var (
		roots []string
		root  string
		bare  bool
	)
	flush := func() {
		if root != "" && !bare {
			roots = append(roots, root)
		}
	}
	for line := range strings.SplitSeq(string(output), "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			flush()
			root, bare = strings.TrimSpace(strings.TrimPrefix(line, "worktree ")), false
		case line == "bare":
			bare = true
		}
	}
	flush()
	slices.Sort(roots)
	return roots
}
