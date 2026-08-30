package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/plantel"
)

// Manager is the single source of truth for session messages.
// Entries form a tree linked by parent IDs; context is built by walking
// from the current leaf back to the session root, honoring compaction.
//
// It is safe for concurrent use: mutations and reads take the internal lock.
type Manager struct {
	mu              sync.Mutex
	cwd             string
	entries         []MessageEntry // session header and session entries
	byIDs           map[string]MessageEntry
	sessionFile     string
	leafID          *string
	shouldFlush     bool
	flushed         bool
	sessionID       string
	model           string
	plan            Plan
	config          ManagerConfig
	hasAssistantMsg bool
	// telemetry is the bounded plan observability budget: runtime-only,
	// counters only, nil-safe. approvedOnce marks that this plan has been
	// approved at least once, so flips after the first approval read as churn
	// even across a flush-and-reopen gap. awaitingReapproval marks that a
	// material change dropped a decided plan's approval, so the next grant
	// counts as a material reapproval.
	telemetry          *plantel.Tracker
	approvedOnce       bool
	awaitingReapproval bool
}

// ManagerConfig holds the options used to build a Manager.
type ManagerConfig struct {
	sessionDir  string
	shouldFlush bool
	parentID    string
	model       string
}

// ManagerOption applies a mutation to ManagerConfig.
type ManagerOption interface {
	Apply(config ManagerConfig) ManagerConfig
}

// OptionFunc adapts a function into a ManagerOption.
type OptionFunc func(config ManagerConfig) ManagerConfig

// Apply calls fn on config and returns the result.
func (fn OptionFunc) Apply(config ManagerConfig) ManagerConfig {
	return fn(config)
}

// WithShouldFlush returns an option that enables JSONL persistence.
func WithShouldFlush(shouldFlush bool) OptionFunc {
	return func(config ManagerConfig) ManagerConfig {
		config.shouldFlush = shouldFlush
		return config
	}
}

// WithSessionDir returns an option that sets the directory for session files.
func WithSessionDir(sessionDir string) OptionFunc {
	return func(config ManagerConfig) ManagerConfig {
		config.sessionDir = sessionDir
		return config
	}
}

// WithParent returns an option that links the session to a parent session ID.
func WithParent(sessionID string) OptionFunc {
	return func(config ManagerConfig) ManagerConfig {
		config.parentID = sessionID
		return config
	}
}

// WithModel returns an option that records the model name used by the session.
func WithModel(name string) OptionFunc {
	return func(config ManagerConfig) ManagerConfig {
		config.model = name
		return config
	}
}

// NewSessionManager creates a session rooted at sessionPath. WithSessionDir +
// WithShouldFlush(true) enable persisting entries as JSONL.
func NewSessionManager(sessionPath string, opt ...ManagerOption) (*Manager, error) {
	config := ManagerConfig{}
	for _, o := range opt {
		config = o.Apply(config)
	}

	sessionID := generateSessionID()
	header := SessionHeader{
		Type:          EntrySession,
		ParentSession: config.parentID,
		ID:            sessionID,
		Timestamp:     time.Now().Format("2006-01-02T15-04-05"),
		Cwd:           sessionPath,
		Model:         config.model,
	}

	m := &Manager{
		cwd:         sessionPath,
		config:      config,
		entries:     make([]MessageEntry, 0, 64),
		byIDs:       make(map[string]MessageEntry, 64),
		leafID:      nil,
		sessionID:   sessionID,
		model:       config.model,
		flushed:     false,
		shouldFlush: config.shouldFlush,
		telemetry:   &plantel.Tracker{},
	}
	m.entries = append(m.entries, header)

	if config.shouldFlush {
		if err := os.MkdirAll(config.sessionDir, 0o755); err != nil {
			return nil, err
		}
		fileTimestamp := time.Now().Format("2006-01-02T15-04-05")
		m.sessionFile = filepath.Join(config.sessionDir, fmt.Sprintf("%s_%s.jsonl", fileTimestamp, m.sessionID))
	}
	return m, nil
}

// NewManager creates an in-memory session manager (no persistence).
func NewManager(sessionDir string) *Manager {
	m, err := NewSessionManager(sessionDir, WithShouldFlush(false))
	if err != nil {
		panic(err) // cannot fail without flush
	}
	return m
}

// GetBranch returns the path of entries from fromID back to the session root,
// newest first.
func (sm *Manager) GetBranch(fromID string) []MessageEntry {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	var message []MessageEntry

	current := sm.byIDs[fromID]
	for current != nil {
		message = append(message, current)
		parent := current.GetParent()
		if parent == nil {
			break
		}
		current = sm.byIDs[*parent]
	}
	return message
}

// BuildContext returns the conversation path from the current leaf to the
// root, oldest first, with compaction applied (compaction entry plus messages
// kept after it).
func (sm *Manager) BuildContext() []MessageEntry {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.leafID == nil {
		return nil
	}
	return buildSessionContext(sm.entries, *sm.leafID, sm.byIDs)
}

// Append adds a message as a new leaf and returns its entry ID.
func (sm *Manager) Append(msg llm.Message) (string, error) {
	return sm.appendMessage(msg, "")
}

// AppendAssistant records an assistant message together with the model name
// that generated it, so a resumed session can pick up where it left off.
func (sm *Manager) AppendAssistant(msg llm.Message, model string) (string, error) {
	return sm.appendMessage(msg, model)
}

func (sm *Manager) appendMessage(msg llm.Message, model string) (string, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	entry := SessionMessageEntry{
		SessionBaseEntry: SessionBaseEntry{
			Type:      EntryMessage,
			ID:        sm.generateID(),
			ParentID:  sm.leafID,
			Timestamp: time.Now(),
		},
		Message: msg,
		Usage:   msg.Usage,
		Model:   model,
	}
	if err := sm.appendEntry(entry); err != nil {
		return "", err
	}
	return entry.ID, nil
}

// AppendCompaction adds a compaction entry as a new leaf and returns its ID.
func (sm *Manager) AppendCompaction(compaction Compaction) (string, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.appendCompactionLocked(compaction)
}

// appendCompactionLocked appends the compaction as a new leaf. Each
// compaction entry fully describes the context shape, so every new one —
// trim, auto-compaction or user drop — inherits the current drop mask and
// unions the caller's additions; without this a later shape change would
// resurrect blocks the user deleted earlier. Callers hold mu.
func (sm *Manager) appendCompactionLocked(compaction Compaction) (string, error) {
	compaction.DroppedEntryIDs = unionIDs(sm.droppedSetLocked(), compaction.DroppedEntryIDs)

	entry := CompactionEntry{
		SessionBaseEntry: SessionBaseEntry{
			Type:      EntryCompaction,
			ID:        sm.generateID(),
			ParentID:  sm.leafID,
			Timestamp: time.Now(),
		},
		Compaction: compaction,
	}
	if err := sm.appendEntry(entry); err != nil {
		return "", err
	}
	return entry.ID, nil
}

// DropContextEntries deletes the given entries from the model's context:
// the log stays append-only (the drop is recorded as a compaction entry),
// but BuildContext and every view built on it no longer contain the entries.
func (sm *Manager) DropContextEntries(ids ...string) error {
	if len(ids) == 0 {
		return nil
	}
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for _, id := range ids {
		if _, ok := sm.byIDs[id]; !ok {
			return errUnknownDropEntry(id)
		}
	}

	// The new shape keeps everything the current context keeps. The anchor
	// must be the first MESSAGE entry of that context: anchoring at a
	// compaction entry would lose the messages that precede it on the path.
	// The prior compaction's summary is folded into the drop marker — the
	// newest compaction wins in buildSessionContext, and dropping one block
	// must not silently discard the summarized history it carried.
	firstKept := ""
	priorSummary := ""
	if sm.leafID != nil {
		for _, e := range buildSessionContext(sm.entries, *sm.leafID, sm.byIDs) {
			if ce, ok := e.(CompactionEntry); ok {
				if priorSummary == "" {
					priorSummary = ce.Compaction.Summary
				}
				continue
			}
			if firstKept == "" {
				firstKept = e.GetID()
			}
		}
	}

	note := fmt.Sprintf(
		"%d context block(s) deleted by the user at %s; they no longer reach the model.",
		len(ids), time.Now().Format("15:04"))
	if priorSummary != "" {
		note = priorSummary + "\n\n" + note
	}

	_, err := sm.appendCompactionLocked(Compaction{
		Summary:          note,
		FirstKeptEntryID: firstKept,
		// Only the new ids: the union with the inherited mask happens in
		// appendCompactionLocked. Cloning keeps the entry independent of callers.
		DroppedEntryIDs: slices.Clone(ids),
		FromTrim:        true,
	})
	return err
}

// droppedSetLocked returns the drop mask of the newest compaction entry on
// the current path — the one whose shape the context follows. Callers hold mu.
func (sm *Manager) droppedSetLocked() map[string]struct{} {
	if sm.leafID == nil {
		return nil
	}
	for _, e := range walkPath(sm.entries, *sm.leafID, sm.byIDs) {
		if ce, ok := e.(CompactionEntry); ok {
			return toSet(ce.Compaction.DroppedEntryIDs)
		}
	}
	return nil
}

// walkPath returns the leaf-to-root entry chain, oldest first.
func walkPath(entries []MessageEntry, leaf string, byIDs map[string]MessageEntry) []MessageEntry {
	path := make([]MessageEntry, 0, len(entries))
	current := byIDs[leaf]
	for current != nil {
		path = append(path, current)
		parentID := current.GetParent()
		if parentID == nil {
			break
		}
		next, ok := byIDs[*parentID]
		if !ok {
			break
		}
		current = next
	}
	slices.Reverse(path)
	return path
}

func toSet(ids []string) map[string]struct{} {
	if len(ids) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set
}

// unionIDs merges a base set with extra ids, sorted for stable JSON output.
func unionIDs(base map[string]struct{}, extra []string) []string {
	if len(base) == 0 && len(extra) == 0 {
		return nil
	}
	out := make([]string, 0, len(base)+len(extra))
	for id := range base {
		out = append(out, id)
	}
	out = append(out, extra...)
	slices.Sort(out)
	// The inherited mask may already contain the caller's ids.
	out = slices.Compact(out)
	return out
}

func (sm *Manager) appendEntry(entry MessageEntry) error {
	prevLeaf := sm.leafID
	leafID := entry.GetID()
	sm.leafID = &leafID
	sm.byIDs[leafID] = entry
	sm.entries = append(sm.entries, entry)

	if !sm.config.shouldFlush {
		return nil
	}

	prevHasAssistant := sm.hasAssistantMsg
	if msgEntry, ok := entry.(SessionMessageEntry); ok && msgEntry.Message.Role == llm.RoleAssistant {
		sm.hasAssistantMsg = true
	}
	if !sm.hasAssistantMsg {
		return nil
	}
	if err := sm.flush(entry); err != nil {
		// Roll the in-memory state back so a failed append is not half-applied
		// while its caller sees an error. The file side trims its own partial
		// line (appendFile), so memory and disk stay in step.
		sm.leafID = prevLeaf
		delete(sm.byIDs, leafID)
		sm.entries = sm.entries[:len(sm.entries)-1]
		sm.hasAssistantMsg = prevHasAssistant
		return err
	}
	return nil
}

func (sm *Manager) flush(entry MessageEntry) error {
	if !sm.flushed {
		if err := sm.flushAllEntries(); err != nil {
			return err
		}
		sm.flushed = true
		return nil
	}
	return sm.appendFile(entry)
}

// openSessionFile opens the JSONL for writing and tightens the file to
// owner-only. OpenFile's mode applies only at create, so a legacy world-
// readable transcript would keep its loose perms while we append to it.
func openSessionFile(path string, flag int) (*os.File, error) {
	f, err := os.OpenFile(path, flag, 0o600)
	if err != nil {
		return nil, err
	}
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close() // the chmod failure is the one worth reporting
		return nil, fmt.Errorf("session: tighten %s: %w", path, err)
	}
	return f, nil
}

// flushAllEntries rewrites the whole log through a temp file + rename, so a
// crash mid-flush can never destroy the only copy of the transcript: the old
// file stays intact until the rename swaps the complete new one in.
func (sm *Manager) flushAllEntries() error {
	tmp, err := os.CreateTemp(filepath.Dir(sm.sessionFile), filepath.Base(sm.sessionFile)+".tmp-*")
	if err != nil {
		return fmt.Errorf("session: create flush temp: %w", err)
	}
	defer os.Remove(tmp.Name()) // no-op once the rename succeeded
	if err := sm.encodeEntries(tmp, sm.entries); err != nil {
		_ = tmp.Close() // the encode failure is the one worth reporting
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close() // the sync failure is the one worth reporting
		return fmt.Errorf("session: sync %s: %w", tmp.Name(), err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// os.CreateTemp is owner-only, so the rename also tightens legacy
	// world-readable transcripts on rewrite.
	if err := os.Rename(tmp.Name(), sm.sessionFile); err != nil {
		return fmt.Errorf("session: swap in flushed %s: %w", sm.sessionFile, err)
	}
	return nil
}

func (sm *Manager) appendFile(entry MessageEntry) error {
	f, err := openSessionFile(sm.sessionFile, os.O_APPEND|os.O_WRONLY)
	if err != nil {
		return err
	}
	defer f.Close()
	end, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	if err := sm.encodeEntries(f, []MessageEntry{entry}); err != nil {
		// A failed append may have left a partial line; trim it so the next
		// append never buries a torn line mid-file, where every future load
		// would fail on it.
		if truncErr := os.Truncate(sm.sessionFile, end); truncErr != nil {
			return fmt.Errorf("session: append %s: %w (trim partial line: %w)", sm.sessionFile, err, truncErr)
		}
		return err
	}
	return nil
}

func (*Manager) encodeEntries(f *os.File, entries []MessageEntry) error {
	encoder := json.NewEncoder(f)
	for _, e := range entries {
		if err := encoder.Encode(e); err != nil {
			return err
		}
	}
	return nil
}

func (sm *Manager) generateID() string {
	for range 100 {
		bytes := make([]byte, 4)
		if _, err := rand.Read(bytes); err != nil {
			panic(err)
		}
		id := hex.EncodeToString(bytes)
		if _, exists := sm.byIDs[id]; !exists {
			return id
		}
	}
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		panic(err)
	}
	return hex.EncodeToString(bytes)
}

// Len returns the number of entries including the session header.
func (sm *Manager) Len() int {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return len(sm.entries)
}

// Model returns the model the session last used: the most recent assistant
// entry that records a model, falling back to the header model.
func (sm *Manager) Model() string {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for _, entry := range slices.Backward(sm.entries) {
		msg, ok := entry.(SessionMessageEntry)
		if ok && msg.Message.Role == llm.RoleAssistant && msg.Model != "" {
			return msg.Model
		}
	}
	return sm.model
}

func generateSessionID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		panic(err)
	}
	return hex.EncodeToString(bytes)
}

// ShortID returns the 8-char display form of a session id (footer, toasts).
func ShortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// buildSessionContext walks from the leaf back to the root and returns the
// messages that form the LLM context: a compaction entry (if any) followed by
// the messages kept after it.
func buildSessionContext(
	entries []MessageEntry,
	leafId string,
	byId map[string]MessageEntry,
) []MessageEntry {
	if len(byId) == 0 {
		for _, entry := range entries {
			byId[entry.GetID()] = entry
		}
	}

	if leafId == "" {
		return nil
	}

	leaf, ok := byId[leafId]
	if !ok {
		leaf = entries[len(entries)-1]
	}

	path := walkPath(entries, leaf.GetID(), byId)

	var (
		messages      []MessageEntry
		compactionIdx = -1
	)
	for i, m := range path {
		if m.GetType() == EntryCompaction {
			compactionIdx = i
		}
	}

	// The newest compaction's drop mask hides user-deleted entries even
	// though they remain in the append-only log.
	dropped := map[string]struct{}{}
	if compactionIdx >= 0 {
		if ce, ok := path[compactionIdx].(CompactionEntry); ok {
			dropped = toSet(ce.Compaction.DroppedEntryIDs)
		}
	}

	appendMessage := func(entry MessageEntry) {
		if entry.GetType() != EntryMessage {
			return
		}
		if _, skip := dropped[entry.GetID()]; skip {
			return
		}
		messages = append(messages, entry)
	}

	if compactionIdx >= 0 {
		compaction := path[compactionIdx]
		messages = append(messages, compaction)

		firstKeptIdx := compactionIdx
		if ce, ok := compaction.(CompactionEntry); ok && ce.Compaction.FirstKeptEntryID != "" {
			for i := compactionIdx; i >= 0; i-- {
				if path[i].GetID() == ce.Compaction.FirstKeptEntryID {
					firstKeptIdx = i
					break
				}
			}
		}

		for i := firstKeptIdx; i < len(path); i++ {
			appendMessage(path[i])
		}
	} else {
		for _, entry := range path {
			appendMessage(entry)
		}
	}
	return messages
}
