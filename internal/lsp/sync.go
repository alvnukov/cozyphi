package lsp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf16"
	"unicode/utf8"
)

// The LSP textDocumentSync kinds this harness negotiates. Anything the server
// does not explicitly advertise resolves to none: the harness never invents
// document sync support.
const (
	syncNone        = 0
	syncFull        = 1
	syncIncremental = 2
)

// docSnapshot is the exact synchronized disk state one query operates on.
// Positions are converted against this text and diagnostics caches are keyed
// by its version and content hash.
type docSnapshot struct {
	uri     string
	path    string
	text    string
	hash    string
	version int
	changed bool // this sync sent didChange
	opened  bool // this sync sent didOpen
}

// docEntry is one synchronized document in a client generation. version is
// the last version sent on the wire; it only advances, never repeats, within
// the generation.
type docEntry struct {
	uri     string
	text    string
	hash    string
	version int
	// notified is true once didOpen reached the server in this generation.
	notified bool
	lastUse  int64
}

// docStore tracks synchronized documents per client generation with LRU
// eviction. Evicted documents are closed with didClose so the server never
// holds text the harness dropped.
type docStore struct {
	mu    sync.Mutex
	docs  map[string]*docEntry
	total int
	clock int64
}

func newDocStore() *docStore {
	return &docStore{docs: make(map[string]*docEntry)}
}

// syncDocument completes the write barrier for file: it reads the current
// disk snapshot, compares the content hash, and sends didOpen on first use or
// one didChange when the text changed — nothing when the hash is unchanged.
// It returns the exact snapshot every later position conversion must use.
func (c *client) syncDocument(ctx context.Context, file string) (docSnapshot, error) {
	raw, err := readSnapshot(file)
	if err != nil {
		return docSnapshot{}, err
	}
	if !utf8.Valid(raw) {
		return docSnapshot{}, newError(ErrInvalid, "%s is not valid UTF-8", file)
	}
	path := filepath.Clean(file)
	inside, err := contained(c.workspace, path)
	if err != nil {
		return docSnapshot{}, newError(ErrProtocol, "document sync: %v", err)
	}
	if !inside {
		return docSnapshot{}, newError(ErrInvalid, "%s is outside the workspace", path)
	}
	uri := uriFromPath(path)
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])
	kind := c.textSyncKind()

	c.docs.mu.Lock()
	defer c.docs.mu.Unlock()
	c.docs.clock++
	snap := docSnapshot{uri: uri, path: path, text: string(raw), hash: hash}

	entry := c.docs.docs[uri]
	if entry == nil {
		entry = &docEntry{uri: uri, text: string(raw), hash: hash, lastUse: c.docs.clock}
		c.docs.docs[uri] = entry
		c.docs.total += len(raw)
		if kind != syncNone {
			entry.version = 1
			entry.notified = true
			snap.version, snap.opened = 1, true
			if err := c.notify(ctx, "textDocument/didOpen", map[string]any{
				"textDocument": map[string]any{
					"uri":        uri,
					"languageId": "go",
					"version":    1,
					"text":       entry.text,
				},
			}); err != nil {
				delete(c.docs.docs, uri)
				c.docs.total -= len(raw)
				return docSnapshot{}, err
			}
		}
		c.docs.evictLocked(ctx, c, uri)
		return snap, nil
	}

	entry.lastUse = c.docs.clock
	if entry.hash == hash {
		// Unchanged content: no notification, reuse the synced state.
		snap.version, snap.text = entry.version, entry.text
		return snap, nil
	}

	old := entry.text
	c.docs.total += len(raw) - len(old)
	entry.text, entry.hash = string(raw), hash
	switch kind {
	case syncFull:
		entry.version++
		snap.version, snap.changed = entry.version, true
		if err := c.notify(ctx, "textDocument/didChange", map[string]any{
			"textDocument":   map[string]any{"uri": uri, "version": entry.version},
			"contentChanges": []any{map[string]any{"text": entry.text}},
		}); err != nil {
			return docSnapshot{}, err
		}
	case syncIncremental:
		entry.version++
		snap.version, snap.changed = entry.version, true
		// One replacement range covering the whole old snapshot in UTF-16
		// coordinates: the server applies it by swapping the text entirely.
		if err := c.notify(ctx, "textDocument/didChange", map[string]any{
			"textDocument": map[string]any{"uri": uri, "version": entry.version},
			"contentChanges": []any{map[string]any{
				"range": wireRange{Start: wirePosition{Line: 0, Character: 0}, End: endOfText(old)},
				"text":  entry.text,
			}},
		}); err != nil {
			return docSnapshot{}, err
		}
	}
	c.docs.evictLocked(ctx, c, uri)
	return snap, nil
}

// currentVersion reports the last version sent for uri, if the document is
// open and notified in this generation.
func (s *docStore) currentVersion(uri string) (int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.docs[uri]
	if e == nil || !e.notified {
		return 0, false
	}
	return e.version, true
}

// evictLocked drops least-recently-used documents until both caps hold. The
// just-synced document (keep) is never evicted; notified victims are closed
// with didClose.
func (s *docStore) evictLocked(ctx context.Context, c *client, keep string) {
	for len(s.docs) > MaxOpenDocuments || s.total > MaxOpenTextBytes {
		var victim *docEntry
		for uri, e := range s.docs {
			if uri == keep {
				continue
			}
			if victim == nil || e.lastUse < victim.lastUse {
				victim = e
			}
		}
		if victim == nil {
			return // only the current document remains
		}
		delete(s.docs, victim.uri)
		s.total -= len(victim.text)
		if victim.notified {
			_ = c.notify(ctx, "textDocument/didClose", map[string]any{
				"textDocument": map[string]any{"uri": victim.uri},
			})
		}
	}
}

// endOfText returns the UTF-16 position just past the last character of text:
// the line index counts every newline and the character is the UTF-16 length
// of the segment after the final newline.
func endOfText(text string) wirePosition {
	line := strings.Count(text, "\n")
	tail := text
	if i := strings.LastIndexByte(text, '\n'); i >= 0 {
		tail = text[i+1:]
	}
	units := 0
	for _, r := range tail {
		if n := utf16.RuneLen(r); n > 0 {
			units += n
		}
	}
	return wirePosition{Line: line, Character: units}
}

// textSyncKind resolves the negotiated textDocumentSync capability: a bare
// kind number or an options object. Missing, none, or openClose-less forms
// mean no text notifications at all.
func (c *client) textSyncKind() int {
	c.capsMu.Lock()
	v := c.caps["textDocumentSync"]
	c.capsMu.Unlock()
	switch t := v.(type) {
	case float64:
		// The number form implies didOpen/didClose support for full and
		// incremental kinds.
		return syncKindFrom(int(t), t == syncFull || t == syncIncremental)
	case map[string]any:
		openClose, _ := t["openClose"].(bool)
		kind, _ := t["change"].(float64)
		return syncKindFrom(int(kind), openClose)
	default:
		return syncNone
	}
}

func syncKindFrom(kind int, openClose bool) int {
	if !openClose || (kind != syncFull && kind != syncIncremental) {
		return syncNone
	}
	return kind
}
