package diag

import "time"

// MemoryLifecycle is where the memory corpus stands. The values separate
// what "no memories" could otherwise mean: a process that never opened a
// store, one whose store failed to open, an open store with no index built,
// and an index built over an empty directory.
type MemoryLifecycle string

// MemoryLifecycle values.
const (
	// MemoryOpen means a store is in force for this session.
	MemoryOpen MemoryLifecycle = "open"
	// MemoryOpenFailed means opening the corpus failed, so this session
	// remembers nothing across turns. It is not the same as an empty corpus.
	MemoryOpenFailed MemoryLifecycle = "open_failed"
	// MemoryNotAttempted means nothing tried to open a corpus for this
	// session at all.
	MemoryNotAttempted MemoryLifecycle = "not_attempted"
	// MemoryIndexed means an index is built and holds at least one document.
	MemoryIndexed MemoryLifecycle = "indexed"
	// MemoryUnindexed means a store is open with no index built yet, so what
	// it holds is not a question this view can answer.
	MemoryUnindexed MemoryLifecycle = "unindexed"
	// MemoryEmpty means an index is built over a directory with nothing in
	// it — an answer, unlike the two above.
	MemoryEmpty MemoryLifecycle = "empty"
)

// MemoryCorpusBinding is how this workspace reaches its corpus. The three
// answers are what "another session edited my memories" turns on: a corpus
// keyed by this directory alone, one keyed by a checkout this workspace is,
// and one keyed by a checkout this workspace was merely made from.
type MemoryCorpusBinding string

// MemoryCorpusBinding values.
const (
	// MemoryCorpusOwn means this working directory keys the corpus, so
	// nothing else reads or writes it.
	MemoryCorpusOwn MemoryCorpusBinding = "own"
	// MemoryCorpusCheckout means the repository checkout keys the corpus and
	// this workspace is that checkout.
	MemoryCorpusCheckout MemoryCorpusBinding = "checkout"
	// MemoryCorpusWorktree means this workspace is a linked worktree reading
	// and writing the checkout's corpus, so a memory written here is a
	// memory the main checkout will read.
	MemoryCorpusWorktree MemoryCorpusBinding = "worktree"
)

// MemoryKindFacts is one row of the kind tally: a kind and how many
// documents carry it. There is no member for a name, a description or a
// body — a count per kind is the whole of what leaves.
type MemoryKindFacts struct {
	// Kind is the taxonomy label, as the owner spells it.
	Kind string
	// Count is how many indexed documents carry it.
	Count int
}

// MemoryStoreFacts is what the harness may say about the memory corpus:
// whether a store opened, whether an index is built over it, how many
// documents it holds by kind, and how long an edit made outside cozyphi may
// go unnoticed.
//
// It is an allowlist by construction: there is no member for a memory's
// name, description, body, links, path or file name for anything to land in.
// Everything here is a count, a state or a duration.
type MemoryStoreFacts struct {
	// Known is false when nobody published an observation — the layer is not
	// wired, rather than wired and empty. Every field it feeds then reports
	// unavailable instead of a zero that would read as "there are no
	// memories".
	Known bool
	// Attempted is whether this process tried to open a corpus at all.
	Attempted bool
	// Opened is whether a store is in force. False with Attempted true is an
	// open that failed, which is a different answer from an empty corpus.
	Opened bool
	// Indexed is whether an index is built. A store opens by building one,
	// so false here means the corpus could not be read.
	Indexed bool
	// Invalidated is whether the index has been marked for re-checking: the
	// agent wrote a memory this turn, and the next read will pick it up. The
	// counts below are the ones in force until then.
	Invalidated bool
	// Count is how many live documents the built index holds.
	Count int
	// Pinned is how many of them are pinned — the ones that reach every turn
	// whatever the budget.
	Pinned int
	// Kinds is one row per kind of the owner's own vocabulary, in the
	// owner's canonical order, so two snapshots of one state read the same.
	Kinds []MemoryKindFacts
	// Vocabulary is the kinds this build knows, in that same order.
	Vocabulary []string
	// VerifyInterval is how long an edit made outside cozyphi may still be
	// served from the cache before a full re-scan backs the cheap check.
	VerifyInterval time.Duration
	// Revision fingerprints the state this observation describes.
	Revision string
}

// Sources for the memory corpus's layers.
var (
	sourceMemoryUnconfigured = Source{
		Kind: SourceComputed,
		Ref: "nothing switches memory off: the corpus directory is created when the process starts, and a " +
			"session either got a store or failed to open one",
	}
	sourceMemoryOpen = Source{
		Kind: SourceComputed,
		Ref: "whether a memory store is in force for this session; an open that failed is not the same as " +
			"a corpus with nothing in it",
	}
	sourceMemoryHeld = Source{
		Kind: SourceComputed,
		Ref: "what the built index holds right now, read from the index already in memory — asking builds " +
			"nothing, re-reads no directory and parses no file",
	}
	sourceMemoryUnopened = Source{
		Kind: SourceComputed,
		Ref:  "no memory store is in force for this session, so there is nothing to count",
	}
	sourceMemoryUnindexed = Source{
		Kind: SourceComputed,
		Ref: "no index is built over this corpus, and building one would read the directory — so how much " +
			"it holds is not stated here rather than reported as none",
	}
	sourceMemoryBase = Source{
		Kind: SourceDefault,
		Ref:  "the directory every corpus is kept under, shared with Claude Code's own memory layout",
	}
	sourceMemoryDir = Source{
		Kind: SourceComputed,
		Ref: "where this session's corpus is kept. The directory below the base is named after the checkout " +
			"it belongs to and is not spelled out here: runtime.workspace.root answers that once",
	}
	sourceMemoryKeyedBy = Source{
		Kind: SourceComputed,
		Ref: "what keys the corpus: the repository checkout inside Git, so every worktree of it shares one " +
			"set of memories, and the working directory outside",
	}
	sourceMemoryBinding = Source{
		Kind: SourceComputed,
		Ref: "how this workspace reaches that corpus: its own, the checkout it is, or the checkout it was " +
			"made a worktree from",
	}
	sourceMemoryForeign = Source{
		Kind: SourceComputed,
		Ref: "whether the memories this session reads are kept for a directory other than its own — a " +
			"memory written here is then one the main checkout reads",
	}
	sourceMemoryUnplanned = Source{
		Kind: SourceComputed,
		Ref: "nothing configures how many memories there are; a document exists because someone or " +
			"something wrote a file",
	}
	sourceMemoryPinned = Source{
		Kind: SourceComputed,
		Ref: "how many of them are pinned: a pinned memory reaches every turn whatever the budget and " +
			"whatever the usage history says",
	}
	sourceMemoryVocabulary = Source{
		Kind: SourceBuild,
		Ref:  "the kinds this build knows; a document labeled anything else is filed under project",
	}
	sourceMemoryByKind = Source{
		Kind: SourceComputed,
		Ref: "the indexed documents by kind. Counts only — no name, description, body, link, file name or " +
			"path of any memory leaves here",
	}
	sourceMemoryKindUnselected = Source{
		Kind: SourceComputed,
		Ref: "no kind is selected or excluded at rest: the order above breaks ties inside one turn's " +
			"recall rather than filtering what is stored",
	}
	sourceMemoryVerifyWindow = Source{
		Kind: SourceDefault,
		Ref: "how long a memory edited outside cozyphi may still be served from the cache before a full " +
			"re-scan backs the cheap directory check",
	}
	sourceMemoryIndexBuilt = Source{
		Kind: SourceComputed,
		Ref:  "whether an index is built over this corpus at all",
	}
	sourceMemoryIndexCurrent = Source{
		Kind: SourceComputed,
		Ref: "whether that index is still the one in force: a turn that wrote a memory marks it for " +
			"re-checking, and the counts above are what stands until the next read",
	}
)

// lifecycle is the corpus read three times over: what could switch it off,
// whether a store is in force, and whether an index over it holds anything.
// The last answer keeps an unread corpus apart from an empty one — which is
// the difference between a broken directory and a fresh project.
func (s MemoryStoreFacts) lifecycle() Field {
	field := s.field(KeyMemoryState, ApplyRestart)
	if !s.Known {
		return field
	}
	field.Configured = notApplicable(sourceMemoryUnconfigured)
	field.Loaded = Present(StringValue(string(s.openState())), sourceMemoryOpen)
	field.Effective = Present(StringValue(string(s.liveState())), sourceMemoryHeld)
	return field
}

// location is where corpora go, where this one is, and who else reads it.
// The middle answer carries a placeholder for the directory named after the
// checkout: that name encodes a path, and the home collapse cannot reach
// inside it.
func (s MemoryStoreFacts) location(anchors StorageAnchors) Field {
	field := s.field(KeyMemoryLocation, ApplyRestart)
	if !s.Known {
		return field
	}
	base, ok := under(anchors.MemoryBase)
	if !anchors.Known || !ok {
		field.Configured = unknown(sourceStorageNoLayout)
	} else {
		field.Configured = Present(StringValue(base), sourceMemoryBase)
	}
	field.Effective = identity(anchors.corpusIdentity(), sourceMemoryKeyedBy)
	switch {
	case !s.Opened:
		field.Loaded = Unset(NoValue(), sourceMemoryUnopened)
	case !ok:
		field.Loaded = unknown(sourceStorageNoLayout)
	default:
		locator, _ := under(base, storageCorpusSegment, storageMemorySegment)
		field.Loaded = Present(StringValue(locator), sourceMemoryDir)
	}
	return field
}

// corpus is who else is reading these memories. A session in a linked
// worktree writes into the checkout's corpus, so a memory written here is
// one the main checkout will read — which is worth stating outright, because
// nothing else about the session says so.
func (s MemoryStoreFacts) corpus(anchors StorageAnchors) Field {
	field := s.field(KeyMemoryCorpus, ApplyRestart)
	if !s.Known {
		return field
	}
	if !anchors.Known {
		return field.everyLayer(unknown(sourceStorageNoLayout))
	}
	field.Configured = Present(StringValue(string(anchors.corpusIdentity())), sourceMemoryKeyedBy)
	field.Loaded = Present(StringValue(string(anchors.corpusBinding())), sourceMemoryBinding)
	field.Effective = Present(BoolValue(anchors.CorpusForeign), sourceMemoryForeign)
	return field
}

// count is how much this corpus holds and how much of it is pinned. A store
// with no index reports neither rather than reporting none: building the
// index is a read of the directory, and this view builds nothing.
func (s MemoryStoreFacts) count() Field {
	field := s.field(KeyMemoryCount, ApplyNextTurn)
	if absent, ok := s.absent(); ok {
		return field.everyLayer(absent)
	}
	if !s.Indexed {
		return field.everyLayer(unknown(sourceMemoryUnindexed))
	}
	field.Configured = notApplicable(sourceMemoryUnplanned)
	field.Loaded = Present(IntValue(int64(s.Count)), sourceMemoryHeld)
	field.Effective = Present(IntValue(int64(s.Pinned)), sourceMemoryPinned)
	return field
}

// kinds is the taxonomy this build knows and how the corpus is spread across
// it. A kind with nothing in it is left out of the tally and named in the
// vocabulary, so the two answers together say which kinds are empty.
func (s MemoryStoreFacts) kinds() Field {
	field := s.field(KeyMemoryKinds, ApplyNextTurn)
	if absent, ok := s.absent(); ok {
		return field.everyLayer(absent)
	}
	if !s.Indexed {
		return field.everyLayer(unknown(sourceMemoryUnindexed))
	}
	field.Configured = Present(ListValue(s.Vocabulary), sourceMemoryVocabulary)
	field.Loaded = Present(ListValue(s.tally()), sourceMemoryByKind)
	field.Effective = notApplicable(sourceMemoryKindUnselected)
	return field
}

// index is how current what was counted above is: the window an outside edit
// may hide in, whether an index is built at all, and whether the one built
// is still in force.
func (s MemoryStoreFacts) index() Field {
	field := s.field(KeyMemoryIndex, ApplyNextTurn)
	if absent, ok := s.absent(); ok {
		return field.everyLayer(absent)
	}
	// Unlike the counts, this field is the one that reports an unbuilt index
	// rather than being silenced by it.
	field.Configured = Present(DurationValue(s.VerifyInterval), sourceMemoryVerifyWindow)
	field.Loaded = Present(BoolValue(s.Indexed), sourceMemoryIndexBuilt)
	field.Effective = Present(BoolValue(s.Indexed && !s.Invalidated), sourceMemoryIndexCurrent)
	return field
}

// tally renders the kind counts as "kind=count", dropping the kinds with
// nothing in them: the vocabulary beside it already names those.
func (s MemoryStoreFacts) tally() []string {
	out := make([]string, 0, len(s.Kinds))
	for _, kind := range s.Kinds {
		if kind.Count > 0 {
			out = append(out, counted(kind.Kind, kind.Count))
		}
	}
	return out
}

// openState is whether a store is in force, and why not when it is not.
func (s MemoryStoreFacts) openState() MemoryLifecycle {
	switch {
	case s.Opened:
		return MemoryOpen
	case s.Attempted:
		return MemoryOpenFailed
	default:
		return MemoryNotAttempted
	}
}

// liveState is what the corpus amounts to right now. An unread corpus and an
// empty one are separated on purpose: only one of them is a fresh project.
func (s MemoryStoreFacts) liveState() MemoryLifecycle {
	switch {
	case !s.Opened:
		return s.openState()
	case !s.Indexed:
		return MemoryUnindexed
	case s.Count == 0:
		return MemoryEmpty
	default:
		return MemoryIndexed
	}
}

// field is the shape every memory field starts from: all three layers
// unavailable, so a layer nobody wired degrades into an honest answer rather
// than into a zero that would read as "there are no memories". The scope is
// the same for all of them — one corpus stands behind every session of this
// workspace — so it is fixed here rather than repeated at each call.
func (s MemoryStoreFacts) field(key string, apply Apply) Field {
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

// absent reports whether a question about what the corpus holds can be
// answered at all, and what to say when it cannot. A layer nobody wired
// knows nothing, and a session with no store has nothing to describe. A
// store whose index was never built is refused one field at a time instead:
// memory.index is the field that reports exactly that.
func (s MemoryStoreFacts) absent() (Observation, bool) {
	switch {
	case !s.Known:
		return Unavailable(), true
	case !s.Opened:
		return notApplicable(sourceMemoryUnopened), true
	default:
		return Observation{}, false
	}
}

// corpusIdentity is what keys the corpus: a checkout inside Git, where every
// worktree shares one set of memories, and the working directory outside.
func (a StorageAnchors) corpusIdentity() StorageIdentity {
	if a.CorpusShared {
		return StorageIdentityRepository
	}
	return StorageIdentityWorkspace
}

// corpusBinding is how this workspace reaches it.
func (a StorageAnchors) corpusBinding() MemoryCorpusBinding {
	switch {
	case !a.CorpusShared:
		return MemoryCorpusOwn
	case a.CorpusForeign:
		return MemoryCorpusWorktree
	default:
		return MemoryCorpusCheckout
	}
}
