package session

import (
	"time"

	"github.com/pulseaiclub/phi/internal/llm"
)

// MessageEntry is one node in the session tree. Entries are linked by
// parent ID, enabling branching and cursor-based context building.
type MessageEntry interface {
	GetType() string
	GetID() string
	GetParent() *string
}

// EntryType is the JSON "type" discriminant of an entry.
type EntryType string

// EntryType values stored in the JSON "type" discriminant.
const (
	EntrySession       = "EntrySession"
	EntryMessage       = "EntryMessage"
	EntryBranchSummary = "EntryBranchSummary"
	EntryCompaction    = "EntryCompaction"
)

// SessionHeader is the root entry of a session file.
type SessionHeader struct {
	Type          string `json:"type"` // Always "session"
	Version       int    `json:"version,omitempty"`
	ID            string `json:"id"`
	Timestamp     string `json:"timestamp"`
	Cwd           string `json:"cwd"`
	ParentSession string `json:"parentSession,omitempty"`
}

// GetType implements MessageEntry.
func (SessionHeader) GetType() string { return EntrySession }

// GetID implements MessageEntry.
func (s SessionHeader) GetID() string { return s.ID }

// GetParent implements MessageEntry.
func (s SessionHeader) GetParent() *string {
	if s.ParentSession == "" {
		return nil
	}
	return &s.ParentSession
}

// SessionBaseEntry is the common header for non-root entries.
type SessionBaseEntry struct {
	Type      string    `json:"type"`
	ID        string    `json:"id"`
	ParentID  *string   `json:"parentID"` // null for root entries
	Timestamp time.Time `json:"timestamp"`
}

// SessionMessageEntry wraps an LLM message as a session tree node.
type SessionMessageEntry struct {
	SessionBaseEntry
	Message llm.Message `json:"message"`
}

// GetType implements MessageEntry.
func (SessionMessageEntry) GetType() string { return EntryMessage }

// GetID implements MessageEntry.
func (s SessionMessageEntry) GetID() string { return s.ID }

// GetParent implements MessageEntry.
func (s SessionMessageEntry) GetParent() *string { return s.ParentID }

// BranchSummary records the summary of a forked branch.
type BranchSummary struct {
	FromID        string `json:"fromId"`
	Summary       string `json:"summary"`
	Details       string `json:"details"`
	FromExecution bool   `json:"fromExecution"`
}

// BranchSummaryEntry is a branch summary node in the session tree.
type BranchSummaryEntry struct {
	SessionBaseEntry
	BranchSummary BranchSummary `json:"branchSummary"`
}

// GetType implements MessageEntry.
func (BranchSummaryEntry) GetType() string { return "branch_summary" }

// GetID implements MessageEntry.
func (b BranchSummaryEntry) GetID() string { return b.ID }

// GetParent implements MessageEntry.
func (b BranchSummaryEntry) GetParent() *string { return b.ParentID }

// Compaction is the data attached to a compaction entry.
type Compaction struct {
	Summary          string         `json:"summary"`
	FirstKeptEntryID string         `json:"firstKeptEntryId"`
	TokensBefore     int            `json:"tokensBefore"`
	Details          any            `json:"details,omitempty"`
	PreserveData     map[string]any `json:"preserveData,omitempty"`
	FromExtension    *bool          `json:"fromExtension,omitempty"`
}

// CompactionEntry is a compaction node in the session tree.
type CompactionEntry struct {
	SessionBaseEntry
	Compaction Compaction `json:"compaction"`
}

// CompactionDetails records file reads/modifications captured at compaction.
type CompactionDetails struct {
	ReadFiles     []string
	ModifiedFiles []string
}

// GetType implements MessageEntry.
func (CompactionEntry) GetType() string { return EntryCompaction }

// GetID implements MessageEntry.
func (c CompactionEntry) GetID() string { return c.ID }

// GetParent implements MessageEntry.
func (c CompactionEntry) GetParent() *string { return c.ParentID }
