package diag

import (
	"context"
	"strings"
)

// Runtime field keys. They are declared statically so the catalog can list
// them and Explain can validate a key without observing anything.
const (
	KeyVersion       = "version"
	KeyBuildCommit   = "build.commit"
	KeyBuildDate     = "build.date"
	KeyMode          = "mode"
	KeyDeveloperMode = "developer_mode"
	KeyWorkspaceRoot = "workspace.root"
	KeySessionID     = "session.id"
)

// developerModeFlag is the only way developer mode is ever switched on. It
// is named here as provenance, so an explanation says where the capability
// came from instead of leaving the reader to assume config or an env var
// could have done it.
const developerModeFlag = "--developer-mode"

// runtimeKeys is the declared key set, in the order Collect returns them.
var runtimeKeys = []string{
	KeyVersion,
	KeyBuildCommit,
	KeyBuildDate,
	KeyMode,
	KeyDeveloperMode,
	KeyWorkspaceRoot,
	KeySessionID,
}

// RuntimeDeps binds the runtime collector to the process it observes.
// Version, Mode and Enabled are fixed at construction because they are fixed
// for the process; Workspace and SessionID are accessors because they are
// not. A session id in particular does not exist until the engine is built,
// and an accessor read at Collect time reports "unavailable" until it does
// rather than a value invented at wiring time.
type RuntimeDeps struct {
	// Version is the build's version string (internal/version.Version).
	Version string
	// Mode is the process shape: "headless" or "tui".
	Mode string
	// Enabled is developer mode's state — always true where this collector
	// is wired today, carried explicitly so the field reports the real
	// answer rather than a constant.
	Enabled bool
	// Workspace returns the workspace root. The registry sanitizes it.
	Workspace func() string
	// SessionID returns the current session id, or "" before one exists.
	SessionID func() string
}

// runtimeCollector observes the process itself: what binary is running, in
// which shape, with which capability, over which workspace and session.
type runtimeCollector struct {
	deps RuntimeDeps
}

// NewRuntimeCollector builds the runtime collector.
func NewRuntimeCollector(deps RuntimeDeps) Collector {
	return &runtimeCollector{deps: deps}
}

func (*runtimeCollector) Category() Category { return CategoryRuntime }

// Status is answered from the declared key set alone — no accessor is
// called, so listing the catalog never reads the engine or the session.
func (*runtimeCollector) Status() Status {
	keys := make([]string, len(runtimeKeys))
	copy(keys, runtimeKeys)
	return Status{Availability: AvailabilityAvailable, Reason: "", Keys: keys}
}

// Collect reads the process now. Everything here is a field read or an
// accessor call: no process is spawned, no file is opened, nothing is
// reloaded.
func (c *runtimeCollector) Collect(_ context.Context) ([]Field, error) {
	return []Field{
		c.version(),
		buildFactUnavailable(KeyBuildCommit),
		buildFactUnavailable(KeyBuildDate),
		c.mode(),
		c.developerMode(),
		c.workspaceRoot(),
		c.sessionID(),
	}, nil
}

// version is compiled in, so its configured and loaded layers genuinely do
// not exist. Saying not_applicable is the honest answer; inventing a
// "configured version" would be worse than saying nothing.
func (c *runtimeCollector) version() Field {
	source := Source{Kind: SourceBuild, Ref: "internal/version.Version"}
	effective := Unavailable()
	if v := strings.TrimSpace(c.deps.Version); v != "" {
		effective = Present(StringValue(v), source)
	}
	return Field{
		Key:        KeyVersion,
		Configured: NotApplicable(SourceBuild),
		Loaded:     NotApplicable(SourceBuild),
		Effective:  effective,
		Apply:      ApplyRestart,
		Scope:      ScopeProcess,
	}
}

// buildFactUnavailable reports a build fact this binary does not carry.
// internal/version exports only Version: there is no commit or date to read,
// so the field exists and says unavailable rather than being omitted (which
// would read as "not a thing") or filled in from somewhere plausible.
func buildFactUnavailable(key string) Field {
	return Field{
		Key:        key,
		Configured: NotApplicable(SourceBuild),
		Loaded:     NotApplicable(SourceBuild),
		Effective:  Unavailable(),
		Apply:      ApplyRestart,
		Scope:      ScopeProcess,
	}
}

// mode is decided by which entry point started the process, so it is
// computed rather than configured.
func (c *runtimeCollector) mode() Field {
	source := Source{Kind: SourceComputed, Ref: "process entry point"}
	effective := Unavailable()
	if mode := strings.TrimSpace(c.deps.Mode); mode != "" {
		effective = Present(StringValue(mode), source)
	}
	return Field{
		Key:        KeyMode,
		Configured: NotApplicable(SourceComputed),
		Loaded:     NotApplicable(SourceComputed),
		Effective:  effective,
		Apply:      ApplyRestart,
		Scope:      ScopeProcess,
	}
}

// developerMode reports the capability that made this whole tool reachable.
// Its configured layer names the flag, because the flag is the only thing
// that can set it: config files, the environment and a resumed session
// cannot, and an explanation that showed a config key here would be a lie
// about the security model.
func (c *runtimeCollector) developerMode() Field {
	source := Source{Kind: SourceCLIFlag, Ref: developerModeFlag}
	configured := Unset(BoolValue(false), Source{Kind: SourceDefault, Ref: developerModeFlag})
	if c.deps.Enabled {
		configured = Present(BoolValue(true), source)
	} else {
		source = Source{Kind: SourceDefault, Ref: developerModeFlag}
	}
	return Field{
		Key:        KeyDeveloperMode,
		Configured: configured,
		Loaded:     Present(BoolValue(c.deps.Enabled), source),
		Effective:  Present(BoolValue(c.deps.Enabled), source),
		Apply:      ApplyRestart,
		Scope:      ScopeProcess,
	}
}

// workspaceRoot is the directory the session works in. It is a path, so the
// registry collapses the user's home to ~ before it leaves.
func (c *runtimeCollector) workspaceRoot() Field {
	source := Source{Kind: SourceComputed, Ref: "session workspace"}
	effective := Unavailable()
	if root := strings.TrimSpace(call(c.deps.Workspace)); root != "" {
		effective = Present(StringValue(root), source)
	}
	return Field{
		Key:        KeyWorkspaceRoot,
		Configured: NotApplicable(SourceComputed),
		Loaded:     NotApplicable(SourceComputed),
		Effective:  effective,
		Apply:      ApplyNewSession,
		Scope:      ScopeSession,
	}
}

// sessionID is read through its accessor at observation time. Before the
// engine exists there is no id, and the field says unavailable: a fabricated
// id would be indistinguishable from a real one to everything downstream.
func (c *runtimeCollector) sessionID() Field {
	source := Source{Kind: SourceSession, Ref: "session store"}
	effective := Unavailable()
	if id := strings.TrimSpace(call(c.deps.SessionID)); id != "" {
		effective = Present(StringValue(id), source)
	}
	return Field{
		Key:        KeySessionID,
		Configured: NotApplicable(SourceSession),
		Loaded:     NotApplicable(SourceSession),
		Effective:  effective,
		Apply:      ApplyNewSession,
		Scope:      ScopeSession,
	}
}

// call reads an optional accessor. A nil accessor is a wiring gap, and the
// field it feeds reports unavailable rather than crashing the snapshot.
func call(accessor func() string) string {
	if accessor == nil {
		return ""
	}
	return accessor()
}
