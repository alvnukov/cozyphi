package diag

import (
	"context"
	"path/filepath"
	"strconv"
)

// Storage field keys. Each store takes its own namespace inside the storage
// category for the same reason MCP, LSP and hooks do inside integrations: a
// key is a stable address for Explain, and the four stores grow apart.
const (
	KeySessionsState    = "sessions.state"
	KeySessionsLocation = "sessions.location"
	KeySessionsEntries  = "sessions.entries"

	KeyMemoryState    = "memory.state"
	KeyMemoryLocation = "memory.location"
	KeyMemoryCorpus   = "memory.corpus"
	KeyMemoryCount    = "memory.count"
	KeyMemoryKinds    = "memory.kinds"
	KeyMemoryIndex    = "memory.index"

	KeyTasksState    = "tasks.state"
	KeyTasksLocation = "tasks.location"
	KeyTasksCount    = "tasks.count"

	KeyUsageState    = "usage.state"
	KeyUsageLocation = "usage.location"
	KeyUsageScopes   = "usage.scopes"
)

// storageKeys is the declared key set, in the order Collect returns them.
// One store at a time, in the order a session acquires them: the transcript
// it is writing, the corpus it reads from, the task notes of the repository
// it sits in, and the history shared by every workspace this user opens.
var storageKeys = []string{
	KeySessionsState,
	KeySessionsLocation,
	KeySessionsEntries,
	KeyMemoryState,
	KeyMemoryLocation,
	KeyMemoryCorpus,
	KeyMemoryCount,
	KeyMemoryKinds,
	KeyMemoryIndex,
	KeyTasksState,
	KeyTasksLocation,
	KeyTasksCount,
	KeyUsageState,
	KeyUsageLocation,
	KeyUsageScopes,
}

// Placeholder segments. A directory name that merely encodes another path —
// the working directory a session directory is named after, the checkout a
// memory corpus is named after — is a path in disguise: the home collapse
// the registry performs cannot reach it, because the separators are gone. So
// the segment is never reported, and the reader is told what fills it
// instead. Which working directory this is remains answerable, once, by
// runtime's own workspace.root.
const (
	storageWorkspaceSegment = "<workspace>"
	storageCorpusSegment    = "<corpus>"
	storageSessionSegment   = "<session>.jsonl"
	storageRepoAnchor       = "<repo>"
	storageMemorySegment    = "memory"
)

// StorageIdentity is what a store is keyed by — change that identity and the
// session is reading and writing a different store. It is the answer four
// otherwise similar questions turn on: two sessions in one workspace share
// three of these stores and not the fourth, and two worktrees of one
// repository share two of them and not the other two.
type StorageIdentity string

// StorageIdentity values, from the narrowest key to the widest.
const (
	// StorageIdentitySession means one conversation owns it. Another
	// session in the same workspace has its own.
	StorageIdentitySession StorageIdentity = "session"
	// StorageIdentityWorkspace means the working directory keys it, so two
	// linked worktrees of one repository do not share it.
	StorageIdentityWorkspace StorageIdentity = "workspace"
	// StorageIdentityRepository means the checkout keys it, so every
	// worktree made from that checkout reads and writes the same store.
	StorageIdentityRepository StorageIdentity = "repository"
	// StorageIdentityUser means one file for this user, shared by every
	// workspace and every session on this machine.
	StorageIdentityUser StorageIdentity = "user"
)

// StorageAnchors is where the layout puts each store, supplied by the owner
// of the layout rather than derived here. Only the parts that are fixed by
// the layout travel: a directory below one of these anchors may encode a
// path into its own name, and those are filled in with a placeholder before
// they reach a reader.
type StorageAnchors struct {
	// Known is false when nobody published the layout — the layer is not
	// wired, rather than wired and empty.
	Known bool
	// SessionBase is the directory every workspace's session files live
	// under. The directory below it is named after the working directory,
	// and is reported as a placeholder.
	SessionBase string
	// MemoryBase is the directory every memory corpus lives under. The
	// directory below it is named after the checkout the corpus belongs to,
	// and is reported as a placeholder for the same reason.
	MemoryBase string
	// UsageDir and UsageFile are the shared history's directory and file.
	// Neither encodes a path, so both are reported as they are.
	UsageDir  string
	UsageFile string
	// CorpusShared is whether the memory corpus is keyed by the repository
	// checkout rather than by this working directory, so every worktree made
	// from that checkout reads and writes one set of memories.
	CorpusShared bool
	// CorpusForeign is whether that checkout is a directory other than this
	// workspace's own — a session in a linked worktree reading and writing
	// the main checkout's memories.
	CorpusForeign bool
}

// sourceStorageNoLayout is what a locator says when nobody published the
// layout it would be composed from. It is a wiring gap rather than a store
// that is missing, and the two are worth telling apart.
var sourceStorageNoLayout = Source{
	Kind: SourceUnknown,
	Ref:  "nobody published the storage layout to this session, so no locator can be composed",
}

// storageReason states what this category answers and what it deliberately
// leaves out.
const storageReason = "where this session's state is kept and how much of it there is: whether the " +
	"conversation is being written to a file or ends with the process, which corpus its memories come " +
	"from and whether another checkout shares it, whether the repository has a task registry, and " +
	"whether the usage history is shared or in memory only. " +
	"Locations are anchors and placeholders — a directory named after a working directory or a " +
	"checkout is a path in disguise and is never spelled out — and what leaves besides them is states, " +
	"counts and identities. No message, no memory name, description or body, no task id, title or " +
	"note, no history key, and no error text. Nothing here opens or creates a store, rebuilds a memory " +
	"index, retrieves a memory, lists a session directory, reads a task note or scans any history: a " +
	"count that is not already in memory is reported as one that is not known, never as a zero"

// StorageDeps binds the storage collector to the owners of what this session
// keeps. Each is optional: a process without one reports that store
// unavailable rather than making the category fail.
type StorageDeps struct {
	// Anchors supplies the layout the stores sit in. It is a description of
	// where things go, computed from the process's own configuration; no
	// accessor here may create a directory or stat one.
	Anchors func() StorageAnchors
	// Sessions observes the session manager this session writes through. It
	// is read, never exercised: no accessor here may append an entry, flush
	// one, open or resume a session, or list the session directory.
	Sessions func() SessionStoreFacts
	// Memory observes the memory store through the owner's own projection,
	// on the same terms: no accessor here may rebuild the index, re-read the
	// corpus directory, parse a memory file, retrieve or record a use.
	Memory func() MemoryStoreFacts
	// Tasks observes the task registry, on the same terms again: no accessor
	// here may read a note, list the registry directory, discover a registry
	// that was never discovered, or create one.
	Tasks func() TaskStoreFacts
	// Usage observes the shared usage history: no accessor here may record a
	// use, prune an entry, re-read the file or write it back.
	Usage func() UsageStoreFacts
}

// storageCollector observes what this session keeps and where.
type storageCollector struct {
	deps StorageDeps
}

// NewStorageCollector builds the storage collector.
func NewStorageCollector(deps StorageDeps) Collector {
	return &storageCollector{deps: deps}
}

func (*storageCollector) Category() Category { return CategoryStorage }

// Status is answered from the declared key set alone: listing the catalog
// opens no store, reads no directory and stats no file.
func (*storageCollector) Status() Status {
	keys := make([]string, len(storageKeys))
	copy(keys, storageKeys)
	return Status{Availability: AvailabilityAvailable, Reason: storageReason, Keys: keys}
}

// Collect reads each owner once and derives every field from that one read,
// so the fields of one answer describe one moment rather than several.
func (c *storageCollector) Collect(_ context.Context) ([]Field, error) {
	anchors := callStorageAnchors(c.deps.Anchors)
	sessions := callSessionStoreFacts(c.deps.Sessions)
	memories := callMemoryStoreFacts(c.deps.Memory)
	notes := callTaskStoreFacts(c.deps.Tasks)
	history := callUsageStoreFacts(c.deps.Usage)
	return []Field{
		sessions.lifecycle(),
		sessions.location(anchors),
		sessions.entries(),
		memories.lifecycle(),
		memories.location(anchors),
		memories.corpus(anchors),
		memories.count(),
		memories.kinds(),
		memories.index(),
		notes.lifecycle(),
		notes.location(),
		notes.count(),
		history.lifecycle(),
		history.location(anchors),
		history.scopes(),
	}, nil
}

// identity states what a store is keyed by. It is the acting layer of every
// location field: a path says where the bytes are, and this says who else is
// reading and writing them.
func identity(id StorageIdentity, source Source) Observation {
	return Present(StringValue(string(id)), source)
}

// under composes a locator from an anchor and the segments below it. An
// anchor nobody published composes nothing: the answer is that the layout is
// not known, not a locator rooted at an empty string.
func under(anchor string, segments ...string) (string, bool) {
	if anchor == "" {
		return "", false
	}
	return filepath.Join(append([]string{anchor}, segments...)...), true
}

// counted renders a tally as "name=count", the same spelling the agents and
// hooks categories already use for one.
func counted(name string, count int) string {
	return name + "=" + strconv.Itoa(count)
}

// callStorageAnchors reads the optional accessor. A nil accessor is a wiring
// gap, and every locator it feeds reports unavailable rather than crashing
// the snapshot.
func callStorageAnchors(accessor func() StorageAnchors) StorageAnchors {
	if accessor == nil {
		return StorageAnchors{}
	}
	return accessor()
}

// callSessionStoreFacts reads the optional accessor, on the same terms.
func callSessionStoreFacts(accessor func() SessionStoreFacts) SessionStoreFacts {
	if accessor == nil {
		return SessionStoreFacts{}
	}
	return accessor()
}

// callMemoryStoreFacts reads the optional accessor, on the same terms.
func callMemoryStoreFacts(accessor func() MemoryStoreFacts) MemoryStoreFacts {
	if accessor == nil {
		return MemoryStoreFacts{}
	}
	return accessor()
}

// callTaskStoreFacts reads the optional accessor, on the same terms.
func callTaskStoreFacts(accessor func() TaskStoreFacts) TaskStoreFacts {
	if accessor == nil {
		return TaskStoreFacts{}
	}
	return accessor()
}

// callUsageStoreFacts reads the optional accessor, on the same terms.
func callUsageStoreFacts(accessor func() UsageStoreFacts) UsageStoreFacts {
	if accessor == nil {
		return UsageStoreFacts{}
	}
	return accessor()
}
