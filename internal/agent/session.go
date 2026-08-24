package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/pulseaiclub/phi/internal/llm"
	"github.com/pulseaiclub/phi/internal/session"
)

// Session owns the message store for the engine loop. It wraps a
// session.Manager and projects entries (including compaction summaries)
// into LLM context. Compaction policy lives on Engine, not here.
type Session struct {
	lastID            string
	manager           *session.Manager
	contextCache      []llm.Message
	contextCacheValid bool
}

// SessionOpts configures how the engine binds a session store.
type SessionOpts struct {
	Cwd        string // written to SessionHeader.Cwd; usually process cwd
	SessionDir string // ~/.phi/session; required when Persist is true
	Persist    bool   // false → in-memory (tests default)
	ResumePath string // open this jsonl; ignores "new session"
	ResumeID   string // resolve under SessionDir (mutually exclusive with ResumePath)
	ParentID   string // reserved for sub-agents; passed to WithParent
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
		resumed := &Session{manager: m, lastID: m.LeafID()}
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

// Append records one or more messages.
func (s *Session) Append(message ...llm.Message) error {
	s.invalidateContextCache()
	for _, msg := range message {
		id, err := s.manager.Append(msg)
		if err != nil {
			return err
		}
		s.lastID = id
	}
	return nil
}

// AppendCompaction records a compaction entry and invalidates the context cache.
func (s *Session) AppendCompaction(c session.Compaction) error {
	s.invalidateContextCache()
	id, err := s.manager.AppendCompaction(c)
	if err != nil {
		return err
	}
	s.lastID = id
	return nil
}

// Plan returns the latest durable model-managed plan snapshot.
func (s *Session) Plan() session.Plan {
	if s == nil || s.manager == nil {
		return session.Plan{}
	}
	return s.manager.Plan()
}

// UpdatePlan validates and persists a complete plan snapshot without changing
// the provider context or conversational leaf.
func (s *Session) UpdatePlan(
	ctx context.Context,
	expectedRevision uint64,
	items []session.PlanItem,
) (session.Plan, error) {
	if err := ctx.Err(); err != nil {
		return session.Plan{}, err
	}
	if s == nil || s.manager == nil {
		return session.Plan{}, errors.New("agent: session unavailable")
	}
	return s.manager.UpdatePlan(expectedRevision, items)
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

// Len returns the number of stored entries (including the session header).
func (s *Session) Len() int {
	return s.manager.Len()
}

// LastID returns the ID of the most recently appended entry.
func (s *Session) LastID() string {
	return s.lastID
}

// AddUser records a single user message for this chat turn.
func (s *Session) AddUser(text string) error {
	return s.Append(llm.Message{
		Role:    llm.RoleUser,
		Content: text,
	})
}

// AddAssistant records an assistant message from an LLM response (including tool_calls).
func (s *Session) AddAssistant(assistant llm.Message, usage llm.Usage) error {
	return s.Append(llm.Message{
		Role:             llm.RoleAssistant,
		Content:          assistant.Content,
		ReasoningContent: assistant.ReasoningContent,
		ToolCalls:        assistant.ToolCalls,
		Usage:            usage,
	})
}

// AddFinalAssistant records the last assistant message when it carries text or tool_calls.
func (s *Session) AddFinalAssistant(resp llm.Response) error {
	if len(resp.Choices) == 0 {
		return nil
	}
	final := resp.Choices[0].Message
	if strings.TrimSpace(final.Content) == "" && len(final.ToolCalls) == 0 {
		return nil
	}
	return s.AddAssistant(final, resp.Usage)
}
