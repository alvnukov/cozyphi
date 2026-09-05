// Package editledger tracks which hashline anchors the current tool session may edit.
package editledger

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/alvnukov/cozyphi/internal/util"
)

const (
	// A session reads far more files than it edits, and only a recent read is
	// a plausible base for one. Bounding both dimensions keeps a long session's
	// ledger small; an evicted read simply has to be repeated before editing.
	maxTrackedSnapshots  = 16
	maxGrantsPerSnapshot = 4
	// A refused edit is retried against the same tag within a few calls;
	// remembering why a snapshot died turns a bare no_capability into the
	// actual reason for exactly that retry window.
	maxRememberedDispositions = 8
)

// Code is the stable wire code for an edit result.
type Code string

const (
	ExactCode             Code = "exact"
	RebasedCode           Code = "rebased"
	TagChangedCode        Code = "tag_changed"
	ChangedDuringEditCode Code = "changed_during_edit"
	OverlapCode           Code = "overlap"
	RangeInvertedCode     Code = "range_inverted"
	OutOfBoundsCode       Code = "out_of_bounds"
)

// Outcome is the typed result of resolving an edit request against the
// ledger. Every refusal names one machine-stable reason; the tool boundary
// renders it for the model instead of reconstructing it from strings.
type Outcome int

const (
	// Granted means the request took the snapshot's authorization.
	Granted Outcome = iota
	// NoCapability: no current-session editable read of this path and TAG.
	NoCapability
	// SnapshotConsumed: an edit that applied already used this snapshot.
	SnapshotConsumed
	// SnapshotEvicted: the snapshot fell out of the bounded ledger.
	SnapshotEvicted
	// SnapshotSuperseded: a later write of this path replaced the snapshot.
	SnapshotSuperseded
	// AnchorNotObserved: an anchor was never returned by any grant of the snapshot.
	AnchorNotObserved
	// MixedGrants: a range's endpoints come from two different reads.
	MixedGrants
	// AmbiguousReanchor: a shifted anchor's hash matches multiple candidate
	// lines, or shifted endpoints disagree on the shift.
	AmbiguousReanchor
	// InvalidRef: an anchor reference is malformed.
	InvalidRef
)

// Code returns the stable wire code for the outcome.
func (o Outcome) Code() Code {
	switch o {
	case Granted:
		return "granted"
	case NoCapability:
		return "no_capability"
	case SnapshotConsumed:
		return "snapshot_consumed"
	case SnapshotEvicted:
		return "snapshot_evicted"
	case SnapshotSuperseded:
		return "snapshot_superseded"
	case AnchorNotObserved:
		return "anchor_not_observed"
	case MixedGrants:
		return "mixed_grants"
	case AmbiguousReanchor:
		return "ambiguous_reanchor"
	case InvalidRef:
		return "invalid_ref"
	default:
		return "unknown"
	}
}

// Refused reports whether the request was denied.
func (o Outcome) Refused() bool { return o != Granted }

// Ledger is a session-owned, concurrency-safe set of editable file snapshots.
type Ledger struct {
	mu     sync.Mutex
	grants map[snapshot][]grant
	// order holds tracked snapshots oldest first, for eviction.
	order orderedSet
	// dispositions remembers why recently dead snapshots died, so a retry
	// against a dead tag learns the reason instead of a bare no_capability.
	dispositions map[snapshot]Outcome
	deadOrder    orderedSet
}

// snapshot identifies one revision of one path. The key is the full
// util.Revision, never its 4-hex display tag: two different contents share a
// tag once every 65536 revisions, and authorization must not be transferable
// between them.
type snapshot struct {
	path string
	rev  util.Revision
}

// orderedSet preserves insertion order for a bounded set of snapshots.
// Callers choose why an evicted snapshot matters; the set only maintains
// membership and order.
type orderedSet struct {
	limit int
	keys  []snapshot
}

func newOrderedSet(limit int) orderedSet { return orderedSet{limit: limit} }

func (s *orderedSet) insert(key snapshot) bool {
	if slices.Contains(s.keys, key) {
		return false
	}
	s.keys = append(s.keys, key)
	return true
}

func (s *orderedSet) remove(key snapshot) {
	for i, stored := range s.keys {
		if stored == key {
			s.keys = append(s.keys[:i], s.keys[i+1:]...)
			return
		}
	}
}

func (s *orderedSet) evictOldest() snapshot {
	oldest := s.keys[0]
	s.keys = s.keys[1:]
	return oldest
}

func (s *orderedSet) overLimit() bool { return len(s.keys) > s.limit }

// tag is the display form the model sees for this snapshot.
func (s snapshot) tag() string { return s.rev.Tag() }

// grant is one read's observed anchors: line number → line hash. The line
// numbers are what re-anchoring needs; the hash is the provenance.
type grant map[int]string

// Ref is one range endpoint as the edit call claims it: the line number is a
// hint, the hash is provenance from the read that observed it.
type Ref struct {
	Line int
	Hash string
}

// Span is an inclusive range of line numbers. A deletion has To one less
// than From, naming the empty gap where its replacement landed.
type Span struct {
	From int
	To   int
}

// Resolution is the resolver's answer for a whole edits array. Lines holds the
// resolved (from, to) lines of every claimed pair — identical to the claimed
// lines on the exact path; Delta is the one shift every rebased endpoint
// moved by, 0 when nothing moved.
type Resolution struct {
	Outcome  Outcome
	Delta    int
	Lines    []Span
	Revision util.Revision
}

// Claim is one attempt's exclusive hold on a path's authorization. The grants
// are already out of the ledger, so a second attempt cannot use them; Release
// puts them back when the edit did not change the file.
type Claim struct {
	path    string
	rev     util.Revision
	removed map[snapshot][]grant
}

// Revision reports the file revision the claim authorizes. The edit compares
// it against the file it is about to rewrite, so a same-tag replacement of
// the file between the read and the edit cannot pass as the read revision.
func (c *Claim) Revision() util.Revision {
	if c == nil {
		return 0
	}
	return c.rev
}

var lineRefPattern = regexp.MustCompile(fmt.Sprintf(`^\s*[>+-]*\s*(\d+)\s*[:#]\s*([a-zA-Z]{%d})`, util.LineHashLen))

// New returns an empty authorization ledger.
func New() *Ledger {
	return &Ledger{
		grants:    make(map[snapshot][]grant),
		order:     newOrderedSet(maxTrackedSnapshots),
		deadOrder: newOrderedSet(maxRememberedDispositions),
	}
}

// Authorize adds the exact anchors returned for one file snapshot, named by
// its full revision.
func (l *Ledger) Authorize(path string, rev util.Revision, anchors []string) {
	if l == nil {
		return
	}
	key := snapshotKey(path, rev)
	observed := newGrant(anchors)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.authorize(key, observed)
}

// Supersede retires every live snapshot of the path and authorizes the
// revision a write just placed on disk. No earlier observation of the file
// describes its content any more, so the retired snapshots are remembered as
// SnapshotSuperseded: an edit that still quotes a pre-write TAG is refused by
// the ledger and pointed at the anchors the write result printed, instead of
// reaching the disk check and being told to read again. A rewrite of the same
// revision ends live with the fresh grant.
func (l *Ledger) Supersede(path string, next util.Revision, anchors []string) {
	if l == nil {
		return
	}
	key := snapshotKey(path, next)
	observed := newGrant(anchors)
	l.mu.Lock()
	defer l.mu.Unlock()
	for candidate := range l.grants {
		if candidate.path == key.path {
			l.forget(candidate)
			l.remember(candidate, SnapshotSuperseded)
		}
	}
	l.authorize(key, observed)
}

// authorize makes the snapshot live and appends one grant to it, keeping the
// most recent maxGrantsPerSnapshot. Callers hold the lock.
func (l *Ledger) authorize(key snapshot, observed grant) {
	l.admit(key)
	l.grants[key] = append(l.grants[key], observed)
	if extra := len(l.grants[key]) - maxGrantsPerSnapshot; extra > 0 {
		l.grants[key] = l.grants[key][extra:]
	}
}

// newGrant indexes the anchors a tool result printed by line; malformed
// anchors authorize nothing.
func newGrant(anchors []string) grant {
	observed := make(grant, len(anchors))
	for _, anchor := range anchors {
		if line, hash, ok := parseAnchor(anchor); ok {
			observed[line] = hash
		}
	}
	return observed
}

// Claim takes the authorization for the snapshot if it covers every requested
// range, and reports the typed resolution either way. Endpoints that all moved
// by one unambiguous shift re-anchor onto the observed lines (Delta); a
// refused claim leaves the ledger untouched: a wrong tag or a mistyped anchor
// costs the model a retry, not a re-read of the file.
func (l *Ledger) Claim(path, tag string, refs []Ref) (*Claim, Resolution) {
	if l == nil {
		return nil, Resolution{Outcome: NoCapability}
	}
	if len(refs) == 0 || len(refs)%2 != 0 {
		return nil, Resolution{Outcome: InvalidRef}
	}
	normalized := make([]Ref, len(refs))
	for i, ref := range refs {
		ref.Hash = strings.ToLower(strings.TrimSpace(ref.Hash))
		if ref.Line < 1 || !validHash(ref.Hash) {
			return nil, Resolution{Outcome: InvalidRef}
		}
		normalized[i] = ref
	}
	clean, wanted := filepath.Clean(path), normalizeTag(tag)
	l.mu.Lock()
	defer l.mu.Unlock()
	key, tracked := l.liveSnapshot(clean, wanted)
	if !tracked {
		if outcome, remembered := l.deadOutcome(clean, wanted); remembered {
			return nil, Resolution{Outcome: outcome}
		}
		return nil, Resolution{Outcome: NoCapability}
	}
	grants := l.grants[key]
	resolution := Resolution{Outcome: Granted, Revision: key.rev, Lines: make([]Span, 0, len(normalized)/2)}
	delta, rebasing := 0, false
	for i := 0; i < len(normalized); i += 2 {
		pair, outcome := resolvePair(grants, normalized[i], normalized[i+1])
		if outcome.Refused() {
			return nil, Resolution{Outcome: outcome}
		}
		for _, end := range &pair {
			if end.delta == 0 {
				// Exact endpoints may coexist with shifted ones.
				continue
			}
			if rebasing && end.delta != delta {
				return nil, Resolution{Outcome: AmbiguousReanchor}
			}
			delta, rebasing = end.delta, true
		}
		resolution.Lines = append(resolution.Lines, Span{From: pair[0].line, To: pair[1].line})
	}
	resolution.Delta = delta
	// Every snapshot of this path goes with the claim: the edit is about to
	// rewrite the file, so anchors from any other read of it are dead too.
	claim := &Claim{path: key.path, rev: key.rev, removed: make(map[snapshot][]grant)}
	for candidate, grant := range l.grants {
		if candidate.path == key.path {
			claim.removed[candidate] = grant
			l.forget(candidate)
			l.remember(candidate, SnapshotConsumed)
		}
	}
	return claim, resolution
}

// Release returns a claim's authorization to the ledger, for an attempt that
// left the file as it was. A claim taken by an edit that applied is settled
// by Commit instead: the old snapshots stay dead.
func (l *Ledger) Release(claim *Claim) {
	if l == nil || claim == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for key, grants := range claim.removed {
		l.admit(key)
		l.grants[key] = append(grants, l.grants[key]...)
		if extra := len(l.grants[key]) - maxGrantsPerSnapshot; extra > 0 {
			l.grants[key] = l.grants[key][extra:]
		}
	}
}

// Commit settles a claim whose edit rewrote the file: the claimed snapshots
// stay dead, and the successor grant for the exact new revision takes their
// place. Callers hand over the anchors of the changed region exactly as the
// edit result printed them, so what the model sees and what authorizes the
// next edit cannot diverge.
func (l *Ledger) Commit(claim *Claim, next util.Revision, anchors []string) {
	if l == nil || claim == nil {
		return
	}
	l.Authorize(claim.path, next, anchors)
}

// resolvedEndpoint is one endpoint after resolution: the observed line and
// the shift it took, 0 on the exact path.
type resolvedEndpoint struct {
	line  int
	delta int
}

// resolvePair anchors one (from, to) pair against the snapshot's grants.
// Both endpoints must resolve inside one and the same grant: a range spliced
// from two reads stays refused. A refusal names the typed reason: a hash no
// read ever returned, two reads spliced together, or an ambiguous shift.
func resolvePair(grants []grant, from, to Ref) ([2]resolvedEndpoint, Outcome) {
	seenFrom, seenTo, ambiguous := false, false, false
	for _, g := range grants {
		fromRes, fromOK, fromAmb := resolveEndpoint(g, from)
		toRes, toOK, toAmb := resolveEndpoint(g, to)
		if fromOK && toOK {
			return [2]resolvedEndpoint{fromRes, toRes}, Granted
		}
		seenFrom = seenFrom || fromOK || fromAmb
		seenTo = seenTo || toOK || toAmb
		ambiguous = ambiguous || fromAmb || toAmb
	}
	switch {
	case ambiguous:
		return [2]resolvedEndpoint{}, AmbiguousReanchor
	case !seenFrom || !seenTo:
		return [2]resolvedEndpoint{}, AnchorNotObserved
	default:
		return [2]resolvedEndpoint{}, MixedGrants
	}
}

// resolveEndpoint anchors one endpoint inside a single grant. The exact line
// wins first — today's behavior is the fast path; otherwise the endpoint is
// a shift only when its hash occurs at exactly one other observed line.
func resolveEndpoint(g grant, ref Ref) (resolvedEndpoint, bool, bool) {
	if hash, exact := g[ref.Line]; exact && hash == ref.Hash {
		return resolvedEndpoint{line: ref.Line}, true, false
	}
	candidates := make([]int, 0, 2)
	for line, hash := range g {
		if line != ref.Line && hash == ref.Hash {
			candidates = append(candidates, line)
			if len(candidates) > 1 {
				break
			}
		}
	}
	switch len(candidates) {
	case 1:
		return resolvedEndpoint{line: candidates[0], delta: candidates[0] - ref.Line}, true, false
	case 0:
		return resolvedEndpoint{}, false, false
	default:
		return resolvedEndpoint{}, false, true
	}
}

func validHash(hash string) bool {
	if len(hash) != util.LineHashLen {
		return false
	}
	for _, r := range hash {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return true
}

// track registers a snapshot in insertion order, evicting the oldest tracked
// one when the ledger is full. Callers hold the lock.
func (l *Ledger) track(key snapshot) {
	if !l.order.insert(key) {
		return
	}
	for l.order.overLimit() {
		oldest := l.order.evictOldest()
		// An evicted read is the second-most-likely retry target after a
		// consumed one; the model deserves the real reason there too.
		l.remember(oldest, SnapshotEvicted)
		l.forget(oldest)
	}
}

// forget drops a snapshot and its place in the eviction order. Callers hold
// the lock.
func (l *Ledger) forget(key snapshot) {
	delete(l.grants, key)
	l.order.remove(key)
}

// remember records why a snapshot died, bounded to the most recent ones.
// Callers hold the lock.
func (l *Ledger) remember(key snapshot, outcome Outcome) {
	if l.dispositions == nil {
		l.dispositions = make(map[snapshot]Outcome)
	}
	if l.deadOrder.insert(key) {
		for l.deadOrder.overLimit() {
			oldest := l.deadOrder.evictOldest()
			delete(l.dispositions, oldest)
		}
	}
	l.dispositions[key] = outcome
}

// revive drops a snapshot's dead disposition: the snapshot is live again.
// Callers hold the lock.
func (l *Ledger) revive(key snapshot) {
	if _, dead := l.dispositions[key]; !dead {
		return
	}
	delete(l.dispositions, key)
	l.deadOrder.remove(key)
}

func snapshotKey(path string, rev util.Revision) snapshot {
	return snapshot{path: filepath.Clean(path), rev: rev}
}

func normalizeTag(tag string) string { return strings.ToUpper(strings.TrimSpace(tag)) }

// liveSnapshot finds the tracked snapshot of the path whose display tag is
// the one the edit call quoted. Authorize keeps at most one such snapshot, so
// the first match is the only one. Callers hold the lock.
func (l *Ledger) liveSnapshot(clean, tag string) (snapshot, bool) {
	for _, key := range l.order.keys {
		if key.path == clean && key.tag() == tag {
			return key, true
		}
	}
	return snapshot{}, false
}

// deadOutcome reports why the most recently retired snapshot of the path with
// this display tag died. Newest first: a tag can have been reused by a later
// revision, and the latest reason is the one that fits the retry. Callers
// hold the lock.
func (l *Ledger) deadOutcome(clean, tag string) (Outcome, bool) {
	for _, key := range slices.Backward(l.deadOrder.keys) {
		if key.path == clean && key.tag() == tag {
			return l.dispositions[key], true
		}
	}
	return 0, false
}

// admit makes a snapshot live: it retires any other live snapshot of the same
// path that shows the same display tag, tracks the key and clears its dead
// disposition. The retirement keeps the invariant Claim relies on — at most
// one live snapshot per (path, tag) — so a tag the model quotes always names
// exactly one revision. The retired snapshot is remembered as SnapshotEvicted
// because that is what happened to it: the ledger dropped it, and the reason
// only ever surfaces once the newer snapshot is dead too. Callers hold the
// lock.
func (l *Ledger) admit(key snapshot) {
	for candidate := range l.grants {
		if candidate.path == key.path && candidate.rev != key.rev && candidate.tag() == key.tag() {
			l.forget(candidate)
			l.remember(candidate, SnapshotEvicted)
		}
	}
	l.track(key)
	// A live snapshot has no dead reason: a re-read of the same revision
	// revives it even if an edit had consumed it before.
	l.revive(key)
}

// parseAnchor extracts the line number and lowercased hash from a LINE#HASH
// anchor exactly as read/grep returned it.
func parseAnchor(ref string) (int, string, bool) {
	if strings.ContainsAny(ref, "\r\n") {
		return 0, "", false
	}
	match := lineRefPattern.FindStringSubmatch(ref)
	if match == nil {
		return 0, "", false
	}
	line, err := strconv.Atoi(match[1])
	if err != nil || line < 1 {
		return 0, "", false
	}
	return line, strings.ToLower(match[2]), true
}
