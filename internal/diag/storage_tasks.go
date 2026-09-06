package diag

// TasksLifecycle is where the task registry stands. The values separate what
// "no tasks" could otherwise mean: a repository with no registry at all, a
// discovery that failed, and a process that never looked.
type TasksLifecycle string

// TasksLifecycle values.
const (
	// TasksDiscovered means a registry was found and this session can work it.
	TasksDiscovered TasksLifecycle = "discovered"
	// TasksAbsent means the repository has no registry: neither the helper's
	// config names one nor the default directory exists. The tool that would
	// work it is then simply not offered.
	TasksAbsent TasksLifecycle = "absent"
	// TasksDiscoverFailed means discovery ran and failed — a config that
	// could not be read, or one naming a path outside the repository. It is
	// not the same as a repository that has no registry.
	TasksDiscoverFailed TasksLifecycle = "discover_failed"
	// TasksNotAttempted means nothing looked for a registry for this session.
	TasksNotAttempted TasksLifecycle = "not_attempted"
)

// TaskStoreFacts is what the harness may say about the task registry:
// whether one was found, where it sits relative to the repository, and what
// keys it.
//
// It is an allowlist by construction: there is no member for a task id,
// title, status, body, acceptance criterion, branch, worktree path or note
// file for anything to land in. The directory is repository-relative, which
// is a name inside the repository rather than a path on this machine.
type TaskStoreFacts struct {
	// Known is false when nobody published an observation — the layer is not
	// wired, rather than wired and empty.
	Known bool
	// Attempted is whether this process looked for a registry at all.
	Attempted bool
	// Failed is whether looking for one failed. A repository with no
	// registry is not a failure: it is an answer.
	Failed bool
	// Found is whether a registry is in force for this session.
	Found bool
	// Dir is where it sits, relative to the repository root — "obsidian-tasks"
	// by default, or whatever the helper's config named. Discovery refuses a
	// path outside the repository, so this never climbs out of it.
	Dir string
	// DefaultDir is where a registry lives when the config does not say, so
	// a reader can tell a default apart from a choice.
	DefaultDir string
	// Revision fingerprints the state this observation describes.
	Revision string
}

// Sources for the task registry's layers.
var (
	sourceTasksDiscovery = Source{
		Kind: SourceComputed,
		Ref: "a registry is discovered from the repository rather than configured for a session: the " +
			"helper's config names one, or the default directory exists",
	}
	sourceTasksFound = Source{
		Kind: SourceComputed,
		Ref: "whether looking for a registry found one; a repository with none is an answer rather than a " +
			"failure, and a failure to look is neither",
	}
	sourceTasksWorkable = Source{
		Kind: SourceComputed,
		Ref:  "whether this session has a registry to work at all",
	}
	sourceTasksDefaultDir = Source{
		Kind: SourceBuild,
		Ref:  "where a registry lives when the helper's config does not name one",
	}
	sourceTasksDir = Source{
		Kind: SourceConfigFile,
		Ref: "where this repository keeps its registry, relative to its own root — discovery refuses a " +
			"path that climbs out of the repository, so this stays inside it",
	}
	sourceTasksNoRegistry = Source{
		Kind: SourceComputed,
		Ref:  "no registry is in force for this session, so there is nowhere for it to be",
	}
	sourceTasksKeyedBy = Source{
		Kind: SourceComputed,
		Ref: "notes are tracked in the main checkout, so every worktree made from it works one registry — " +
			"a task started in a worktree is the same note the checkout reads",
	}
	sourceTasksUnplanned = Source{
		Kind: SourceComputed,
		Ref:  "nothing configures how many tasks there are; a note exists because someone wrote one",
	}
	sourceTasksUncounted = Source{
		Kind: SourceComputed,
		Ref: "the registry keeps no count in memory: every listing reads the directory and parses each " +
			"note, and this view reads neither — so the number is not stated rather than reported as none",
	}
)

// lifecycle is the registry read three times over: what would configure one,
// what looking for one found, and whether this session has one to work. The
// middle answer is where "there are no tasks" is taken apart: a repository
// with no registry, a config that could not be read, and a process that
// never looked are three different states with one symptom.
func (s TaskStoreFacts) lifecycle() Field {
	field := s.field(KeyTasksState, ApplyRestart, ScopeWorkspace)
	if !s.Known {
		return field
	}
	field.Configured = notApplicable(sourceTasksDiscovery)
	field.Loaded = Present(StringValue(string(s.liveState())), sourceTasksFound)
	field.Effective = Present(BoolValue(s.Found), sourceTasksWorkable)
	return field
}

// location is where a registry lives by default, where this one lives, and
// who else works it. All of it is repository-relative on purpose: the
// registry is tracked in the repository, so its address inside one is the
// whole of what a reader needs, and the path to the checkout is a question
// runtime.workspace.root already answers.
func (s TaskStoreFacts) location() Field {
	field := s.field(KeyTasksLocation, ApplyRestart, ScopeWorkspace)
	if !s.Known {
		return field
	}
	if s.DefaultDir != "" {
		def, _ := under(storageRepoAnchor, s.DefaultDir)
		field.Configured = Present(StringValue(def), sourceTasksDefaultDir)
	}
	field.Effective = identity(StorageIdentityRepository, sourceTasksKeyedBy)
	if !s.Found || s.Dir == "" {
		field.Loaded = Unset(NoValue(), sourceTasksNoRegistry)
		return field
	}
	dir, _ := under(storageRepoAnchor, s.Dir)
	field.Loaded = Present(StringValue(dir), sourceTasksDir)
	return field
}

// count is the one question this category refuses. The registry holds
// nothing in memory — every listing reads the directory and parses each note
// — so a number here would either be a read this view does not do or a zero
// that reads as an empty registry. It is reported as not known instead.
func (s TaskStoreFacts) count() Field {
	field := s.field(KeyTasksCount, ApplyImmediate, ScopeWorkspace)
	if !s.Known {
		return field
	}
	if !s.Found {
		return field.everyLayer(notApplicable(sourceTasksNoRegistry))
	}
	field.Configured = notApplicable(sourceTasksUnplanned)
	field.Loaded = unknown(sourceTasksUncounted)
	field.Effective = unknown(sourceTasksUncounted)
	return field
}

// liveState is what looking for a registry came to.
func (s TaskStoreFacts) liveState() TasksLifecycle {
	switch {
	case !s.Attempted:
		return TasksNotAttempted
	case s.Failed:
		return TasksDiscoverFailed
	case s.Found:
		return TasksDiscovered
	default:
		return TasksAbsent
	}
}

// field is the shape every tasks field starts from: all three layers
// unavailable, so a layer nobody wired degrades into an honest answer rather
// than into a zero that would read as "this repository has no tasks".
func (s TaskStoreFacts) field(key string, apply Apply, scope Scope) Field {
	return Field{
		Key:        key,
		Configured: Unavailable(),
		Loaded:     Unavailable(),
		Effective:  Unavailable(),
		Apply:      apply,
		Scope:      scope,
		Revision:   s.Revision,
	}
}
