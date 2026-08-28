package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
)

// Session owns the message store for the engine loop. It wraps a
// session.Manager and projects entries (including compaction summaries)
// into LLM context. Compaction policy lives on Engine, not here.
type Session struct {
	manager           *session.Manager
	contextCache      []llm.Message
	contextCacheValid bool
}

// SessionOpts configures how the engine binds a session store.
type SessionOpts struct {
	Cwd        string // written to SessionHeader.Cwd; usually process cwd
	SessionDir string // ~/.cozyphi/session; required when Persist is true
	Persist    bool   // false → in-memory (tests default)
	ResumePath string // open this jsonl; ignores "new session"
	ResumeID   string // resolve under SessionDir (mutually exclusive with ResumePath)
	ParentID   string // reserved for sub-agents; passed to WithParent
	Model      string // recorded in the header of a new session
}

// NewSession creates a session wrapper according to opts.
func NewSession(opts SessionOpts) (*Session, error) {
	if opts.ResumePath != "" && opts.ResumeID != "" {
		return nil, errors.New("agent: ResumePath and ResumeID are mutually exclusive")
	}

	if opts.ResumePath != "" || opts.ResumeID != "" {
		path := opts.ResumePath
		if path == "" {
			if opts.SessionDir == "" {
				return nil, errors.New("agent: SessionDir required to resume by id")
			}
			var err error
			path, err = session.FindSessionFile(opts.SessionDir, opts.ResumeID)
			if err != nil {
				return nil, err
			}
		}
		m, err := session.OpenSession(path)
		if err != nil {
			return nil, err
		}
		resumed := &Session{manager: m}
		if _, err := resumed.RepairPendingToolCalls(); err != nil {
			return nil, err
		}
		return resumed, nil
	}

	if opts.Persist {
		if opts.SessionDir == "" {
			return nil, errors.New("agent: SessionDir required when Persist is true")
		}
		m, err := session.NewSessionManager(opts.Cwd,
			session.WithSessionDir(opts.SessionDir),
			session.WithShouldFlush(true),
			session.WithParent(opts.ParentID),
			session.WithModel(opts.Model),
		)
		if err != nil {
			return nil, err
		}
		return &Session{manager: m}, nil
	}

	return &Session{manager: session.NewManager(opts.Cwd)}, nil
}

// ID returns the durable session id (empty only if manager missing).
func (s *Session) ID() string {
	if s == nil || s.manager == nil {
		return ""
	}
	return s.manager.ID()
}

// File returns the JSONL path, or empty in memory mode / before first flush path assignment.
func (s *Session) File() string {
	if s == nil || s.manager == nil {
		return ""
	}
	return s.manager.File()
}

// Cwd returns the session header cwd.
func (s *Session) Cwd() string {
	if s == nil || s.manager == nil {
		return ""
	}
	return s.manager.Cwd()
}

func (s *Session) invalidateContextCache() {
	s.contextCacheValid = false
}

// Model returns the model the session last used.
func (s *Session) Model() string {
	if s == nil || s.manager == nil {
		return ""
	}
	return s.manager.Model()
}

// Append records one or more messages.
func (s *Session) Append(message ...llm.Message) error {
	s.invalidateContextCache()
	for _, msg := range message {
		if _, err := s.manager.Append(msg); err != nil {
			return err
		}
	}
	return nil
}

// AppendAssistant records an assistant message with the model that generated it.
func (s *Session) AppendAssistant(assistant llm.Message, model string) error {
	s.invalidateContextCache()
	if _, err := s.manager.AppendAssistant(assistant, model); err != nil {
		return err
	}
	return nil
}

// AppendCompaction records a compaction entry and invalidates the context cache.
func (s *Session) AppendCompaction(c session.Compaction) error {
	s.invalidateContextCache()
	if _, err := s.manager.AppendCompaction(c); err != nil {
		return err
	}
	return nil
}

// InspectContext itemizes the current LLM context (see Manager.InspectContext).
func (s *Session) InspectContext() session.ContextReport {
	if s == nil || s.manager == nil {
		return session.ContextReport{}
	}
	return s.manager.InspectContext()
}

// TrimContextFrom drops everything before the entry from the model's context
// (append-only; see Manager.TrimContextFrom).
func (s *Session) TrimContextFrom(entryID string) error {
	if s == nil || s.manager == nil {
		return errors.New("agent: session unavailable")
	}
	if _, err := s.manager.TrimContextFrom(entryID); err != nil {
		return err
	}
	s.invalidateContextCache()
	return nil
}

// DropContextEntries deletes the given entries from the model's context
// (append-only; see Manager.DropContextEntries).
func (s *Session) DropContextEntries(ids []string) error {
	if s == nil || s.manager == nil {
		return errors.New("agent: session unavailable")
	}
	if err := s.manager.DropContextEntries(ids...); err != nil {
		return err
	}
	s.invalidateContextCache()
	return nil
}

// Plan returns the latest durable model-managed plan snapshot.
func (s *Session) Plan() session.Plan {
	if s == nil || s.manager == nil {
		return session.Plan{}
	}
	return s.manager.Plan()
}

// ReplacePlan validates and persists a complete plan snapshot without changing
// the provider context or conversational leaf. Replacement and automatic
// approval are committed as one durable session operation.
func (s *Session) ReplacePlan(
	ctx context.Context,
	items []session.PlanItem,
	autoApprove bool,
) (session.Plan, error) {
	if err := ctx.Err(); err != nil {
		return session.Plan{}, err
	}
	if s == nil || s.manager == nil {
		return session.Plan{}, errors.New("agent: session unavailable")
	}
	return s.manager.ReplacePlanWithAutoApprove(items, autoApprove)
}

// ReplacePlanV2 validates and persists a complete v2 work contract. The
// durable result is a draft: approval stays the user's move.
func (s *Session) ReplacePlanV2(
	ctx context.Context,
	contract session.PlanV2,
	autoApprove bool,
) (session.Plan, error) {
	if err := ctx.Err(); err != nil {
		return session.Plan{}, err
	}
	if s == nil || s.manager == nil {
		return session.Plan{}, errors.New("agent: session unavailable")
	}
	return s.manager.ReplacePlanV2(contract, autoApprove)
}

// RenamePlanStepTypes migrates current-plan type references while preserving
// approval and all other fields.
func (s *Session) RenamePlanStepTypes(
	ctx context.Context,
	renames map[session.StepType]session.StepType,
) (session.Plan, error) {
	if err := ctx.Err(); err != nil {
		return session.Plan{}, err
	}
	if s == nil || s.manager == nil {
		return session.Plan{}, errors.New("agent: session unavailable")
	}
	return s.manager.RenamePlanStepTypes(renames)
}

// SetPlanApproved flips the durable plan approval flag.
func (s *Session) SetPlanApproved(approved bool) (session.Plan, error) {
	if s == nil || s.manager == nil {
		return session.Plan{}, errors.New("agent: session unavailable")
	}
	return s.manager.SetPlanApproved(approved)
}

// ClearPlan drops the durable plan and resets its revision counter.
func (s *Session) ClearPlan() (session.Plan, error) {
	if s == nil || s.manager == nil {
		return session.Plan{}, errors.New("agent: session unavailable")
	}
	return s.manager.ClearPlan()
}

// PathEntries returns the current leaf-to-root session entries for compaction.
func (s *Session) PathEntries() []session.MessageEntry {
	return s.manager.BuildContext()
}

// BuildContext returns the messages for LLM inference, oldest first.
// Compaction entries are projected as user messages carrying the summary.
// The provider view is repaired defensively so legacy interrupted sessions
// cannot send orphan or missing tool results to any backend.
func (s *Session) BuildContext() []llm.Message {
	if s.contextCacheValid {
		return s.contextCache
	}
	msgs := s.buildRawContext()
	msgs, _ = llm.RepairToolHistory(msgs)
	s.contextCache = msgs
	s.contextCacheValid = true
	return msgs
}

func (s *Session) buildRawContext() []llm.Message {
	entries := s.manager.BuildContext()
	msgs := make([]llm.Message, 0, len(entries))
	for _, entry := range entries {
		switch entry.GetType() {
		case session.EntryCompaction:
			m := entry.(session.CompactionEntry)
			msgs = append(msgs, llm.Message{
				Role:    llm.RoleUser,
				Content: m.Compaction.Summary,
			})
		case session.EntryMessage:
			m := entry.(session.SessionMessageEntry)
			msgs = append(msgs, m.Message)
		}
	}
	return msgs
}

// RepairPendingToolCalls durably closes an interrupted trailing tool round.
// It only appends results when the broken round is the current tail; older
// malformed history is repaired in the provider projection without rewriting
// the append-only audit log.
func (s *Session) RepairPendingToolCalls() (int, error) {
	if s == nil || s.manager == nil {
		return 0, errors.New("agent: session unavailable")
	}
	pending := llm.PendingToolResults(s.buildRawContext())
	if len(pending) == 0 {
		return 0, nil
	}
	if err := s.Append(pending...); err != nil {
		return 0, fmt.Errorf("persist recovery results: %w", err)
	}
	return len(pending), nil
}
