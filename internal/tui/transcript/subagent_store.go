package transcript

import (
	"strings"
	"sync"
	"time"

	"github.com/alvnukov/cozyphi/internal/components/block"
	"github.com/alvnukov/cozyphi/internal/components/status"
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/tools"
)

// SubagentRun is everything the UI knows about one child run: the tool rows
// it has made, and — once its outcome has arrived — what it came back with.
// It is a value copy, safe to read while the child keeps working.
type SubagentRun struct {
	JobID    string
	Children []block.ChildTool
	Summary  string
	Error    string
	// Status is the child's terminal job status, "" while it still runs.
	Status job.Status
	// Started/Finished bound the run. Started is when this session first
	// heard of the child; Finished is when its outcome landed, zero while
	// it works. Both are zero for a run this process never watched.
	Started  time.Time
	Finished time.Time
}

// Running reports a child that has begun and has not reported an outcome.
func (r SubagentRun) Running() bool { return !r.Started.IsZero() && r.Finished.IsZero() }

// SubagentStore holds UI-only nested tool trees for agent_* rows.
// It is not part of the session / LLM transcript.
type SubagentStore struct {
	mu       sync.Mutex
	now      func() time.Time
	byParent map[string]*subagentView
	byJob    map[string]*subagentView
}

type subagentView struct {
	JobID           string
	ParentToolUseID string
	Children        []block.ChildTool
	childIdx        map[string]int
	Summary         string
	Error           string
	Status          job.Status
	Started         time.Time
	Finished        time.Time
}

// NewSubagentStore creates an empty store.
func NewSubagentStore() *SubagentStore {
	return &SubagentStore{
		now:      time.Now,
		byParent: make(map[string]*subagentView),
		byJob:    make(map[string]*subagentView),
	}
}

func (s *SubagentStore) clock() time.Time {
	if s.now == nil {
		return time.Now()
	}
	return s.now()
}

// Bind links a job id to the parent agent tool_use id, and starts the run's
// clock: the spawn row is the first moment this session knows a child exists.
func (s *SubagentStore) Bind(jobID, parentToolUseID string) {
	if s == nil || jobID == "" || parentToolUseID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.viewLocked(jobID, parentToolUseID)
	v.JobID = jobID
	v.ParentToolUseID = parentToolUseID
	if v.Started.IsZero() {
		v.Started = s.clock()
	}
}

func (s *SubagentStore) viewLocked(jobID, parentToolUseID string) *subagentView {
	var v *subagentView
	if parentToolUseID != "" {
		v = s.byParent[parentToolUseID]
	}
	if v == nil && jobID != "" {
		v = s.byJob[jobID]
	}
	if v == nil {
		v = &subagentView{
			JobID:           jobID,
			ParentToolUseID: parentToolUseID,
			childIdx:        make(map[string]int),
		}
	} else {
		if jobID != "" {
			v.JobID = jobID
		}
		if parentToolUseID != "" {
			v.ParentToolUseID = parentToolUseID
		}
		if v.childIdx == nil {
			v.childIdx = make(map[string]int)
		}
	}
	if v.ParentToolUseID != "" {
		s.byParent[v.ParentToolUseID] = v
	}
	if v.JobID != "" {
		s.byJob[v.JobID] = v
	}
	return v
}

// ApplyProgress upserts a child tool row under the parent agent tool.
// Returns true when the nested tree view actually changed.
func (s *SubagentStore) ApplyProgress(p job.Progress) bool {
	if s == nil || (p.JobID == "" && p.ParentToolUseID == "") {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.viewLocked(p.JobID, p.ParentToolUseID)
	if v.Started.IsZero() {
		v.Started = s.clock()
	}
	key := p.ToolUseID
	if key == "" {
		key = p.Name + "\x00" + p.Detail
	}
	child := block.ChildTool{
		Name:   p.Name,
		Detail: p.Detail,
		Status: progressStatus(p.Status),
	}
	if i, ok := v.childIdx[key]; ok {
		if v.Children[i] == child {
			return false
		}
		v.Children[i] = child
		return true
	}
	v.childIdx[key] = len(v.Children)
	v.Children = append(v.Children, child)
	return true
}

// ApplyResult records terminal summary / binding from agent tool JSON output.
// An agent_wait that returned a finished job also ends the run's clock: the
// parent read the result there, so no separate outcome delivery follows.
func (s *SubagentStore) ApplyResult(parentToolUseID string, r tools.AgentResult) {
	if s == nil || !r.OK {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.viewLocked(r.JobID, parentToolUseID)
	if sum := r.RenderableSummary(); sum != "" {
		v.Summary = sum
	}
	if r.Terminal() {
		if v.Status == "" {
			v.Status = job.Status(r.Status)
		}
		if v.Error == "" {
			v.Error = r.Error
		}
		if v.Finished.IsZero() {
			v.Finished = s.clock()
		}
	}
}

// ApplyOutcome records a delivered child outcome: the summary the parent was
// given, the terminal status the glyph settles on, and the moment the elapsed
// time freezes. Reports whether anything the UI shows actually changed.
func (s *SubagentStore) ApplyOutcome(o job.Outcome) bool {
	if s == nil || (o.JobID == "" && o.ParentToolUseID == "") || !o.Status.Terminal() {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.viewLocked(o.JobID, o.ParentToolUseID)
	summary := strings.TrimSpace(o.Summary)
	if v.Status == o.Status && v.Summary == summary && v.Error == o.Error {
		return false
	}
	v.Status = o.Status
	if summary != "" {
		v.Summary = summary
	}
	v.Error = o.Error
	if v.Finished.IsZero() {
		v.Finished = s.clock()
	}
	return true
}

// Run returns what the UI knows about one child, addressed by the parent
// agent tool_use id or — when the row never learned one — the job id.
func (s *SubagentStore) Run(parentToolUseID, jobID string) (SubagentRun, bool) {
	if s == nil || (parentToolUseID == "" && jobID == "") {
		return SubagentRun{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.byParent[parentToolUseID]
	if v == nil && jobID != "" {
		v = s.byJob[jobID]
	}
	if v == nil {
		return SubagentRun{}, false
	}
	return SubagentRun{
		JobID:    v.JobID,
		Children: append([]block.ChildTool(nil), v.Children...),
		Summary:  v.Summary,
		Error:    v.Error,
		Status:   v.Status,
		Started:  v.Started,
		Finished: v.Finished,
	}, true
}

func progressStatus(s string) status.ToolStatus {
	switch s {
	case "done":
		return status.ToolDone
	case "error":
		return status.ToolError
	case "cancelled":
		return status.ToolCancelled
	case "rejected", "rejected-by-user":
		return status.ToolRejected
	case "queued":
		return status.ToolQueued
	default:
		return status.ToolRunning
	}
}
