package hooks

import (
	"fmt"
	"time"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// everyTool is the selector of a tool-loop hook that names no tool. The
// matcher treats an empty selector and "*" alike, and reporting the two
// differently would invent a distinction the tool loop does not make.
const everyTool = "*"

// LoadFacts is what one load of the hook directories saw and a Manager
// cannot answer afterwards. A Manager is a list of entries: it does not know
// which directory defined one, whose definition that one replaced, or how
// many problems the load met on the way. Those belong to the load — and a
// manager that is simply absent cannot, on its own, tell hooks switched off
// from a load that never happened, which is most of what the answer is worth.
//
// Every member is unexported on purpose. The value travels from the load site
// to the observation site through owners that only carry it, so a plugin
// file, a run path or the text of a warning has nowhere to leak from.
type LoadFacts struct {
	// attempted is whether a load happened at all. A zero LoadFacts is a
	// process that never loaded hooks, and the whole layer reports
	// unavailable rather than "no hooks are configured".
	attempted bool
	// disabled is whether the environment switched hooks off, in which case
	// discovery read no directory at all.
	disabled bool
	// failed is whether discovery returned an unexpected error.
	failed bool
	// userDir and projectDir are the two directories consulted, in
	// precedence order.
	userDir    string
	projectDir string
	// warnings is how many non-fatal problems the load met. What they said
	// is not kept: a warning quotes the file it was found in.
	warnings int
	// found is the safe half of every manifest discovery kept.
	found []manifestFacts
}

// manifestFacts is the safe half of one discovered manifest: what it is
// called, what it answers, where it came from and how it behaves — with no
// member for the run path, the plugin file, the hook directory, an argument
// or the text of a warning to land in.
type manifestFacts struct {
	name       string
	event      Kind
	origins    []string
	plugin     string
	match      string
	timeout    time.Duration
	failClosed bool
	async      bool
}

// entryKey addresses one registration. A name alone is not enough: the same
// plugin may register one name for two events, and the manager keeps them as
// two entries.
type entryKey struct {
	name  string
	event Kind
}

// ObserveLoad records what one load of the hook directories saw. It is called
// where the load is, because that is the only place the directories, the
// discovery and the warnings exist together.
//
// The error and the warnings are counted and dropped on purpose: a warning
// names the plugin file it was found in and quotes the text that would not
// parse, and a load error names the directory it could not read. How many
// there were leaves this call; nothing of what they said does.
func ObserveLoad(userDir, projectDir string, found []Discovered, warns []Warning, err error) LoadFacts {
	facts := LoadFacts{
		attempted:  true,
		disabled:   HooksDisabled(),
		failed:     err != nil,
		userDir:    userDir,
		projectDir: projectDir,
		warnings:   len(warns),
		found:      make([]manifestFacts, 0, len(found)),
	}
	for _, d := range found {
		facts.found = append(facts.found, manifestFacts{
			name:       d.Manifest.Name,
			event:      d.Manifest.Kind,
			origins:    append(append([]string(nil), d.Shadowed...), d.Source),
			plugin:     d.Manifest.Plugin,
			match:      toolSelector(d.Manifest.Kind, d.Manifest.Match),
			timeout:    hookTimeout(d.Manifest.Timeout),
			failClosed: d.Manifest.FailClosed,
			async:      d.Manifest.Async,
		})
	}
	return facts
}

// Count is how many hooks the load discovered, for a caller that reports the
// size of a reload without being handed the discovery itself.
func (f LoadFacts) Count() int { return len(f.found) }

// Observe reports the hook layer's state for the harness view: whether hooks
// are switched on, what the last load of the directories found, which
// directory defined each one, what the manager in force actually holds, and
// how the two differ.
//
// It reads the manager's own entry list and the load's own record, and does
// nothing else. No hook is run — not an external script, not an in-process
// one — no directory is re-read, no manifest re-parsed, no manager rebuilt
// and no hook policy changed. A hook that has never fired is reported as one,
// and the tool loop's own pre/post hooks keep running exactly as the executor
// arranges them: this view is not in that path and cannot switch it off.
//
// Nothing that could carry a secret is copied out: not a run path, a plugin
// file, a hook directory, an environment entry, an argument or the text of a
// warning. What leaves is the name the manifest chose, an event, an origin, a
// tool selector, a timeout and a pair of flags.
func Observe(m *Manager, load LoadFacts) diag.HooksState {
	state := diag.HooksState{
		Known:          load.attempted,
		Enabled:        !load.disabled,
		Loaded:         load.attempted,
		LoadFailed:     load.failed,
		UserDir:        load.userDir,
		ProjectDir:     load.projectDir,
		Events:         eventVocabulary(),
		DefaultTimeout: defaultTimeout,
		Warnings:       load.warnings,
		Hooks:          []diag.HookFacts{},
	}
	live := liveEntries(m)
	seen := make(map[entryKey]bool, len(load.found))
	for _, manifest := range load.found {
		key := entryKey{name: manifest.name, event: manifest.event}
		seen[key] = true
		_, registered := live[key]
		state.Hooks = append(state.Hooks, manifest.observed(registered))
	}
	if m == nil {
		state.Revision = loadRevision(state)
		return state
	}

	// A manager can hold an entry no manifest accounts for: a built-in
	// policy, a hook a test registered, or one left from a load older than
	// this record. It is reported as itself rather than dropped — an entry
	// nobody can point at a source for is exactly what a reader wants to see.
	state.Managed = true
	for _, entry := range m.entries {
		if entry.Hook == nil {
			continue
		}
		key := entryKey{name: entry.Hook.Name(), event: entry.Kind}
		if seen[key] {
			continue
		}
		seen[key] = true
		state.Hooks = append(state.Hooks, diag.HookFacts{
			Name:       key.name,
			Event:      string(key.event),
			Origins:    []diag.HookOrigin{diag.HookOriginProcess},
			Tool:       toolSelector(key.event, ""),
			Timeout:    defaultTimeout,
			Registered: true,
			FailClosed: entry.FailClosed,
			Async:      entry.Async,
		})
	}
	state.Revision = loadRevision(state)
	return state
}

// observed renders one manifest for the harness view, together with whether
// the manager in force actually holds it. The two are separate answers: a
// manifest discovery kept is not automatically an entry, and an entry with no
// manifest behind it is not a configuration mistake.
func (f manifestFacts) observed(registered bool) diag.HookFacts {
	return diag.HookFacts{
		Name:       f.name,
		Event:      string(f.event),
		Origins:    observedOrigins(f.origins),
		Plugin:     f.plugin,
		Tool:       f.match,
		Timeout:    f.timeout,
		Discovered: true,
		Registered: registered,
		FailClosed: f.failClosed,
		Async:      f.async,
	}
}

// liveEntries indexes what the manager in force holds. The entry's own name
// is read — the one accessor the manager itself already uses when it logs —
// and no event is dispatched to any hook.
func liveEntries(m *Manager) map[entryKey]Entry {
	out := make(map[entryKey]Entry)
	if m == nil {
		return out
	}
	for _, entry := range m.entries {
		if entry.Hook == nil {
			continue
		}
		out[entryKey{name: entry.Hook.Name(), event: entry.Kind}] = entry
	}
	return out
}

// eventVocabulary is every event this build fans out, in the order the tool
// loop and the session lifecycle meet them. The harness view reports it
// rather than keeping a copy of its own, so the two cannot drift apart.
func eventVocabulary() []string {
	return []string{
		string(KindPreTool),
		string(KindPostTool),
		string(KindCommand),
		string(KindSessionStart),
		string(KindSessionBeforeSwitch),
		string(KindSessionShutdown),
		string(KindPostTurn),
	}
}

// observedOrigins maps discovery's own source labels onto the view's closed
// vocabulary. Discovery records only the two directories; anything else is
// dropped rather than guessed at.
func observedOrigins(sources []string) []diag.HookOrigin {
	out := make([]diag.HookOrigin, 0, len(sources))
	for _, source := range sources {
		switch source {
		case SourceUser:
			out = append(out, diag.HookOriginUser)
		case SourceProject:
			out = append(out, diag.HookOriginProject)
		default:
			continue
		}
	}
	return out
}

// toolSelector is the tool a hook applies to, in the form the matcher uses:
// every tool for a hook that names none, and nothing at all for a hook that
// is not on the tool loop — a slash command and a session hook are not
// selected by a tool name, and reporting one for them would invent a filter
// that does not exist.
func toolSelector(kind Kind, match string) string {
	if kind != KindPreTool && kind != KindPostTool {
		return ""
	}
	if match == "" || match == everyTool {
		return everyTool
	}
	return match
}

// hookTimeout is the budget one hook runs under: its own when the manifest
// names one, and the default when it does not — the same resolution
// NewCommandHook makes when it builds the hook.
func hookTimeout(declared time.Duration) time.Duration {
	if declared <= 0 {
		return defaultTimeout
	}
	return declared
}

// loadRevision fingerprints what this observation describes: how many hooks
// the load found, how many of them the manager in force holds, how many it
// holds that no manifest accounts for, and how many problems the load met.
// Nothing in the hooks package counts these — it is only a way to see that
// two snapshots taken across a reload are of two different states.
func loadRevision(state diag.HooksState) string {
	registered, extra := 0, 0
	for _, hook := range state.Hooks {
		if hook.Registered {
			registered++
		}
		if !hook.Discovered {
			extra++
		}
	}
	return fmt.Sprintf("d%d.r%d.p%d.w%d", len(state.Hooks)-extra, registered, extra, state.Warnings)
}
