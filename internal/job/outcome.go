package job

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Outcome is a correlated, immutable child result, not a user instruction or approval.
// EventID is shared by explicit waits and automatic delivery.
type Outcome struct {
	EventID         string `json:"event_id"`
	JobID           string `json:"job_id"`
	OwnerID         string `json:"owner_id"`
	ParentSessionID string `json:"parent_session_id"`
	ChildSessionID  string `json:"child_session_id"`
	ParentToolUseID string `json:"parent_tool_use_id,omitempty"`
	PreviousJobID   string `json:"previous_job_id,omitempty"`
	Status          Status `json:"status"`
	StopReason      string `json:"stop_reason,omitempty"`
	UserIntervened  bool   `json:"user_intervened"`
	Summary         string `json:"summary,omitempty"`
	Error           string `json:"error,omitempty"`
	ResultPath      string `json:"result_path,omitempty"`
}

// terminalOutcome is shared by push delivery and explicit waits so acknowledging
// either path consumes the same complete, bounded envelope.
func terminalOutcome(meta Meta) *Outcome {
	if !meta.Status.Terminal() || meta.OutcomeID == "" {
		return nil
	}
	return &Outcome{
		EventID: meta.OutcomeID, JobID: meta.ID, OwnerID: meta.OwnerID,
		ParentSessionID: meta.ParentID, ChildSessionID: meta.ChildSessionID,
		ParentToolUseID: meta.ParentToolUseID, PreviousJobID: meta.PreviousJobID,
		Status: meta.Status, StopReason: meta.StopReason, UserIntervened: meta.UserIntervened,
		Summary: meta.OutcomeSummary, Error: boundedOutcomeText(meta.Error), ResultPath: meta.ResultPath,
	}
}

func (m *Manager) waitResult(info Info) WaitResult {
	summary, _ := m.store.readResult(info.Meta)
	outcome := terminalOutcome(info.Meta)
	if summary == "" && outcome != nil {
		// Failed runners need not have written result.md; metadata retains their partial text.
		summary = outcome.Summary
	}
	return WaitResult{Info: info, Summary: summary, Outcome: outcome}
}

func boundedOutcomeText(text string) string {
	const maxBytes = 12000
	if len(text) <= maxBytes {
		return text
	}
	end := maxBytes
	for end > 0 && !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end]
}

// PendingOutcomes reads a bounded batch from durable job metadata. It never
// consumes results; callers must persist their parent-context receipt before
// acknowledging. The owner and conversation must both match, including after
// /clear or /resume. An empty owner cannot subscribe to legacy/headless jobs.
func (m *Manager) PendingOutcomes(ctx context.Context, ownerID, parentID string, limit int) ([]Outcome, error) {
	if ownerID == "" || parentID == "" || limit < 1 || limit > 64 {
		return nil, fmt.Errorf("%w: outcome query requires owner, parent and limit 1..64", ErrInvalid)
	}
	if err := m.reconcileOutcomes(ctx, func(meta Meta) bool {
		return meta.OwnerID == ownerID && meta.ParentID == parentID
	}); err != nil {
		return nil, err
	}
	ids, err := m.store.listIDs()
	if err != nil {
		return nil, err
	}
	var out []Outcome
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		belongs, err := m.store.outcomeBelongsTo(id, ownerID, parentID)
		if err != nil {
			if m.onStoreError != nil {
				m.onStoreError("read outcome scope", id, err)
			}
			continue
		}
		if !belongs {
			continue
		}
		meta, err := m.store.readMeta(id)
		if err != nil {
			return nil, fmt.Errorf("job: read outcome %s: %w", id, err)
		}
		if meta.OwnerID != ownerID || meta.ParentID != parentID || !meta.Status.Terminal() || meta.OutcomeID == "" {
			continue
		}
		receipt, err := os.ReadFile(filepath.Join(m.store.root, id, "outcome.receipt"))
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("job: read outcome receipt %s: %w", id, err)
		}
		if string(receipt) == meta.OutcomeID {
			continue
		}
		out = append(out, *terminalOutcome(meta))
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

// AcknowledgeOutcome persists consumption after the caller has appended the same
// event identity to the exact parent's context. Repeated acknowledgements are
// harmless; mismatched owners, conversations or events fail closed. This does
// not mutate the assignment or the result returned by Wait.
func (m *Manager) AcknowledgeOutcome(ctx context.Context, ownerID, parentID, jobID, eventID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if jobID == "" || jobID == "." || jobID == ".." || strings.ContainsAny(jobID, `/\`) {
		return fmt.Errorf("%w: invalid outcome job ID", ErrInvalid)
	}
	meta, err := m.store.readMeta(jobID)
	if err != nil {
		return err
	}
	if ownerID == "" || parentID == "" || eventID == "" || meta.ID != jobID ||
		meta.OwnerID != ownerID || meta.ParentID != parentID || meta.OutcomeID != eventID || !meta.Status.Terminal() {
		return fmt.Errorf("%w: outcome identity does not match its parent", ErrInvalid)
	}
	// A partial receipt cannot match EventID: interrupted writes cause redelivery,
	// never false consumption. Parent-side event deduplication makes that safe.
	if err := os.WriteFile(filepath.Join(m.store.root, jobID, "outcome.receipt"), []byte(eventID), 0o600); err != nil {
		return fmt.Errorf("job: acknowledge outcome %s: %w", jobID, err)
	}
	return nil
}
