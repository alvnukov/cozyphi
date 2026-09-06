package diag

// SessionsLifecycle is where this session's transcript stands. The values
// separate what "nothing is on disk" could otherwise mean: a session opened
// never to be written at all, one bound to a file that no turn has reached
// yet, and one already written.
type SessionsLifecycle string

// SessionsLifecycle values.
const (
	// SessionsPersistent means the session was opened to be written to a
	// file. It is the configured layer's answer and says nothing about
	// whether anything has been written yet.
	SessionsPersistent SessionsLifecycle = "persistent"
	// SessionsEphemeral means the session was opened without persistence, so
	// it ends with the process and nothing below applies.
	SessionsEphemeral SessionsLifecycle = "ephemeral"
	// SessionsFile means a transcript file is bound to this session: a
	// persisting session takes its file the moment it opens, before any turn.
	SessionsFile SessionsLifecycle = "file"
	// SessionsMemoryOnly means no file is bound, so the conversation exists
	// only in this process.
	SessionsMemoryOnly SessionsLifecycle = "memory_only"
	// SessionsPending means a file is bound and nothing has reached it yet:
	// a session persists on its first assistant turn, and until then the
	// file is empty even though it is held.
	SessionsPending SessionsLifecycle = "pending"
	// SessionsPersisted means the transcript has been written at least once,
	// so the conversation would survive this process.
	SessionsPersisted SessionsLifecycle = "persisted"
)

// SessionStoreFacts is what the harness may say about the transcript this
// session writes: whether it is persisted at all, whether a file is bound,
// whether anything has reached it, and how many entries the manager holds.
//
// It is an allowlist by construction: there is no member for a message, a
// role, a title, a plan, a tool call, a session id or a path for anything to
// land in. Where the file is, is composed from the layout's own anchors.
type SessionStoreFacts struct {
	// Known is false when nobody published an observation — the layer is not
	// wired, rather than wired and empty. Every field it feeds then reports
	// unavailable instead of a zero that would read as "nothing is stored".
	Known bool
	// Persisting is whether this session was opened to be written to a file,
	// fixed by the entry point when the session was built.
	Persisting bool
	// Bound is whether a transcript file is held right now. A persisting
	// session acquires its file when it opens, so this is true well before
	// anything is written to it.
	Bound bool
	// Written is whether the transcript has reached the file at least once.
	// Until then the bound file is empty: a session flushes on its first
	// assistant turn.
	Written bool
	// Local is whether the bound file sits in this workspace's own session
	// directory. A session resumed from a path someone named sits elsewhere,
	// and the locator says so rather than pointing at a directory the file
	// is not in.
	Local bool
	// Entries is how many entries the manager holds: the session header and
	// everything appended after it. It is a count of what is in memory and
	// costs no read of the file.
	Entries int
	// Revision fingerprints the state this observation describes, so two
	// snapshots taken across a turn are visibly of two different states.
	Revision string
}

// Sources for the session transcript's layers.
var (
	sourceSessionsPersistence = Source{
		Kind: SourceSession,
		Ref: "whether this session was opened to be written to a file; the entry point fixes it when the " +
			"session is built and nothing in a running session changes it",
	}
	sourceSessionsBinding = Source{
		Kind: SourceSession,
		Ref: "whether a transcript file is held right now; a persisting session takes its file the moment " +
			"it opens, before any turn has reached it",
	}
	sourceSessionsWritten = Source{
		Kind: SourceSession,
		Ref: "whether the transcript has reached that file at least once — a session flushes on its first " +
			"assistant turn, so a held file with nothing behind it is still empty",
	}
	sourceSessionsBase = Source{
		Kind: SourceDefault,
		Ref:  "the directory every workspace's transcripts are kept under",
	}
	sourceSessionsFile = Source{
		Kind: SourceSession,
		Ref: "where this session's transcript is kept. The directory below the base is named after the " +
			"working directory and the file after the session, and neither is spelled out here: " +
			"runtime.workspace.root and runtime.session.id answer those once each",
	}
	sourceSessionsUnbound = Source{
		Kind: SourceSession,
		Ref:  "this session holds no transcript file, so it is kept nowhere and ends with the process",
	}
	sourceSessionsElsewhere = Source{
		Kind: SourceSession,
		Ref: "this session's transcript is not in this workspace's own directory: it was resumed from a " +
			"path someone named, or the directory was overridden on the command line. Either way the " +
			"path is arbitrary and is not reported here",
	}
	sourceSessionsIdentity = Source{
		Kind: SourceComputed,
		Ref: "one transcript belongs to one conversation: another session in this same workspace writes " +
			"its own file and never this one",
	}
	sourceSessionsUnplanned = Source{
		Kind: SourceComputed,
		Ref:  "nothing configures how long a conversation runs; an entry exists because a turn produced one",
	}
	sourceSessionsHeld = Source{
		Kind: SourceSession,
		Ref: "how many entries the manager holds: the session header and everything appended after it, " +
			"counted in memory without reading the file",
	}
	sourceSessionsUnwritten = Source{
		Kind: SourceSession,
		Ref: "nothing has been written to the file yet, so how much of the conversation would survive this " +
			"process is not a count this view can state",
	}
	sourceSessionsOnDisk = Source{
		Kind: SourceSession,
		Ref: "how much of the conversation the last flush put on the file. It is what the manager wrote, " +
			"not what a read of the file found: this view reads no transcript",
	}
)

// lifecycle is the transcript read three times over: whether this session is
// persisted at all, whether a file is bound, and whether anything has
// actually reached it. The last two differ on purpose — a held file with no
// turns behind it is the state a reader mistakes for a lost conversation.
func (s SessionStoreFacts) lifecycle() Field {
	field := s.field(KeySessionsState, ApplyNewSession, ScopeSession)
	if !s.Known {
		return field
	}
	field.Configured = Present(StringValue(string(s.persistence())), sourceSessionsPersistence)
	field.Loaded = Present(StringValue(string(s.binding())), sourceSessionsBinding)
	field.Effective = Present(StringValue(string(s.liveState())), sourceSessionsWritten)
	return field
}

// location is where transcripts go, where this one is, and who else reads
// it. The middle answer carries placeholders where a segment merely encodes
// another path: the directory is named after the working directory and the
// file after the session, and spelling either out would smuggle a path
// through a name the home collapse cannot reach.
func (s SessionStoreFacts) location(anchors StorageAnchors) Field {
	field := s.field(KeySessionsLocation, ApplyNewSession, ScopeSession)
	if !s.Known {
		return field
	}
	base, ok := under(anchors.SessionBase)
	if !anchors.Known || !ok {
		field.Configured = unknown(sourceStorageNoLayout)
	} else {
		field.Configured = Present(StringValue(base), sourceSessionsBase)
	}
	field.Effective = identity(StorageIdentitySession, sourceSessionsIdentity)
	switch {
	case !s.Bound:
		field.Loaded = Unset(NoValue(), sourceSessionsUnbound)
	case !s.Local:
		field.Loaded = unknown(sourceSessionsElsewhere)
	case !ok:
		field.Loaded = unknown(sourceStorageNoLayout)
	default:
		locator, _ := under(base, storageWorkspaceSegment, storageSessionSegment)
		field.Loaded = Present(StringValue(locator), sourceSessionsFile)
	}
	return field
}

// entries is how long the conversation is and how much of it would survive
// this process. A session that has not flushed reports the second as not
// known rather than as zero: nothing has been written, and zero would read
// as an empty transcript that had been.
func (s SessionStoreFacts) entries() Field {
	field := s.field(KeySessionsEntries, ApplyImmediate, ScopeSession)
	if !s.Known {
		return field
	}
	field.Configured = notApplicable(sourceSessionsUnplanned)
	field.Loaded = Present(IntValue(int64(s.Entries)), sourceSessionsHeld)
	if !s.Written {
		field.Effective = Unset(NoValue(), sourceSessionsUnwritten)
		return field
	}
	field.Effective = Present(IntValue(int64(s.Entries)), sourceSessionsOnDisk)
	return field
}

// persistence is what the session was opened for.
func (s SessionStoreFacts) persistence() SessionsLifecycle {
	if s.Persisting {
		return SessionsPersistent
	}
	return SessionsEphemeral
}

// binding is whether a file is held right now.
func (s SessionStoreFacts) binding() SessionsLifecycle {
	if s.Bound {
		return SessionsFile
	}
	return SessionsMemoryOnly
}

// liveState is what the transcript amounts to right now.
func (s SessionStoreFacts) liveState() SessionsLifecycle {
	switch {
	case !s.Bound:
		return SessionsEphemeral
	case !s.Written:
		return SessionsPending
	default:
		return SessionsPersisted
	}
}

// field is the shape every sessions field starts from: all three layers
// unavailable, so a layer nobody wired degrades into an honest answer rather
// than into a zero that would read as "nothing is stored".
func (s SessionStoreFacts) field(key string, apply Apply, scope Scope) Field {
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
