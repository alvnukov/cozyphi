package diag

// UsageLifecycle is where the shared usage history stands. The values
// separate what "nothing is remembered" could otherwise mean: a process
// keeping history only in memory, a file that failed to load, and a file
// that loaded with nothing in it yet.
type UsageLifecycle string

// UsageLifecycle values.
const (
	// UsagePersistent means the history is backed by a file, so what one
	// session learns the next one starts with.
	UsagePersistent UsageLifecycle = "persistent"
	// UsageMemoryOnly means the store keeps history in memory alone: it
	// ranks this process's own choices and is gone when it exits.
	UsageMemoryOnly UsageLifecycle = "memory_only"
	// UsageOpen means the file was read, or was simply not there yet.
	UsageOpen UsageLifecycle = "open"
	// UsageOpenFailed means the file could not be read or could not be
	// parsed. The store still works — ranking degrades rather than failing —
	// and this is what says why the process started with no history.
	UsageOpenFailed UsageLifecycle = "open_failed"
	// UsageLoaded means the store holds history for at least one scope.
	UsageLoaded UsageLifecycle = "loaded"
	// UsageEmpty means it holds none — a fresh install, or a file that
	// failed to load.
	UsageEmpty UsageLifecycle = "empty"
)

// UsageScopeFacts is one row of the history tally: a scope and how many
// items it remembers. The items themselves never leave — a memory's key
// carries the directory it belongs to, so a count is the only safe answer.
type UsageScopeFacts struct {
	// Scope is the scope name, as the owner spells it.
	Scope string
	// Items is how many distinct items that scope remembers.
	Items int
}

// UsageStoreFacts is what the harness may say about the shared usage
// history: whether it is backed by a file, whether that file loaded, and how
// many items each scope remembers.
//
// It is an allowlist by construction: there is no member for an item key for
// anything to land in, which matters more here than elsewhere — a memory's
// key is built from the directory its corpus belongs to, and a model's from
// a name the user typed.
type UsageStoreFacts struct {
	// Known is false when nobody published an observation — the layer is not
	// wired, rather than wired and empty.
	Known bool
	// Persistent is whether the store is backed by a file. A store with no
	// path ranks this process's own choices and forgets them at exit.
	Persistent bool
	// OpenFailed is whether reading or parsing that file failed. The store
	// is usable either way: ranking degrades, it does not fail.
	OpenFailed bool
	// Scopes is one row per scope of the owner's own vocabulary, in the
	// owner's canonical order, so two snapshots of one state read the same.
	Scopes []UsageScopeFacts
	// Vocabulary is the scopes this build records, in that same order.
	Vocabulary []string
	// Items is how many items the store remembers in all.
	Items int
	// Revision fingerprints the state this observation describes.
	Revision string
}

// Sources for the usage history's layers.
var (
	sourceUsageBacking = Source{
		Kind: SourceComputed,
		Ref: "whether this history is backed by a file; a store with no path ranks this process's own " +
			"choices and forgets them when it exits",
	}
	sourceUsageOpen = Source{
		Kind: SourceComputed,
		Ref: "whether that file was read: a history that failed to load leaves the store usable and " +
			"empty, so ranking degrades rather than failing",
	}
	sourceUsageHeld = Source{
		Kind: SourceComputed,
		Ref:  "what the store holds right now, read from memory — asking re-reads no file and writes none",
	}
	sourceUsageDir = Source{
		Kind: SourceDefault,
		Ref:  "the owner-local directory this history is kept in",
	}
	sourceUsageFile = Source{
		Kind: SourceDefault,
		Ref:  "the file it is kept in; nothing here encodes another path, so it is reported as it is",
	}
	sourceUsageNoFile = Source{
		Kind: SourceComputed,
		Ref:  "this history is kept in memory alone, so it is stored nowhere",
	}
	sourceUsageKeyedBy = Source{
		Kind: SourceComputed,
		Ref: "one history for this user, shared by every workspace and every session on this machine — a " +
			"choice made in one project ranks the pickers of the next",
	}
	sourceUsageVocabulary = Source{
		Kind: SourceBuild,
		Ref:  "the scopes this build records; a use recorded under any other name is refused",
	}
	sourceUsageByScope = Source{
		Kind: SourceComputed,
		Ref: "how many items each scope remembers. Counts only — no item key leaves here, and one of " +
			"these scopes keys its items by the directory a memory corpus belongs to",
	}
	sourceUsageTotal = Source{
		Kind: SourceComputed,
		Ref: "how many items the history remembers in all; it is what decays with disuse and what a later " +
			"record prunes",
	}
)

// lifecycle is the history read three times over: whether a file backs it,
// whether that file loaded, and whether anything is remembered. The middle
// answer is the one a reader is otherwise missing — a history that failed to
// parse looks exactly like a fresh install from every picker in the process.
func (s UsageStoreFacts) lifecycle() Field {
	field := s.field(KeyUsageState, ApplyRestart, ScopeProcess)
	if !s.Known {
		return field
	}
	field.Configured = Present(StringValue(string(s.backing())), sourceUsageBacking)
	field.Loaded = Present(StringValue(string(s.openState())), sourceUsageOpen)
	field.Effective = Present(StringValue(string(s.liveState())), sourceUsageHeld)
	return field
}

// location is where the history is kept and who else reads it. Neither
// segment encodes another path, so unlike the transcript and the corpus this
// one is spelled out in full below the home collapse.
func (s UsageStoreFacts) location(anchors StorageAnchors) Field {
	field := s.field(KeyUsageLocation, ApplyRestart, ScopeProcess)
	if !s.Known {
		return field
	}
	dir, ok := under(anchors.UsageDir)
	if !anchors.Known || !ok {
		field.Configured = unknown(sourceStorageNoLayout)
	} else {
		field.Configured = Present(StringValue(dir), sourceUsageDir)
	}
	field.Effective = identity(StorageIdentityUser, sourceUsageKeyedBy)
	file, hasFile := under(anchors.UsageFile)
	switch {
	case !s.Persistent:
		field.Loaded = Unset(NoValue(), sourceUsageNoFile)
	case !anchors.Known || !hasFile:
		field.Loaded = unknown(sourceStorageNoLayout)
	default:
		field.Loaded = Present(StringValue(file), sourceUsageFile)
	}
	return field
}

// scopes is what this build records, how much of each is remembered, and how
// much there is in all. Only counts leave: one of these scopes keys its
// items by the directory a memory corpus belongs to, and another by names a
// user typed.
func (s UsageStoreFacts) scopes() Field {
	field := s.field(KeyUsageScopes, ApplyImmediate, ScopeProcess)
	if !s.Known {
		return field
	}
	field.Configured = Present(ListValue(s.Vocabulary), sourceUsageVocabulary)
	field.Loaded = Present(ListValue(s.tally()), sourceUsageByScope)
	field.Effective = Present(IntValue(int64(s.Items)), sourceUsageTotal)
	return field
}

// tally renders the scope counts as "scope=count", dropping the scopes with
// nothing in them: the vocabulary beside it already names those.
func (s UsageStoreFacts) tally() []string {
	out := make([]string, 0, len(s.Scopes))
	for _, scope := range s.Scopes {
		if scope.Items > 0 {
			out = append(out, counted(scope.Scope, scope.Items))
		}
	}
	return out
}

// backing is whether a file stands behind this history.
func (s UsageStoreFacts) backing() UsageLifecycle {
	if s.Persistent {
		return UsagePersistent
	}
	return UsageMemoryOnly
}

// openState is whether that file was read.
func (s UsageStoreFacts) openState() UsageLifecycle {
	if s.OpenFailed {
		return UsageOpenFailed
	}
	return UsageOpen
}

// liveState is whether anything is remembered right now.
func (s UsageStoreFacts) liveState() UsageLifecycle {
	if s.Items == 0 {
		return UsageEmpty
	}
	return UsageLoaded
}

// field is the shape every usage field starts from: all three layers
// unavailable, so a layer nobody wired degrades into an honest answer rather
// than into a zero that would read as "nothing has ever been used".
func (s UsageStoreFacts) field(key string, apply Apply, scope Scope) Field {
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
