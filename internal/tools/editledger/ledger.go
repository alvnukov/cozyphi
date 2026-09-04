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
	grants map[snapshot][]map[string]struct{}
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

// Claim is one attempt's exclusive hold on a path's authorization. The grants
// are already out of the ledger, so a second attempt cannot use them; Release
// puts them back when the edit did not change the file.
type Claim struct {
	path    string
	removed map[snapshot][]map[string]struct{}
}

var lineRefPattern = regexp.MustCompile(fmt.Sprintf(`^\s*[>+-]*\s*(\d+)\s*[:#]\s*([a-zA-Z]{%d})`, util.LineHashLen))

// New returns an empty authorization ledger.
func New() *Ledger {
	return &Ledger{grants: make(map[snapshot][]map[string]struct{})}
}

// Authorize adds the exact anchors returned for one file snapshot.
func (l *Ledger) Authorize(path, tag string, anchors []string) {
	if l == nil {
		return
	}
	key := snapshotKey(path, tag)
	grant := make(map[string]struct{}, len(anchors))
	for _, anchor := range anchors {
		if normalized, ok := normalizeAnchor(anchor); ok {
			grant[normalized] = struct{}{}
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
// anchor pair, and reports the typed outcome either way. A refused claim
// leaves the ledger untouched: a wrong tag or a mistyped anchor costs the
// model a retry, not a re-read of the file.
func (l *Ledger) Claim(path, tag string, anchors []string) (*Claim, Outcome) {
	if l == nil {
		return nil, NoCapability
	}
	if len(anchors) == 0 || len(anchors)%2 != 0 {
		return nil, InvalidRef
	}
	normalized := make([]string, len(anchors))
	for i, anchor := range anchors {
		var valid bool
		normalized[i], valid = normalizeAnchor(anchor)
		if !valid {
			return nil, InvalidRef
		}
	}
	key := snapshotKey(path, tag)
	l.mu.Lock()
	defer l.mu.Unlock()
	grants, tracked := l.grants[key]
	if !tracked {
		if outcome, remembered := l.dispositions[key]; remembered {
			return nil, outcome
		}
		return nil, NoCapability
	}
	for i := 0; i < len(normalized); i += 2 {
		if outcome, refused := coverage(grants, normalized[i], normalized[i+1]); refused {
			return nil, outcome
		}
	}
	// Every snapshot of this path goes with the claim: the edit is about to
	// rewrite the file, so anchors from any other read of it are dead too.
	claim := &Claim{path: key.path, removed: make(map[snapshot][]map[string]struct{})}
	for candidate, grant := range l.grants {
		if candidate.path == key.path {
			claim.removed[candidate] = grant
			l.forget(candidate)
			l.remember(candidate, SnapshotConsumed)
		}
	}
	return claim, Granted
}

// Release returns a claim's authorization to the ledger, for an attempt that
// left the file as it was. A claim taken by an edit that applied is simply
// never released: the file changed, so its anchors are dead.
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

// coverage reports why a pair is not covered by a single returned snapshot:
// an endpoint no read ever returned, or two endpoints from different reads.
func coverage(grants []map[string]struct{}, from, to string) (Outcome, bool) {
	seenFrom, seenTo := false, false
	for _, grant := range grants {
		_, hasFrom := grant[from]
		_, hasTo := grant[to]
		if hasFrom && hasTo {
			return Granted, false
		}
		seenFrom = seenFrom || hasFrom
		seenTo = seenTo || hasTo
	}
	if !seenFrom || !seenTo {
		return AnchorNotObserved, true
	}
	return MixedGrants, true
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

func normalizeAnchor(ref string) (string, bool) {
	if strings.ContainsAny(ref, "\r\n") {
		return "", false
	}
	match := lineRefPattern.FindStringSubmatch(ref)
	if match == nil {
		return "", false
	}
	line, err := strconv.Atoi(match[1])
	if err != nil || line < 1 {
		return "", false
	}
	return fmt.Sprintf("%d#%s", line, strings.ToLower(match[2])), true
}
