package diag

import "slices"

// LSP field keys. The language-server layer takes its own namespace inside
// the integrations category for the same reason MCP does: a key is a stable
// address for Explain, and the two services grow independently.
//
//nolint:gosec // G101: field addresses, not credentials; no value is one.
const (
	KeyLSPState       = "lsp.state"
	KeyLSPWorkspace   = "lsp.workspace"
	KeyLSPLanguages   = "lsp.languages"
	KeyLSPServers     = "lsp.servers"
	KeyLSPOperations  = "lsp.operations"
	KeyLSPRoots       = "lsp.roots"
	KeyLSPStart       = "lsp.start"
	KeyLSPDiagnostics = "lsp.diagnostics"
)

// LSPLifecycle is where the language-server subsystem stands. The values
// separate the things "no language server" could otherwise mean, which have
// nothing in common but the symptom: switched off in the configuration,
// never opened, opened and refused, shut down, open with nothing on this
// machine to run, and open with nothing asked of it yet.
type LSPLifecycle string

// LSPLifecycle values.
const (
	// LSPEnabled means the configuration the manager was opened with leaves
	// LSP on. It is the configured layer's answer and says nothing about
	// whether a manager exists.
	LSPEnabled LSPLifecycle = "enabled"
	// LSPDisabled means the configuration switched LSP off, so no manager
	// was built and nothing below it applies.
	LSPDisabled LSPLifecycle = "disabled"
	// LSPOpened means a manager exists for this workspace.
	LSPOpened LSPLifecycle = "opened"
	// LSPNotOpened means nobody built one — no open was attempted.
	LSPNotOpened LSPLifecycle = "not_opened"
	// LSPOpenFailed means opening one was attempted and refused: the
	// workspace or the configured command did not pass validation. What was
	// wrong with it is not reported; the rejection can quote a path.
	LSPOpenFailed LSPLifecycle = "open_failed"
	// LSPNotInstalled means the manager is open and no server it carries is
	// on this machine, so the first query will fail rather than start one.
	LSPNotInstalled LSPLifecycle = "not_installed"
	// LSPIdle means the manager is open and holds no live server. That is
	// where every session starts: a server is started by the first query
	// that needs one.
	LSPIdle LSPLifecycle = "idle"
	// LSPRunning means at least one server process is live.
	LSPRunning LSPLifecycle = "running"
	// LSPClosed means the manager was shut down; no server can be started
	// again without a new one.
	LSPClosed LSPLifecycle = "closed"
)

// LSPInstall is whether a server profile's executable is on this machine. It
// is answered by a lookup that runs nothing: an absolute command is checked
// where it stands, a bare name is looked for in the owner's own bin
// directory and then on PATH. Nothing is executed, downloaded or installed.
type LSPInstall string

// LSPInstall values.
const (
	// LSPInstallUnknown means nothing here knows. There is no command to
	// look for, and this view does not read the configuration file to find
	// one: the file has trust rules of its own, and an observation is not
	// allowed to be the second reader that skips them.
	LSPInstallUnknown LSPInstall = "unknown"
	// LSPInstallPresent means the executable was found.
	LSPInstallPresent LSPInstall = "present"
	// LSPInstallMissing means it was looked for and is not there.
	LSPInstallMissing LSPInstall = "missing"
)

// LSPStart is what became of the most recent attempt to start a server, in
// the owner's own closed vocabulary. It is never the attempt's message: a
// start error can name the resolved executable, echo the configured argv or
// quote the server's own output, and none of that may leave the owner.
type LSPStart string

// LSPStart values.
const (
	// LSPStartNotAttempted means nothing has asked for a server yet, which
	// is where every session begins.
	LSPStartNotAttempted LSPStart = "not_attempted"
	// LSPStartSucceeded means the most recent attempt produced a server.
	// The server may since have exited; what is running is a separate
	// question, answered by the roots.
	LSPStartSucceeded LSPStart = "succeeded"
	// LSPStartUnavailable means no process could be produced: the
	// executable was not found, or too many attempts failed too recently
	// and the next one is refused for a while.
	LSPStartUnavailable LSPStart = "unavailable"
	// LSPStartProtocol means a process started and the exchange with it did
	// not complete.
	LSPStartProtocol LSPStart = "protocol"
	// LSPStartClosed means the manager was shut down while starting.
	LSPStartClosed LSPStart = "closed"
	// LSPStartRejected means the attempt was refused before any process,
	// because the request was not one this build serves.
	LSPStartRejected LSPStart = "rejected"
)

// LSPServerFacts is what the harness may say about one language-server
// profile. It is an allowlist by construction: the build's own name for the
// profile, the language it serves, whether it is on this machine, and where
// it is running — with no member for the configured command, an argument, an
// environment entry, an initialization option, a settings value or the text
// of an error to land in.
type LSPServerFacts struct {
	// Name is the build's name for the profile, never the configured
	// command: a command can be an absolute path the owner chose, and a
	// path is not a name.
	Name string
	// Language is the language this profile serves. A language absent from
	// every profile is one this build has no server for at all.
	Language string
	// Installed is whether the executable was found on this machine.
	Installed LSPInstall
	// Roots are the workspace-relative directories with a live server right
	// now. The manager runs one server per Go root, so this is both what is
	// running and what it is running for.
	Roots []string
	// Operations are the queries this profile answers, as the build fixes
	// them. A live server gates each one again on what it advertises, which
	// is a question only a query answers.
	Operations []string
}

// LSPState is one observation of the language-server layer: whether the
// subsystem is on, whether a manager exists, where it works, one entry per
// server profile, and what became of the last attempt to start one.
type LSPState struct {
	// Known is false when nobody published an observation — the layer is
	// not wired, rather than wired and empty. Every field it feeds then
	// reports unavailable instead of an empty list that would read as "no
	// language server is configured".
	Known bool
	// Enabled is whether the configuration leaves LSP on.
	Enabled bool
	// Opened is whether a manager exists.
	Opened bool
	// OpenFailed is whether building one was attempted and refused.
	OpenFailed bool
	// Closed is whether the manager has been shut down.
	Closed bool
	// Workspace is the one directory this manager works in.
	Workspace string
	// Servers is one entry per server profile the manager carries.
	Servers []LSPServerFacts
	// LastStart is what became of the most recent start attempt, for any
	// root. The manager keeps one such record, so this belongs to the layer
	// rather than to a server.
	LastStart LSPStart
	// Freshness is the vocabulary a diagnostics answer draws its provenance
	// from, in precedence order, as the owner spells it. It is the owner's
	// list rather than a copy kept here, so the two cannot drift apart.
	Freshness []string
	// Revision fingerprints the state this observation describes, so two
	// snapshots taken across a first query or a shutdown are visibly of two
	// different states.
	Revision string
}

// Sources for the layers whose origin is the view's own reading rather than
// an owner's record.
var (
	sourceLSPOnByDefault = Source{
		Kind: SourceDefault,
		Ref:  "LSP is on unless the configuration the manager was opened with switches it off",
	}
	sourceLSPSwitchedOff = Source{
		Kind: SourceConfigFile,
		Ref:  "the configuration the manager was opened with switched LSP off",
	}
	sourceLSPOpen = Source{
		Kind: SourceComputed,
		Ref:  "whether a manager was built when this workspace opened",
	}
	sourceLSPManager = Source{
		Kind: SourceSession,
		Ref:  "the language-server manager this workspace holds",
	}
	sourceLSPOff = Source{
		Kind: SourceConfigFile,
		Ref:  "the configuration switched LSP off, so there is no manager for this to describe",
	}
	sourceLSPWorkspace = Source{
		Kind: SourceSession,
		Ref:  "the one directory this manager works in, resolved once before any server existed",
	}
	sourceLSPWorkspaceOwner = Source{
		Kind: SourceConfigFile,
		Ref:  "the workspace this session opened decides it; no configuration file does",
	}
	sourceLSPProfiles = Source{
		Kind: SourceBuild,
		Ref: "the server profiles this build carries; a language absent from this list is one cozyphi has no " +
			"server for at all, whatever is installed on the machine",
	}
	sourceLSPInstalled = Source{
		Kind: SourceComputed,
		Ref: "the profiles whose executable was found on this machine — an absolute command where it stands, a " +
			"bare name through the owner's bin directory and then PATH; the lookup runs and downloads nothing",
	}
	sourceLSPRunning = Source{
		Kind: SourceSession,
		Ref: "the profiles with a live server process right now; a server is started by the first query that " +
			"needs one, never by this",
	}
	sourceLSPOperationSet = Source{
		Kind: SourceBuild,
		Ref:  "the operations this build implements; no configuration adds to them or takes from them",
	}
	sourceLSPOperationsRunnable = Source{
		Kind: SourceComputed,
		Ref:  "the operations of the profiles found on this machine; nothing can run for a server that is not here",
	}
	sourceLSPOperationsGated = Source{
		Kind: SourceSession,
		Ref: "an operation is gated again on what the running server advertises at the moment it is asked, and " +
			"asking is the one thing this view does not do",
	}
	sourceLSPRootsDiscovered = Source{
		Kind: SourceComputed,
		Ref: "a root is chosen by the file a query asks about — nearest go.work, then go.mod, then the workspace " +
			"itself — so no source names the set in advance",
	}
	sourceLSPRootsLive = Source{
		Kind: SourceSession,
		Ref:  "the roots with a live server right now, relative to the workspace; one server runs per root",
	}
	sourceLSPStartOwner = Source{
		Kind: SourceComputed,
		Ref:  "whether a start succeeds is not configured anywhere; it is what happened",
	}
	sourceLSPStartLayer = Source{
		Kind: SourceSession,
		Ref:  "a start outcome is observed, not loaded: it belongs to the attempt that met it",
	}
	sourceLSPStartRecord = Source{
		Kind: SourceSession,
		Ref: "the manager's own record of its most recent start attempt, as its typed category and never as the " +
			"message — a start error can name the resolved executable or quote the server's own output",
	}
	sourceLSPFreshnessVocabulary = Source{
		Kind: SourceBuild,
		Ref:  "the provenances a diagnostics answer can carry, in precedence order",
	}
	sourceLSPFreshnessLayer = Source{
		Kind: SourceSession,
		Ref:  "nothing carries a freshness until a query asks for one; there is no standing value to load",
	}
	sourceLSPFreshnessUnasked = Source{
		Kind: SourceSession,
		Ref: "freshness is settled by one diagnostics query against the files it synchronizes, and this view runs " +
			"none: it is not promised here, and a past answer is not repeated as though it were current",
	}
)

// lifecycle separates the things "no language server" could mean: switched
// off, never opened, opened and refused, shut down, open with nothing on the
// machine to run, and open with nothing asked of it yet. They are fixed in
// entirely different places, and two of them are not problems at all.
func (s LSPState) lifecycle() Field {
	field := s.field(KeyLSPState, ApplyRestart)
	if !s.Known {
		return field
	}
	setting, source := LSPEnabled, sourceLSPOnByDefault
	if !s.Enabled {
		setting, source = LSPDisabled, sourceLSPSwitchedOff
	}
	field.Configured = Present(StringValue(string(setting)), source)
	field.Loaded = Present(StringValue(string(s.loadState())), sourceLSPOpen)
	field.Effective = Present(StringValue(string(s.liveState())), sourceLSPManager)
	return field
}

// workspace names the one directory the manager works in. Nothing in a
// configuration file sets it — the session's own workspace does — so the
// configured layer does not exist rather than reporting a default.
func (s LSPState) workspace() Field {
	field := s.field(KeyLSPWorkspace, ApplyRestart)
	if absent, ok := s.absent(); ok {
		return field.everyLayer(absent)
	}
	observed := Present(StringValue(s.Workspace), sourceLSPWorkspace)
	field.Configured = notApplicable(sourceLSPWorkspaceOwner)
	field.Loaded = observed
	field.Effective = observed
	return field
}

// languages is the direct answer to "what can this session ask about": the
// languages the build serves, the subset whose server is on this machine,
// and the subset with a server running. A language missing from the first
// list is not misconfigured — it is one cozyphi has no server for, and no
// amount of installing will change that here.
func (s LSPState) languages() Field {
	return s.shrinking(KeyLSPLanguages, func(f LSPServerFacts) string { return f.Language })
}

// servers is the same shrink one level down, naming the server profiles
// themselves: configured, installed, running. The gaps are the answer — a
// profile that is configured and not installed is a missing binary, and an
// installed one that is not running has simply not been needed yet.
func (s LSPState) servers() Field {
	return s.shrinking(KeyLSPServers, func(f LSPServerFacts) string { return f.Name })
}

// shrinking builds the configured/installed/running triple over one naming
// of the profiles, so the language view and the server view cannot disagree
// about which of them is which.
func (s LSPState) shrinking(key string, name func(LSPServerFacts) string) Field {
	field := s.field(key, ApplyRestart)
	if absent, ok := s.absent(); ok {
		return field.everyLayer(absent)
	}
	field.Configured = Present(ListValue(s.names(name, func(LSPServerFacts) bool { return true })), sourceLSPProfiles)
	field.Loaded = Present(ListValue(s.names(name, installed)), sourceLSPInstalled)
	field.Effective = Present(ListValue(s.names(name, running)), sourceLSPRunning)
	return field
}

// operations is what the model may ask a language server for. The build
// fixes the set, a server that is not on the machine can answer none of it,
// and what a running server will actually accept is settled per call
// against the capabilities it advertised — which is why the acting layer
// says it does not know rather than repeating the list as a promise.
func (s LSPState) operations() Field {
	field := s.field(KeyLSPOperations, ApplyRestart)
	if absent, ok := s.absent(); ok {
		return field.everyLayer(absent)
	}
	every := s.operationSet(func(LSPServerFacts) bool { return true })
	field.Configured = Present(ListValue(every), sourceLSPOperationSet)
	field.Loaded = Present(ListValue(s.operationSet(installed)), sourceLSPOperationsRunnable)
	field.Effective = unknown(sourceLSPOperationsGated)
	return field
}

// roots is what is running and what it is running for. Nothing configures
// the set and nothing loads it: a root is discovered from the file a query
// asks about, so the only layer that can have an answer is the acting one.
func (s LSPState) roots() Field {
	field := s.field(KeyLSPRoots, ApplyImmediate)
	if absent, ok := s.absent(); ok {
		return field.everyLayer(absent)
	}
	field.Configured = notApplicable(sourceLSPRootsDiscovered)
	field.Loaded = notApplicable(sourceLSPRootsDiscovered)
	field.Effective = Present(ListValue(s.liveRoots()), sourceLSPRootsLive)
	return field
}

// start is why there is no server when there is no server. The manager
// keeps one record of its most recent attempt, and this reports its
// category — never its message, which can name the resolved executable or
// carry the server's own words.
func (s LSPState) start() Field {
	field := s.field(KeyLSPStart, ApplyImmediate)
	if absent, ok := s.absent(); ok {
		return field.everyLayer(absent)
	}
	field.Configured = notApplicable(sourceLSPStartOwner)
	field.Loaded = notApplicable(sourceLSPStartLayer)
	field.Effective = Present(StringValue(string(s.lastStart())), sourceLSPStartRecord)
	return field
}

// diagnostics states what this view will not promise. A diagnostic's
// provenance is decided by the query that fetched it, against the files that
// query synchronized; there is no standing value, and reporting the last one
// as though it were current is exactly the mistake worth refusing. The
// vocabulary is reported so the refusal is legible rather than blank.
func (s LSPState) diagnostics() Field {
	field := s.field(KeyLSPDiagnostics, ApplyImmediate)
	if absent, ok := s.absent(); ok {
		return field.everyLayer(absent)
	}
	field.Configured = Present(ListValue(s.Freshness), sourceLSPFreshnessVocabulary)
	field.Loaded = notApplicable(sourceLSPFreshnessLayer)
	field.Effective = unknown(sourceLSPFreshnessUnasked)
	return field
}

// field is the shape every LSP field starts from: all three layers
// unavailable, so a layer nobody wired degrades into an honest answer rather
// than into an empty list that would read as "nothing is configured".
func (s LSPState) field(key string, apply Apply) Field {
	return Field{
		Key:        key,
		Configured: Unavailable(),
		Loaded:     Unavailable(),
		Effective:  Unavailable(),
		Apply:      apply,
		Scope:      ScopeWorkspace,
		Revision:   s.Revision,
	}
}

// absent reports whether the manager can answer a per-server question at
// all, and what to say when it cannot. The cases are not the same: a
// subsystem switched off has no manager to describe, while one that was
// never opened, or refused, cannot say what it would have carried. Off is
// tested first because switching LSP off is what stops a manager from being
// built — the missing manager is the consequence, and reporting it as the
// reason would hide an answer that is known for one that is not.
func (s LSPState) absent() (Observation, bool) {
	switch {
	case !s.Known:
		return Unavailable(), true
	case !s.Enabled:
		return notApplicable(sourceLSPOff), true
	case !s.Opened:
		return Unavailable(), true
	default:
		return Observation{}, false
	}
}

// loadState is what became of the configuration: a manager, nothing, or a
// refusal nobody may see the text of.
func (s LSPState) loadState() LSPLifecycle {
	switch {
	case s.OpenFailed:
		return LSPOpenFailed
	case s.Opened:
		return LSPOpened
	default:
		return LSPNotOpened
	}
}

// liveState is what LSP amounts to right now, answered in the order the
// answers stop mattering: off beats everything, a manager that never
// arrived beats what it would have held, a closed one beats what it holds,
// and a running server beats every reason there might not have been one.
func (s LSPState) liveState() LSPLifecycle {
	switch {
	case !s.Enabled:
		return LSPDisabled
	case s.OpenFailed:
		return LSPOpenFailed
	case !s.Opened:
		return LSPNotOpened
	case s.Closed:
		return LSPClosed
	case s.anyServer(running):
		return LSPRunning
	case !s.anyServer(installed):
		return LSPNotInstalled
	default:
		return LSPIdle
	}
}

// lastStart defaults an unset record to "nothing has been asked of it yet",
// so a manager nobody has queried is never mistaken for one whose start
// outcome went unrecorded.
func (s LSPState) lastStart() LSPStart {
	if s.LastStart == "" {
		return LSPStartNotAttempted
	}
	return s.LastStart
}

// installed reports whether a profile's executable is on this machine.
func installed(f LSPServerFacts) bool { return f.Installed == LSPInstallPresent }

// running reports whether a profile has a live server right now.
func running(f LSPServerFacts) bool { return len(f.Roots) > 0 }

// anyServer reports whether any profile matches keep.
func (s LSPState) anyServer(keep func(LSPServerFacts) bool) bool {
	return slices.ContainsFunc(s.Servers, keep)
}

// names collects one naming of the profiles matching keep, in the order the
// owner reported them and without repeating a name two profiles share.
func (s LSPState) names(name func(LSPServerFacts) string, keep func(LSPServerFacts) bool) []string {
	out := make([]string, 0, len(s.Servers))
	for _, facts := range s.Servers {
		value := name(facts)
		if !keep(facts) || value == "" || slices.Contains(out, value) {
			continue
		}
		out = append(out, value)
	}
	return out
}

// operationSet collects the operations of the profiles matching keep, in the
// order the owner reported them and without repeating one two profiles both
// answer.
func (s LSPState) operationSet(keep func(LSPServerFacts) bool) []string {
	out := make([]string, 0)
	for _, facts := range s.Servers {
		if !keep(facts) {
			continue
		}
		for _, op := range facts.Operations {
			if op == "" || slices.Contains(out, op) {
				continue
			}
			out = append(out, op)
		}
	}
	return out
}

// liveRoots collects every root with a live server, across profiles, in the
// order the owner reported them.
func (s LSPState) liveRoots() []string {
	out := make([]string, 0)
	for _, facts := range s.Servers {
		for _, root := range facts.Roots {
			if slices.Contains(out, root) {
				continue
			}
			out = append(out, root)
		}
	}
	return out
}

// callLSPState reads the optional accessor. A nil accessor is a wiring gap,
// and every layer it feeds reports unavailable rather than crashing the
// snapshot.
func callLSPState(accessor func() LSPState) LSPState {
	if accessor == nil {
		return LSPState{}
	}
	return accessor()
}
