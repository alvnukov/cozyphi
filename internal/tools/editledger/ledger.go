// Package editledger tracks which hashline anchors the current tool session may edit.
package editledger

import (
	"fmt"
	"path/filepath"
	"regexp"
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
func (o Outcome) Code() string {
	switch o {
	case Granted:
		return "granted"
	case NoCapability:
		return "no_capability"
	case SnapshotConsumed:
		return "snapshot_consumed"
	case SnapshotEvicted:
		return "snapshot_evicted"
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
	order []snapshot
	// dispositions remembers why recently dead snapshots died, so a retry
	// against a dead tag learns the reason instead of a bare no_capability.
	dispositions map[snapshot]Outcome
	deadOrder    []snapshot
}

type snapshot struct {
	path string
	tag  string
}

// grant is one read's observed anchors: line number → line hash. The line
// numbers are what re-anchoring needs; the hash is the provenance.
type grant map[int]string

// Ref is one range endpoint as the edit call claims it: the line number is a
// hint, the hash is provenance from the read that observed it.
type Ref struct {
	Line int
	Hash string
}

// Resolution is the resolver's answer for a whole edits array. Lines holds the
// resolved (from, to) lines of every claimed pair — identical to the claimed
// lines on the exact path; Delta is the one shift every rebased endpoint
// moved by, 0 when nothing moved.
type Resolution struct {
	Outcome Outcome
	Delta   int
	Lines   [][2]int
}

// Claim is one attempt's exclusive hold on a path's authorization. The grants
// are already out of the ledger, so a second attempt cannot use them; Release
// puts them back when the edit did not change the file.
type Claim struct {
	path    string
	removed map[snapshot][]grant
}

var lineRefPattern = regexp.MustCompile(fmt.Sprintf(`^\s*[>+-]*\s*(\d+)\s*[:#]\s*([a-zA-Z]{%d})`, util.LineHashLen))

// New returns an empty authorization ledger.
func New() *Ledger {
	return &Ledger{grants: make(map[snapshot][]grant)}
}

// Authorize adds the exact anchors returned for one file snapshot.
func (l *Ledger) Authorize(path, tag string, anchors []string) {
	if l == nil {
		return
	}
	key := snapshotKey(path, tag)
	grant := make(grant, len(anchors))
	for _, anchor := range anchors {
		if line, hash, ok := parseAnchor(anchor); ok {
			grant[line] = hash
		}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.track(key)
	// A live snapshot has no dead reason: a re-read of the same revision
	// revives the tag even if an edit had consumed it before.
	l.revive(key)
	l.grants[key] = append(l.grants[key], grant)
	if extra := len(l.grants[key]) - maxGrantsPerSnapshot; extra > 0 {
		l.grants[key] = l.grants[key][extra:]
	}
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
	key := snapshotKey(path, tag)
	l.mu.Lock()
	defer l.mu.Unlock()
	grants, tracked := l.grants[key]
	if !tracked {
		if outcome, remembered := l.dispositions[key]; remembered {
			return nil, Resolution{Outcome: outcome}
		}
		return nil, Resolution{Outcome: NoCapability}
	}
	resolution := Resolution{Outcome: Granted, Lines: make([][2]int, 0, len(normalized)/2)}
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
		resolution.Lines = append(resolution.Lines, [2]int{pair[0].line, pair[1].line})
	}
	resolution.Delta = delta
	// Every snapshot of this path goes with the claim: the edit is about to
	// rewrite the file, so anchors from any other read of it are dead too.
	claim := &Claim{path: key.path, removed: make(map[snapshot][]grant)}
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
		l.track(key)
		l.grants[key] = append(grants, l.grants[key]...)
		if extra := len(l.grants[key]) - maxGrantsPerSnapshot; extra > 0 {
			l.grants[key] = l.grants[key][extra:]
		}
		// The failed attempt put the snapshot back, so it is live again.
		l.revive(key)
	}
}

// Commit settles a claim whose edit rewrote the file: the claimed snapshots
// stay dead, and the successor grant for the exact new revision takes their
// place. Callers hand over the anchors of the changed region exactly as the
// edit result printed them, so what the model sees and what authorizes the
// next edit cannot diverge.
func (l *Ledger) Commit(claim *Claim, newTag string, anchors []string) {
	if l == nil || claim == nil {
		return
	}
	l.Authorize(claim.path, newTag, anchors)
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
	if _, exists := l.grants[key]; exists {
		return
	}
	for len(l.order) >= maxTrackedSnapshots {
		oldest := l.order[0]
		// An evicted read is the second-most-likely retry target after a
		// consumed one; the model deserves the real reason there too.
		l.remember(oldest, SnapshotEvicted)
		l.forget(oldest)
	}
	l.order = append(l.order, key)
}

// forget drops a snapshot and its place in the eviction order. Callers hold
// the lock.
func (l *Ledger) forget(key snapshot) {
	delete(l.grants, key)
	for i, tracked := range l.order {
		if tracked == key {
			l.order = append(l.order[:i], l.order[i+1:]...)
			return
		}
	}
}

// remember records why a snapshot died, bounded to the most recent ones.
// Callers hold the lock.
func (l *Ledger) remember(key snapshot, outcome Outcome) {
	if l.dispositions == nil {
		l.dispositions = make(map[snapshot]Outcome)
	}
	if _, exists := l.dispositions[key]; !exists {
		l.deadOrder = append(l.deadOrder, key)
	}
	l.dispositions[key] = outcome
	for len(l.deadOrder) > maxRememberedDispositions {
		oldest := l.deadOrder[0]
		l.deadOrder = l.deadOrder[1:]
		delete(l.dispositions, oldest)
	}
}

// revive drops a snapshot's dead disposition: the snapshot is live again.
// Callers hold the lock.
func (l *Ledger) revive(key snapshot) {
	if _, dead := l.dispositions[key]; !dead {
		return
	}
	delete(l.dispositions, key)
	for i, tracked := range l.deadOrder {
		if tracked == key {
			l.deadOrder = append(l.deadOrder[:i], l.deadOrder[i+1:]...)
			return
		}
	}
}

func snapshotKey(path, tag string) snapshot {
	return snapshot{path: filepath.Clean(path), tag: strings.ToUpper(strings.TrimSpace(tag))}
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
