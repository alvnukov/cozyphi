package memory

import (
	"fmt"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// OpenFacts is what the caller knew at the moment it opened a store and
// cannot recover afterwards. Open returns a nil store on failure, so a
// session that failed to open one holds nothing to ask — and "no store" and
// "a store that would not open" are different answers. Re-running Open to
// find out is not allowed: it creates the directory and rebuilds the index,
// which would make asking a question change what is stored.
type OpenFacts struct {
	// Attempted is whether an open happened at all. A zero OpenFacts is a
	// caller that never opened a corpus, and the whole layer reports
	// unavailable rather than "there are no memories".
	Attempted bool
	// Failed is whether the open returned an error.
	Failed bool
}

// ObserveOpen records the outcome of one Open call. It is called where the
// open is, because that is the only place both answers are known.
//
// The error is taken and dropped on purpose. It names the directory it
// failed on and can quote the path it could not resolve; Failed says that
// the open failed, and none of the error's text leaves this call.
func ObserveOpen(err error) OpenFacts {
	return OpenFacts{Attempted: true, Failed: err != nil}
}

// Observe reports the corpus's state for the harness view: whether a store
// is in force, whether an index is built over it, how many documents it
// holds by kind, and how long an edit made outside cozyphi may still be
// served from the cache.
//
// It reads the index the store already holds, under the store's own lock,
// and does nothing else. The directory is not listed, no file is opened or
// parsed, no index is built or refreshed, no recall runs and no use is
// recorded — a corpus whose index was never built is reported as one, which
// is the whole point of asking.
//
// Nothing that could carry a secret is copied out: not a memory's name,
// description, body, links, file name or path. What leaves is states, a
// duration and counts by kind.
func Observe(s *Store, open OpenFacts) diag.MemoryStoreFacts {
	facts := diag.MemoryStoreFacts{
		Known:          open.Attempted || s != nil,
		Attempted:      open.Attempted,
		Opened:         s != nil,
		Kinds:          []diag.MemoryKindFacts{},
		Vocabulary:     vocabulary(),
		VerifyInterval: verifyInterval,
	}
	if s == nil {
		return facts
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	facts.Invalidated = s.dirty
	if s.cached == nil {
		return facts
	}
	facts.Indexed = true
	facts.Count = len(s.cached.entries)
	counts := make(map[Kind]int, len(kindOrder))
	for _, entry := range s.cached.entries {
		counts[entry.Kind]++
		if entry.Pinned {
			facts.Pinned++
		}
	}
	for _, kind := range kindOrder {
		facts.Kinds = append(facts.Kinds, diag.MemoryKindFacts{Kind: string(kind), Count: counts[kind]})
	}
	facts.Revision = storeRevision(facts)
	return facts
}

// vocabulary is the kinds this build knows, in the order that groups the
// index and breaks recall ties, as the owner spells them.
func vocabulary() []string {
	out := make([]string, 0, len(kindOrder))
	for _, kind := range kindOrder {
		out = append(out, string(kind))
	}
	return out
}

// storeRevision fingerprints what this observation describes: how many
// documents the index holds, how many are pinned, and whether a turn has
// marked the index for re-checking. It is not a counter the store keeps —
// nothing in the store increments — so it is only a way to see that two
// snapshots taken across a written memory are of two different states.
func storeRevision(facts diag.MemoryStoreFacts) string {
	return fmt.Sprintf("d%d.p%d.i%t", facts.Count, facts.Pinned, facts.Invalidated)
}
