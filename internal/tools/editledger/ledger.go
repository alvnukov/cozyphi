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

// Ledger is a session-owned, concurrency-safe set of editable file snapshots.
type Ledger struct {
	mu     sync.Mutex
	grants map[snapshot][]map[string]struct{}
}

type snapshot struct {
	path string
	tag  string
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
	l.mu.Lock()
	defer l.mu.Unlock()
	grant := make(map[string]struct{}, len(anchors))
	for _, anchor := range anchors {
		if normalized, ok := normalizeAnchor(anchor); ok {
			grant[normalized] = struct{}{}
		}
	}
	l.grants[key] = append(l.grants[key], grant)
}

// Consume removes authorization for the snapshot and reports whether every
// requested anchor was in editable output returned during this session.
func (l *Ledger) Consume(path, tag string, anchors []string) bool {
	if l == nil {
		return false
	}
	key := snapshotKey(path, tag)
	l.mu.Lock()
	defer l.mu.Unlock()
	grants, ok := l.grants[key]
	for candidate := range l.grants {
		if candidate.path == key.path {
			delete(l.grants, candidate)
		}
	}
	if !ok || len(anchors) == 0 || len(anchors)%2 != 0 {
		return false
	}
	normalized := make([]string, len(anchors))
	for i, anchor := range anchors {
		var valid bool
		normalized[i], valid = normalizeAnchor(anchor)
		if !valid {
			return false
		}
	}
	for i := 0; i < len(normalized); i += 2 {
		authorized := false
		for _, grant := range grants {
			_, hasFrom := grant[normalized[i]]
			_, hasTo := grant[normalized[i+1]]
			if hasFrom && hasTo {
				authorized = true
				break
			}
		}
		if !authorized {
			return false
		}
	}
	return true
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
