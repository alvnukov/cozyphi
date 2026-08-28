package session

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alvnukov/cozyphi/internal/llm"
)

// SessionMeta is a lightweight listing row for persisted sessions.
type SessionMeta struct {
	ID        string
	File      string
	Timestamp string
	Cwd       string
	Mtime     time.Time
	Preview   string // truncated last user text
}

// ListSessions returns session files under dir, newest mtime first.
// Callers should pass a per-cwd directory (e.g. project.SessionDir()), not the
// global session base, so listings stay scoped to the current project.
func ListSessions(dir string) ([]SessionMeta, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	out := make([]SessionMeta, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		meta, err := readSessionMeta(path, e)
		if err != nil {
			continue // skip unreadable / malformed files in listings
		}
		out = append(out, meta)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Mtime.After(out[j].Mtime)
	})
	return out, nil
}

func readSessionMeta(path string, e os.DirEntry) (SessionMeta, error) {
	info, err := e.Info()
	if err != nil {
		return SessionMeta{}, err
	}
	f, err := os.Open(path)
	if err != nil {
		return SessionMeta{}, err
	}
	defer f.Close()

	meta := SessionMeta{
		File:  path,
		Mtime: info.ModTime(),
	}
	sc := bufio.NewScanner(f)
	// Session files can grow; allow large lines for tool payloads.
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := sc.Bytes()
		if strings.TrimSpace(string(line)) == "" {
			continue
		}
		entry, err := decodeEntryLine(line, lineNo)
		if err != nil {
			if meta.ID == "" {
				return SessionMeta{}, err
			}
			break
		}
		switch e := entry.(type) {
		case SessionHeader:
			meta.ID = e.ID
			meta.Timestamp = e.Timestamp
			meta.Cwd = e.Cwd
		case SessionMessageEntry:
			if e.Message.Role == llm.RoleUser && strings.TrimSpace(e.Message.Content) != "" {
				meta.Preview = truncatePreview(e.Message.Content, 72)
			}
		}
	}
	if meta.ID != "" {
		return meta, nil
	}
	// Fall back to filename id when header missing.
	id, ok := sessionIDFromFilename(filepath.Base(path))
	if !ok {
		return SessionMeta{}, fmt.Errorf("session: no header in %s", path)
	}
	meta.ID = id
	return meta, nil
}

func truncatePreview(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// FindSessionFile resolves id to a unique jsonl path under dir.
// id may be exact or a unique prefix of the session id.
func FindSessionFile(dir, id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", errors.New("session: empty id")
	}
	if filepath.IsAbs(id) || strings.HasSuffix(id, ".jsonl") {
		if _, err := os.Stat(id); err != nil {
			return "", fmt.Errorf("session: file %q: %w", id, err)
		}
		return id, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}

	var exact, prefix []string
	var exactNames, prefixIDs []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		sid, ok := sessionIDFromFilename(e.Name())
		if !ok {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if sid == id {
			exact = append(exact, path)
			exactNames = append(exactNames, strings.TrimSuffix(e.Name(), ".jsonl"))
		} else if strings.HasPrefix(sid, id) {
			prefix = append(prefix, path)
			prefixIDs = append(prefixIDs, sid)
		}
	}
	if len(exact) == 1 {
		return exact[0], nil
	}
	if len(exact) > 1 {
		// Same id in several files: names, not ids, tell them apart.
		return "", fmt.Errorf("session: ambiguous id %q (%d matches: %s)", id, len(exact), listCandidates(exactNames))
	}
	if len(prefix) == 1 {
		return prefix[0], nil
	}
	if len(prefix) > 1 {
		return "", fmt.Errorf(
			"session: ambiguous id prefix %q (%d matches: %s)",
			id,
			len(prefix),
			listCandidates(prefixIDs),
		)
	}
	return "", fmt.Errorf("session: id %q not found in %s", id, dir)
}

// maxCandidates caps how many ids an ambiguous-match error lists, so a short
// prefix over a large session directory cannot flood the terminal.
const maxCandidates = 5

func listCandidates(vals []string) string {
	shown := vals
	if len(vals) > maxCandidates {
		return fmt.Sprintf("%s + %d more", strings.Join(vals[:maxCandidates], ", "), len(vals)-maxCandidates)
	}
	return strings.Join(shown, ", ")
}

func sessionIDFromFilename(name string) (string, bool) {
	base := strings.TrimSuffix(name, ".jsonl")
	i := strings.IndexByte(base, '_')
	if i < 0 || i+1 >= len(base) {
		return "", false
	}
	return base[i+1:], true
}

// maxEntryLine matches the historical scan cap: session lines can be large
// (tool payloads) but not unbounded.
const maxEntryLine = 8 * 1024 * 1024

// readEntryLine returns the next line without its '\n' and whether the line
// was newline-terminated. An unterminated final line is the signature of a
// crash mid-append; a terminated but undecodable line is deliberate
// corruption and fails the load.
func readEntryLine(r *bufio.Reader) (line []byte, terminated bool, err error) {
	raw, err := r.ReadBytes('\n')
	if len(raw) > 0 && raw[len(raw)-1] == '\n' {
		raw = raw[:len(raw)-1]
		terminated = true
	}
	if len(raw) > maxEntryLine {
		return nil, false, fmt.Errorf("session: entry line exceeds %d bytes", maxEntryLine)
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, false, err
	}
	return raw, terminated, nil
}

// OpenSession loads a JSONL session file and returns a Manager ready to append.
func OpenSession(path string) (*Manager, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := bufio.NewReaderSize(f, 64*1024)

	var (
		entries         []MessageEntry
		byIDs           = make(map[string]MessageEntry, 64)
		header          *SessionHeader
		leafID          *string
		plan            Plan
		hasAssistantMsg bool
		lineNo          int
		goodBytes       int64
	)

	for {
		line, terminated, err := readEntryLine(r)
		if err != nil {
			return nil, fmt.Errorf("session: read %s: %w", path, err)
		}
		if len(line) == 0 && !terminated {
			break // clean EOF
		}
		lineNo++
		if strings.TrimSpace(string(line)) == "" {
			if terminated {
				goodBytes += int64(len(line)) + 1
				continue
			}
			// Dangling whitespace with no newline: junk tail, drop it.
			break
		}
		entry, err := decodeEntryLine(line, lineNo)
		if err != nil {
			if terminated {
				return nil, err
			}
			// A crash mid-append leaves one torn unterminated line at EOF.
			// Drop it and trim the file so the next append never buries it
			// mid-file, where every future load would fail on it.
			if err := os.Truncate(path, goodBytes); err != nil {
				return nil, fmt.Errorf("session: trim torn tail of %s: %w", path, err)
			}
			break
		}
		switch e := entry.(type) {
		case SessionHeader:
			if header != nil {
				return nil, fmt.Errorf("session: duplicate header at %s:%d", path, lineNo)
			}
			h := e
			header = &h
			entries = append(entries, h)
		case PlanEntry:
			if header == nil {
				return nil, fmt.Errorf("session: first entry must be session header at %s:%d", path, lineNo)
			}
			normalized, err := normalizeLoadedPlan(e.Plan)
			if err != nil {
				return nil, fmt.Errorf("session: invalid plan at %s:%d: %w", path, lineNo, err)
			}
			e.Plan = normalized
			id := e.GetID()
			byIDs[id] = e
			entries = append(entries, e)
			plan = e.Plan
		default:
			if header == nil {
				return nil, fmt.Errorf("session: first entry must be session header at %s:%d", path, lineNo)
			}
			id := entry.GetID()
			byIDs[id] = entry
			entries = append(entries, entry)
			leafID = &id
			if msg, ok := entry.(SessionMessageEntry); ok && msg.Message.Role == llm.RoleAssistant {
				hasAssistantMsg = true
			}
		}
		if terminated {
			goodBytes += int64(len(line)) + 1
			continue
		}
		// The entry decoded but its newline never landed. Restore the line
		// terminator so the next append starts on a fresh line instead of
		// gluing itself onto this one.
		if err := terminateLastLine(path); err != nil {
			return nil, err
		}
		break
	}
	if header == nil {
		return nil, fmt.Errorf("session: missing header in %s", path)
	}

	parent := header.ParentSession
	return &Manager{
		cwd:         header.Cwd,
		entries:     entries,
		byIDs:       byIDs,
		sessionFile: path,
		leafID:      leafID,
		shouldFlush: true,
		flushed:     true,
		sessionID:   header.ID,
		model:       header.Model,
		plan:        plan,
		config: ManagerConfig{
			sessionDir:  filepath.Dir(path),
			shouldFlush: true,
			parentID:    parent,
		},
		hasAssistantMsg: hasAssistantMsg,
	}, nil
}

// terminateLastLine appends the '\n' a fully-written final entry lost to a
// crash between write and flush, so later appends start on a fresh line.
func terminateLastLine(path string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("session: terminate %s: %w", path, err)
	}
	defer f.Close()
	if _, err := f.WriteString("\n"); err != nil {
		return fmt.Errorf("session: terminate %s: %w", path, err)
	}
	return nil
}

func decodeEntryLine(raw []byte, lineNo int) (MessageEntry, error) {
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, fmt.Errorf("session: line %d: %w", lineNo, err)
	}
	switch probe.Type {
	case EntrySession, "session":
		var h SessionHeader
		if err := json.Unmarshal(raw, &h); err != nil {
			return nil, fmt.Errorf("session: line %d header: %w", lineNo, err)
		}
		h.Type = EntrySession
		return h, nil
	case EntryMessage:
		var m SessionMessageEntry
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("session: line %d message: %w", lineNo, err)
		}
		// Message.Usage is intentionally excluded from provider payload JSON.
		// Restore the persisted wrapper value so every in-memory consumer sees
		// the same message shape before and after resume.
		m.Message.Usage = m.Usage
		return m, nil
	case EntryCompaction:
		var c CompactionEntry
		if err := json.Unmarshal(raw, &c); err != nil {
			return nil, fmt.Errorf("session: line %d compaction: %w", lineNo, err)
		}
		return c, nil
	case EntryPlan:
		var p PlanEntry
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("session: line %d plan: %w", lineNo, err)
		}
		return p, nil
	case EntryBranchSummary, "branch_summary":
		var b BranchSummaryEntry
		if err := json.Unmarshal(raw, &b); err != nil {
			return nil, fmt.Errorf("session: line %d branch_summary: %w", lineNo, err)
		}
		return b, nil
	default:
		return nil, fmt.Errorf("session: line %d: unknown type %q", lineNo, probe.Type)
	}
}

// ID returns the session identifier from the header.
func (sm *Manager) ID() string {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.sessionID
}

// File returns the JSONL path, or empty when not persisting.
func (sm *Manager) File() string {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.sessionFile
}

// Cwd returns the session working directory recorded in the header.
func (sm *Manager) Cwd() string {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.cwd
}

// LeafID returns the current leaf entry id, or empty if none.
func (sm *Manager) LeafID() string {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.leafID == nil {
		return ""
	}
	return *sm.leafID
}
