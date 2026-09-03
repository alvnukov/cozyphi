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
)

// Ledger is a session-owned, concurrency-safe set of editable file snapshots.
type Ledger struct {
	mu     sync.Mutex
	grants map[snapshot][]map[string]struct{}
	// order holds tracked snapshots oldest first, for eviction.
	order []snapshot
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
	l.grants[key] = append(l.grants[key], grant)
	if extra := len(l.grants[key]) - maxGrantsPerSnapshot; extra > 0 {
		l.grants[key] = l.grants[key][extra:]
	}
}

// Claim takes the authorization for the snapshot if it covers every requested
// anchor pair, and reports whether it did. A refused claim leaves the ledger
// untouched: a wrong tag or a mistyped anchor costs the model a retry, not a
// re-read of the file.
func (l *Ledger) Claim(path, tag string, anchors []string) (*Claim, bool) {
	if l == nil {
		return nil, false
	}
	key := snapshotKey(path, tag)
	l.mu.Lock()
	defer l.mu.Unlock()
	grants, ok := l.grants[key]
	if !ok || len(anchors) == 0 || len(anchors)%2 != 0 {
		return nil, false
	}
	normalized := make([]string, len(anchors))
	for i, anchor := range anchors {
		var valid bool
		normalized[i], valid = normalizeAnchor(anchor)
		if !valid {
			return nil, false
		}
	}
	for i := 0; i < len(normalized); i += 2 {
		if !coveredByOneGrant(grants, normalized[i], normalized[i+1]) {
			return nil, false
		}
	}
	// Every snapshot of this path goes with the claim: the edit is about to
	// rewrite the file, so anchors from any other read of it are dead too.
	claim := &Claim{path: key.path, removed: make(map[snapshot][]map[string]struct{})}
	for candidate, grant := range l.grants {
		if candidate.path == key.path {
			claim.removed[candidate] = grant
			l.forget(candidate)
		}
	}
	return claim, true
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
	}
}

// coveredByOneGrant reports whether a single returned snapshot carried both
// ends of the range: two reads must not be spliced into one edit.
func coveredByOneGrant(grants []map[string]struct{}, from, to string) bool {
	for _, grant := range grants {
		_, hasFrom := grant[from]
		_, hasTo := grant[to]
		if hasFrom && hasTo {
			return true
		}
	}
	return false
}

// track registers a snapshot in insertion order, evicting the oldest tracked
// one when the ledger is full. Callers hold the lock.
func (l *Ledger) track(key snapshot) {
	if _, exists := l.grants[key]; exists {
		return
	}
	for len(l.order) >= maxTrackedSnapshots {
		l.forget(l.order[0])
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
