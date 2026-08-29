package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alvnukov/cozyphi/internal/redact"
)

const (
	maxPlanItems           = 32
	maxPlanContentRunes    = 512
	maxPlanNoteRunes       = 512
	maxPlanEvidenceRunes   = 512
	maxPlanSerializedBytes = 16 * 1024

	// The v2 contract carries plan-level context and per-step metadata beyond
	// step prose, so its serialized budget is larger than the legacy snapshot
	// cap while staying explicitly bounded.
	maxPlanV2SerializedBytes   = 96 * 1024
	maxPlanGoalRunes           = 512
	maxPlanApproachRunes       = 1024
	maxPlanWorkingContextRunes = 2048
	maxPlanDirectiveEntries    = 8
	maxPlanDirectiveRunes      = 512 // one success criterion or constraint
	maxPlanStepIDRunes         = 64
	maxPlanStepWhyRunes        = 512
	maxPlanStepDoneWhenRunes   = 512
	maxPlanStepOutcomeRunes    = 512
	maxPlanStepRiskRunes       = 512
	maxPlanEvidenceRefsPerStep = 8
	maxPlanEvidenceRefRunes    = 128

	maxPlanStepBlockerRunes    = 512
	maxPlanStepResumeWhenRunes = 512

	// maxPlanReasonRunes bounds the prose that explains a transition:
	// cancel/reopen reasons and no-evidence explanations.
	maxPlanReasonRunes = 512

	// maxPlanEvents bounds both the audit trail and the mutation ledger:
	// both live in the plan snapshot itself, so one cap bounds its growth.
	maxPlanEvents = 24

	// Previous releases allowed these values. Loading remains compatible;
	// only newly authored snapshots use the tighter model-facing budget.
	legacyMaxPlanItems        = 64
	legacyMaxPlanContentRunes = 512
	// A v2 file larger than twice its write budget cannot have been written by
	// this harness; loading fails closed instead of trusting unbounded input.
	legacyMaxPlanV2SerializedBytes = 96 * 1024
)

// planStepIDPattern is the canonical stable step identity: a lowercase slug so
// ids sort, diff and survive being quoted in prompts and patches.
var planStepIDPattern = regexp.MustCompile(
	fmt.Sprintf(`^[a-z0-9][a-z0-9._-]{0,%d}$`, maxPlanStepIDRunes-1),
)

// ReplacePlan validates and durably appends a complete legacy plan snapshot,
// atomically replacing whatever the manager currently holds under one lock. It
// never compares against a model-supplied revision: the harness owner of the
// plan is the only writer, so a whole-snapshot replace cannot produce a lost
// update. Plan metadata never becomes the session leaf, so it cannot perturb
// branching, compaction, or the provider message context.
func (sm *Manager) ReplacePlan(items []PlanItem) (Plan, error) {
	return sm.ReplacePlanWithAutoApprove(items, false)
}

// ReplacePlanWithAutoApprove atomically replaces a plan and applies the current
// auto-approval policy to the same durable snapshot. Active plans are approved
// when autoApprove is true; plans with no remaining work are always unapproved.
func (sm *Manager) ReplacePlanWithAutoApprove(items []PlanItem, autoApprove bool) (Plan, error) {
	if sm == nil {
		return Plan{}, errors.New("session: plan manager is nil")
	}
	return sm.replacePlanLocked(items, autoApprove)
}

// PlanV2 is the author-facing v2 work contract: what the plan is for, how it
// will get there, what proves it done, and the bounded working context that
// outlives compaction. Steps carry their own stable identity and exit
// conditions; Result and ClosedAt record how a finished plan ended.
type PlanV2 struct {
	Goal            string
	Approach        string
	SuccessCriteria []string
	Constraints     []string
	WorkingContext  string
	Items           []PlanItem
	Result          PlanResult
	ClosedAt        *time.Time
}

// ReplacePlanV2 validates the v2 contract strictly and durably replaces the
// current plan under one lock. Approval resets when the contract changes, not
// when operational metadata does; see materialDiff. The returned diff is the
// material change against the previous snapshot — empty when the replace
// kept the user's approval. Replacing the contract starts a new plan: the
// transition audit trail and mutation ledger do not carry over.
func (sm *Manager) ReplacePlanV2(contract PlanV2, autoApprove bool) (Plan, []PlanMaterialChange, error) {
	if sm == nil {
		return Plan{}, nil, errors.New("session: plan manager is nil")
	}
	plan, err := normalizePlanV2(contract)
	if err != nil {
		return Plan{}, nil, err
	}
	// The whole serialized plan, contract included, stays under one budget.
	if err := planWithinSerializedBudget(plan); err != nil {
		return Plan{}, nil, err
	}
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.commitPlanLocked(plan, autoApprove)
}

// RenamePlanStepTypes durably migrates type references without changing plan
// approval, content, status, note, evidence, or the v2 contract. It is the
// session half of a global settings rename transaction.
func (sm *Manager) RenamePlanStepTypes(renames map[StepType]StepType) (Plan, error) {
	if sm == nil {
		return Plan{}, errors.New("session: plan manager is nil")
	}
	for from, to := range renames {
		if from == "" || to == "" {
			return Plan{}, errors.New("session: step type rename cannot be empty")
		}
	}
	sm.mu.Lock()
	defer sm.mu.Unlock()
	plan := sm.plan.Clone()
	changed := false
	for i := range plan.Items {
		if renamed, ok := renames[plan.Items[i].Type]; ok && renamed != plan.Items[i].Type {
			plan.Items[i].Type = renamed
			changed = true
		}
	}
	if !changed {
		return sm.plan.Clone(), nil
	}
	// A type rewrite changes what a step may run, which is material for the
	// just-in-time grants even though the rename transaction deliberately
	// keeps the user's plan approval: grants expire with the contract they
	// approved.
	plan.ContractEpoch = sm.plan.ContractEpoch + 1
	plan.Revision = sm.plan.Revision + 1
	plan.UpdatedAt = time.Now()
	return sm.persistPlanLocked(plan)
}

// replacePlanLocked normalizes, validates, and persists a legacy full-snapshot
// replace. Model-supplied v2 step fields are stripped: the legacy tool contract
// speaks only content/status/type/note/evidence, and the harness owns step
// identity and contract metadata. Replacing a v2 plan through the legacy path
// therefore drops the plan-level contract and resets approval — the legacy
// snapshot is the whole truth of what the model sent.
func (sm *Manager) replacePlanLocked(items []PlanItem, autoApprove bool) (Plan, error) {
	normalized, err := normalizePlanItems(items)
	if err != nil {
		return Plan{}, err
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	plan, _, err := sm.commitPlanLocked(Plan{Schema: PlanSchemaLegacy, Items: normalized}, autoApprove)
	return plan, err
}

// commitPlanLocked stamps the revision, decides approval, and persists. The
// material diff against the live snapshot is the single approval authority:
// an empty diff keeps the user's approval, any material change drops it, and
// approval always closes when no active work remains. The caller holds sm.mu.
func (sm *Manager) commitPlanLocked(next Plan, autoApprove bool) (Plan, []PlanMaterialChange, error) {
	diff := materialDiff(sm.plan, next)
	if len(diff) > 0 && sm.plan.Schema.IsV2() {
		// Re-deciding a live v2 contract is a material revision; authoring
		// the first plan or upgrading a legacy one is not.
		sm.telemetry.MaterialRevision()
	}
	approved := sm.plan.Approved
	if len(diff) > 0 {
		approved = false
	}
	if !planItemsHaveActiveWork(next.Items) {
		approved = false
	} else if autoApprove {
		approved = true
	}
	// The contract epoch moves only with a material change: it is the
	// expiry clock for just-in-time step grants, which must survive the
	// operational writes (status, attempts, evidence) that work proceeds by.
	next.ContractEpoch = sm.plan.ContractEpoch
	if len(diff) > 0 {
		next.ContractEpoch++
	}
	next.Revision = sm.plan.Revision + 1
	next.UpdatedAt = time.Now()
	next.Approved = approved
	plan, err := sm.persistPlanLocked(next)
	if err != nil {
		return Plan{}, nil, err
	}
	return plan, diff, nil
}

// PlanStatus is the lifecycle state of one model-managed plan item.
type PlanStatus string

const (
	PlanPending    PlanStatus = "pending"
	PlanInProgress PlanStatus = "in_progress"
	PlanBlocked    PlanStatus = "blocked"
	PlanCompleted  PlanStatus = "completed"
	PlanCancelled  PlanStatus = "cancelled"
)

// StepType classifies what a plan item is allowed to do. The plan gate maps
// a step's type onto the set of tools it may call.
type StepType string

const (
	StepExplore   StepType = "explore"   // read, grep, find, ls
	StepEdit      StepType = "edit"      // + write, edit
	StepRun       StepType = "run"       // + bash
	StepDelegate  StepType = "delegate"  // + agent_*
	StepIntegrate StepType = "integrate" // + mcp_*
)

// PlanSchema marks the authoring contract a snapshot was written under. The
// zero value means the file predates the field and is treated as legacy.
type PlanSchema int

const (
	PlanSchemaLegacy PlanSchema = iota + 1
	PlanSchemaV2
)

// IsV2 reports whether the snapshot carries the full v2 work contract.
func (s PlanSchema) IsV2() bool { return s == PlanSchemaV2 }

// PlanResult records how a finished plan ended. It is lifecycle metadata: it
// never resets approval and never reopens closed steps.
type PlanResult string

const (
	PlanResultSuccess   PlanResult = "success"
	PlanResultAbandoned PlanResult = "abandoned"
)

// PlanItem is one actionable step in the current session plan. Content is the
// canonical action. The v2 contract fields (ID, Why, DoneWhen, Outcome, Risk,
// JIT, EvidenceRefs) are required or owned by the v2 authoring path; Blocker
// and ResumeWhen are owned by the block transition; Attempts are
// harness-recorded evidence of accepted tool calls. Legacy snapshots simply
// leave the empty fields out in the same canonical shape.
type PlanItem struct {
	Content  string     `json:"content"`
	Status   PlanStatus `json:"status"`
	Type     StepType   `json:"type,omitempty"`
	Note     string     `json:"note,omitempty"`
	Evidence string     `json:"evidence,omitempty"`

	ID           string   `json:"id,omitempty"`
	Why          string   `json:"why,omitempty"`
	DoneWhen     string   `json:"doneWhen,omitempty"`
	Outcome      string   `json:"outcome,omitempty"`
	Risk         string   `json:"risk,omitempty"`
	JIT          bool     `json:"jit,omitempty"`
	EvidenceRefs []string `json:"evidenceRefs,omitempty"`
	Blocker      string   `json:"blocker,omitempty"`
	ResumeWhen   string   `json:"resumeWhen,omitempty"`

	Attempts []PlanAttempt `json:"attempts,omitempty"`
}

// Plan is the latest durable, ordered plan snapshot for a session. Legacy
// snapshots carry only Items; v2 snapshots add the work contract, result
// metadata, and the bounded transition history (audit events plus the
// mutation ledger). Both load into this one canonical representation.
type Plan struct {
	Revision  uint64     `json:"revision"`
	UpdatedAt time.Time  `json:"updatedAt"`
	Items     []PlanItem `json:"items"`
	Approved  bool       `json:"approved,omitempty"`

	Schema          PlanSchema `json:"schema,omitempty"`
	Goal            string     `json:"goal,omitempty"`
	Approach        string     `json:"approach,omitempty"`
	SuccessCriteria []string   `json:"successCriteria,omitempty"`
	Constraints     []string   `json:"constraints,omitempty"`
	WorkingContext  string     `json:"workingContext,omitempty"`
	Result          PlanResult `json:"result,omitempty"`
	ClosedAt        *time.Time `json:"closedAt,omitempty"`

	Events    []PlanEvent    `json:"events,omitempty"`
	Mutations []PlanMutation `json:"mutations,omitempty"`

	// ContractEpoch counts material changes to the work contract. The
	// approval policy resets on every move; just-in-time step grants are
	// pinned to the epoch they were granted at, so operational writes keep
	// them and any material change expires them.
	ContractEpoch uint64 `json:"contractEpoch,omitempty"`
	// JITApprovals records user-owned just-in-time approvals, at most one
	// per step. The model cannot author them: create drops them and no
	// patch operation writes them.
	JITApprovals []JITApproval `json:"jitApprovals,omitempty"`
}

// JITApproval is one user-owned just-in-time grant: the step it frees and
// the contract epoch it was granted at. A grant is valid only while the
// epoch still matches — operational writes keep it, any material change to
// the contract expires it, and it never covers another step.
type JITApproval struct {
	StepID string    `json:"stepId"`
	Epoch  uint64    `json:"epoch"`
	At     time.Time `json:"at,omitempty"`
}

// JITGranted reports whether the user approved this just-in-time step at
// the current contract epoch.
func (p Plan) JITGranted(stepID string) bool {
	if stepID == "" {
		return false
	}
	for _, grant := range p.JITApprovals {
		if grant.StepID == stepID && grant.Epoch == p.ContractEpoch {
			return true
		}
	}
	return false
}

// Clone returns a snapshot whose items do not alias manager state.
func (p Plan) Clone() Plan {
	p.Items = slices.Clone(p.Items)
	for i := range p.Items {
		p.Items[i].EvidenceRefs = slices.Clone(p.Items[i].EvidenceRefs)
		p.Items[i].Attempts = slices.Clone(p.Items[i].Attempts)
	}
	p.SuccessCriteria = slices.Clone(p.SuccessCriteria)
	p.Constraints = slices.Clone(p.Constraints)
	p.Events = slices.Clone(p.Events)
	for i := range p.Events {
		p.Events[i].EvidenceRefs = slices.Clone(p.Events[i].EvidenceRefs)
	}
	p.Mutations = slices.Clone(p.Mutations)
	p.JITApprovals = slices.Clone(p.JITApprovals)
	if p.ClosedAt != nil {
		closed := *p.ClosedAt
		p.ClosedAt = &closed
	}
	return p
}

// HasActiveWork reports whether the plan contains a step that is not completed
// or cancelled. An empty plan has no active work.
func (p Plan) HasActiveWork() bool { return planItemsHaveActiveWork(p.Items) }

func planItemsHaveActiveWork(items []PlanItem) bool {
	for _, item := range items {
		if item.Status != PlanCompleted && item.Status != PlanCancelled {
			return true
		}
	}
	return false
}

// PlanEntry stores a plan snapshot in the append-only session log without
// moving the conversational leaf or entering the model context.
type PlanEntry struct {
	SessionBaseEntry
	Plan Plan `json:"plan"`
}

func (PlanEntry) GetType() string    { return EntryPlan }
func (p PlanEntry) GetID() string    { return p.ID }
func (PlanEntry) GetParent() *string { return nil }

// Plan returns the latest session plan snapshot.
func (sm *Manager) Plan() Plan {
	if sm == nil {
		return Plan{}
	}
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.plan.Clone()
}

// SetPlanApproved flips the user-owned approval flag and durably appends the
// new snapshot. The rest of the plan, v2 contract included, stays untouched;
// only the revision moves.
func (sm *Manager) SetPlanApproved(approved bool) (Plan, error) {
	if sm == nil {
		return Plan{}, errors.New("session: plan manager is nil")
	}
	sm.mu.Lock()
	defer sm.mu.Unlock()
	wasDecided := sm.plan.Approved || sm.approvedOnce
	if sm.plan.Approved {
		sm.approvedOnce = true
	}
	if wasDecided && approved != sm.plan.Approved {
		// Flipping an already-decided plan is churn; approving a fresh plan
		// is the decision itself. The runtime flag bridges the gap between
		// a withdrawal and the re-grant, and re-seeds from persisted state.
		sm.telemetry.ApprovalChurn()
	}
	plan := sm.plan.Clone()
	plan.Revision = sm.plan.Revision + 1
	plan.UpdatedAt = time.Now()
	plan.Approved = approved
	return sm.persistPlanLocked(plan)
}

// SetStepJITApproved records or withdraws the user-owned just-in-time
// approval for one step. Granting requires the step to exist and carry the
// JIT marker; withdrawing is lenient — the safe state needs no ceremony.
// The grant is pinned to the current contract epoch (see JITGranted), so
// only the revision moves: plan approval itself is untouched.
func (sm *Manager) SetStepJITApproved(stepID string, granted bool) (Plan, error) {
	if sm == nil {
		return Plan{}, errors.New("session: plan manager is nil")
	}
	stepID = strings.TrimSpace(stepID)
	if stepID == "" {
		return Plan{}, errors.New("session: just-in-time step id is required")
	}
	sm.mu.Lock()
	defer sm.mu.Unlock()
	plan := sm.plan.Clone()
	hadGrant := false
	for _, grant := range plan.JITApprovals {
		if grant.StepID == stepID {
			hadGrant = true
			break
		}
	}
	if hadGrant {
		// Re-grants and withdrawals of live grants are re-decisions; the
		// first grant of a step is the decision itself.
		sm.telemetry.ApprovalChurn()
	}
	if granted {
		idx := findStepByID(plan.Items, stepID)
		if idx < 0 {
			return Plan{}, fmt.Errorf("session: step %q not found", stepID)
		}
		if !plan.Items[idx].JIT {
			return Plan{}, fmt.Errorf("session: step %q is not marked just-in-time", stepID)
		}
		plan.JITApprovals = upsertJITApproval(plan.JITApprovals, JITApproval{
			StepID: stepID,
			Epoch:  plan.ContractEpoch,
			At:     time.Now(),
		})
	} else {
		plan.JITApprovals = removeJITApproval(plan.JITApprovals, stepID)
	}
	plan.Revision = sm.plan.Revision + 1
	plan.UpdatedAt = time.Now()
	return sm.persistPlanLocked(plan)
}

// upsertJITApproval folds one grant into the approval list: one entry per
// step, the newest grant wins.
func upsertJITApproval(grants []JITApproval, grant JITApproval) []JITApproval {
	for i := range grants {
		if grants[i].StepID == grant.StepID {
			grants[i] = grant
			return grants
		}
	}
	return append(grants, grant)
}

func removeJITApproval(grants []JITApproval, stepID string) []JITApproval {
	for i := range grants {
		if grants[i].StepID == stepID {
			return append(grants[:i:i], grants[i+1:]...)
		}
	}
	return grants
}

// ClearPlan drops the durable plan and resets its revision counter to zero.
func (sm *Manager) ClearPlan() (Plan, error) {
	if sm == nil {
		return Plan{}, errors.New("session: plan manager is nil")
	}
	sm.mu.Lock()
	defer sm.mu.Unlock()
	plan := Plan{UpdatedAt: time.Now()}
	return sm.persistPlanLocked(plan)
}

// persistPlanLocked appends the plan snapshot and rolls back on flush failure.
// The caller holds sm.mu.
func (sm *Manager) persistPlanLocked(plan Plan) (Plan, error) {
	entry := PlanEntry{
		SessionBaseEntry: SessionBaseEntry{Type: EntryPlan, ID: sm.generateID(), Timestamp: plan.UpdatedAt},
		Plan:             plan,
	}
	previousLen := len(sm.entries)
	sm.entries = append(sm.entries, entry)
	sm.byIDs[entry.ID] = entry
	if sm.config.shouldFlush {
		if err := sm.flush(entry); err != nil {
			sm.entries = sm.entries[:previousLen]
			delete(sm.byIDs, entry.ID)
			return Plan{}, fmt.Errorf("session: persist plan: %w", err)
		}
	}
	sm.plan = plan
	return plan.Clone(), nil
}

// normalizePlanItems validates a legacy full-snapshot replace and strips any
// v2 step fields smuggled through the legacy tool contract.
func normalizePlanItems(items []PlanItem) ([]PlanItem, error) {
	stripped := make([]PlanItem, len(items))
	for i, item := range items {
		stripped[i] = stripV2StepFields(item)
	}
	// The legacy door sees the same model-authored prose as create/patch; the
	// sanitize throat applies here too so no door around it exists.
	for i := range stripped {
		var err error
		if stripped[i].Content, err = sanitizePlanProse("step content", stripped[i].Content); err != nil {
			return nil, err
		}
		if stripped[i].Note, err = sanitizePlanProse("step note", stripped[i].Note); err != nil {
			return nil, err
		}
		if stripped[i].Evidence, err = sanitizePlanProse("step evidence", stripped[i].Evidence); err != nil {
			return nil, err
		}
	}
	return validatePlanItems(stripped, maxPlanItems, maxPlanContentRunes, true)
}

// stripV2StepFields removes the contract fields the legacy wire contract has no
// authority over: step identity and v2 metadata belong to the harness-owned
// authoring path, not to a full-content snapshot replace.
func stripV2StepFields(item PlanItem) PlanItem {
	item.ID = ""
	item.Why = ""
	item.DoneWhen = ""
	item.Outcome = ""
	item.Risk = ""
	item.JIT = false
	item.EvidenceRefs = nil
	item.Blocker = ""
	item.ResumeWhen = ""
	item.Attempts = nil
	return item
}

// sanitizePlanProse is the one throat plan text goes through on every write
// door: control characters other than tab, newline and carriage return are
// refused (they corrupt terminals, logs and diffs), and known secret shapes
// are masked so no surface — projection, sidebar, receipt or audit — ever
// echoes a credential that rode in through model prose or tool output.
func sanitizePlanProse(field, value string) (string, error) {
	for _, r := range value {
		if r == '\t' || r == '\n' || r == '\r' {
			continue
		}
		if r < 0x20 || r == 0x7f {
			return "", fmt.Errorf("session: plan %s must not contain control characters", field)
		}
	}
	return redact.Redact(value), nil
}

// planWithinSerializedBudget bounds the whole plan — contract, items and
// audit — under one budget; every writer enforces it so no door can grow the
// durable snapshot past what load accepts.
func planWithinSerializedBudget(plan Plan) error {
	encoded, err := json.Marshal(plan)
	if err != nil {
		return fmt.Errorf("session: encode plan for size validation: %w", err)
	}
	if len(encoded) > maxPlanV2SerializedBytes {
		return fmt.Errorf("session: plan is %d bytes; maximum is %d", len(encoded), maxPlanV2SerializedBytes)
	}
	return nil
}

// normalizePlanV2 trims and strictly validates the v2 contract. Bounds are
// checked once by boundPlanV2Fields; this path adds only the requireds the
// contract cannot live without.
func normalizePlanV2(contract PlanV2) (Plan, error) {
	goal, err := sanitizePlanProse("goal", strings.TrimSpace(contract.Goal))
	if err != nil {
		return Plan{}, err
	}
	if goal == "" {
		return Plan{}, errors.New("session: plan goal is required")
	}
	approach, err := sanitizePlanProse("approach", strings.TrimSpace(contract.Approach))
	if err != nil {
		return Plan{}, err
	}
	if approach == "" {
		return Plan{}, errors.New("session: plan approach is required")
	}
	criteria, err := normalizeDirectives(contract.SuccessCriteria, "success criterion", true)
	if err != nil {
		return Plan{}, err
	}
	constraints, err := normalizeDirectives(contract.Constraints, "constraint", false)
	if err != nil {
		return Plan{}, err
	}
	if err := validatePlanResult(contract.Result, contract.ClosedAt); err != nil {
		return Plan{}, err
	}
	workingContext, err := sanitizePlanProse("working context", strings.TrimSpace(contract.WorkingContext))
	if err != nil {
		return Plan{}, err
	}
	// Attempts are harness-recorded evidence; the authoring path cannot
	// seed them, so a contract that arrives carrying attempts loses them.
	stripped := slices.Clone(contract.Items)
	for i := range stripped {
		stripped[i].Attempts = nil
	}
	items, err := validatePlanItems(stripped, maxPlanItems, maxPlanContentRunes, false)
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{
		Schema:          PlanSchemaV2,
		Goal:            goal,
		Approach:        approach,
		SuccessCriteria: criteria,
		Constraints:     constraints,
		WorkingContext:  workingContext,
		Items:           items,
		Result:          contract.Result,
		ClosedAt:        contract.ClosedAt,
	}
	if err := boundPlanV2Fields(plan); err != nil {
		return Plan{}, err
	}
	plan.Items, err = normalizeV2Steps(plan.Items)
	if err != nil {
		return Plan{}, err
	}
	return plan, nil
}

// normalizeDirectives trims a list of short directives and rejects empty
// entries; required lists must carry at least one entry.
func normalizeDirectives(entries []string, what string, required bool) ([]string, error) {
	if len(entries) == 0 {
		if required {
			return nil, fmt.Errorf("session: plan %s is required", what)
		}
		return nil, nil
	}
	out := make([]string, len(entries))
	for i, entry := range entries {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			return nil, fmt.Errorf("session: plan %s %d is empty", what, i+1)
		}
		sanitized, err := sanitizePlanProse(fmt.Sprintf("%s %d", what, i+1), trimmed)
		if err != nil {
			return nil, err
		}
		out[i] = sanitized
	}
	return out, nil
}

// validPlanResult reports whether result names a known lifecycle outcome.
func validPlanResult(result PlanResult) bool {
	return result == PlanResultSuccess || result == PlanResultAbandoned
}

func validatePlanResult(result PlanResult, closedAt *time.Time) error {
	switch {
	case result == "":
		if closedAt != nil {
			return errors.New("session: plan closed_at requires a result")
		}
	case validPlanResult(result):
		if closedAt == nil {
			return fmt.Errorf("session: plan result %q requires closed_at", result)
		}
	default:
		return fmt.Errorf(
			"session: plan result %q is not one of %q, %q",
			result,
			PlanResultSuccess,
			PlanResultAbandoned,
		)
	}
	return nil
}

// normalizeV2Steps enforces the v2 step contract on every item: a stable
// lowercase-slug id, the reason the step exists, and the observable exit
// condition.
func normalizeV2Steps(items []PlanItem) ([]PlanItem, error) {
	seen := make(map[string]struct{}, len(items))
	for i := range items {
		if err := normalizeV2Step(&items[i], i, true, seen); err != nil {
			return nil, err
		}
	}
	return items, nil
}

// normalizeV2Step trims one item's v2 metadata and validates it. The strict
// authoring path requires id, why and done_when; the lenient load path only
// validates what is present. IDs, when present, must be unique slugs.
func normalizeV2Step(item *PlanItem, i int, requireID bool, seen map[string]struct{}) error {
	item.Why = strings.TrimSpace(item.Why)
	item.DoneWhen = strings.TrimSpace(item.DoneWhen)
	item.Outcome = strings.TrimSpace(item.Outcome)
	item.Risk = strings.TrimSpace(item.Risk)
	item.Blocker = strings.TrimSpace(item.Blocker)
	item.ResumeWhen = strings.TrimSpace(item.ResumeWhen)
	for j, ref := range item.EvidenceRefs {
		// Refs are model-authored free text; mask them like prose so a
		// credential or control byte cannot persist under a ref's clothes.
		v, err := sanitizePlanProse(fmt.Sprintf("evidence ref %d", j+1), strings.TrimSpace(ref))
		if err != nil {
			return err
		}
		item.EvidenceRefs[j] = v
	}
	// The whole step's prose goes through the sanitize throat once, before
	// the bounds: masking shortens, so caps stay meaningful, and control
	// characters never persist.
	for _, f := range []struct {
		name  string
		value *string
	}{
		{"step content", &item.Content},
		{"step note", &item.Note},
		{"step evidence", &item.Evidence},
		{"step why", &item.Why},
		{"step done_when", &item.DoneWhen},
		{"step outcome", &item.Outcome},
		{"step risk", &item.Risk},
		{"step blocker", &item.Blocker},
		{"step resume_when", &item.ResumeWhen},
	} {
		v, err := sanitizePlanProse(f.name, *f.value)
		if err != nil {
			return err
		}
		*f.value = v
	}
	if err := boundStepV2Fields(*item, i); err != nil {
		return err
	}
	if item.ID != "" {
		if utf8.RuneCountInString(item.ID) > maxPlanStepIDRunes {
			return fmt.Errorf("session: plan step %d id exceeds %d characters", i+1, maxPlanStepIDRunes)
		}
		if !planStepIDPattern.MatchString(item.ID) {
			return fmt.Errorf(
				"session: plan step %d id %q must be a lowercase slug of letters, digits, '.', '_' or '-'",
				i+1, item.ID,
			)
		}
		if _, dup := seen[item.ID]; dup {
			return fmt.Errorf("session: plan step %d duplicates id %q", i+1, item.ID)
		}
		seen[item.ID] = struct{}{}
	} else if requireID {
		return fmt.Errorf("session: plan step %d id is required", i+1)
	}
	if requireID && item.Why == "" {
		return fmt.Errorf("session: plan step %d why is required", i+1)
	}
	if requireID && item.DoneWhen == "" {
		return fmt.Errorf("session: plan step %d done_when is required", i+1)
	}
	return nil
}

// boundStepV2Fields bounds the optional v2 step metadata. Shared by the strict
// authoring path and the lenient load path.
func boundStepV2Fields(item PlanItem, i int) error {
	if utf8.RuneCountInString(item.Why) > maxPlanStepWhyRunes {
		return fmt.Errorf("session: plan step %d why exceeds %d characters", i+1, maxPlanStepWhyRunes)
	}
	if utf8.RuneCountInString(item.DoneWhen) > maxPlanStepDoneWhenRunes {
		return fmt.Errorf("session: plan step %d done_when exceeds %d characters", i+1, maxPlanStepDoneWhenRunes)
	}
	if utf8.RuneCountInString(item.Outcome) > maxPlanStepOutcomeRunes {
		return fmt.Errorf("session: plan step %d outcome exceeds %d characters", i+1, maxPlanStepOutcomeRunes)
	}
	if utf8.RuneCountInString(item.Risk) > maxPlanStepRiskRunes {
		return fmt.Errorf("session: plan step %d risk exceeds %d characters", i+1, maxPlanStepRiskRunes)
	}
	if utf8.RuneCountInString(item.Blocker) > maxPlanStepBlockerRunes {
		return fmt.Errorf("session: plan step %d blocker exceeds %d characters", i+1, maxPlanStepBlockerRunes)
	}
	if utf8.RuneCountInString(item.ResumeWhen) > maxPlanStepResumeWhenRunes {
		return fmt.Errorf("session: plan step %d resume_when exceeds %d characters", i+1, maxPlanStepResumeWhenRunes)
	}
	if len(item.EvidenceRefs) > maxPlanEvidenceRefsPerStep {
		return fmt.Errorf("session: plan step %d has %d evidence refs; maximum is %d",
			i+1, len(item.EvidenceRefs), maxPlanEvidenceRefsPerStep)
	}
	for j, ref := range item.EvidenceRefs {
		if utf8.RuneCountInString(ref) > maxPlanEvidenceRefRunes {
			return fmt.Errorf("session: plan step %d evidence ref %d exceeds %d characters",
				i+1, j+1, maxPlanEvidenceRefRunes)
		}
	}
	if len(item.Attempts) > maxPlanAttemptsPerStep {
		return fmt.Errorf(
			"session: plan step %d has %d attempts; maximum is %d",
			i+1, len(item.Attempts), maxPlanAttemptsPerStep,
		)
	}
	for j, attempt := range item.Attempts {
		if attempt.CallID == "" || attempt.Tool == "" {
			return fmt.Errorf("session: plan step %d attempt %d lacks call id or tool", i+1, j+1)
		}
		if utf8.RuneCountInString(attempt.CallID) > maxPlanAttemptCallIDRunes {
			return fmt.Errorf(
				"session: plan step %d attempt %d call id exceeds %d characters",
				i+1, j+1, maxPlanAttemptCallIDRunes,
			)
		}
		if utf8.RuneCountInString(attempt.Tool) > maxPlanAttemptToolRunes {
			return fmt.Errorf(
				"session: plan step %d attempt %d tool exceeds %d characters",
				i+1, j+1, maxPlanAttemptToolRunes,
			)
		}
		if !validPlanAttemptStatus(attempt.Status) {
			return fmt.Errorf(
				"session: plan step %d attempt %d has unknown status %q",
				i+1, j+1, attempt.Status,
			)
		}
		if utf8.RuneCountInString(attempt.Summary) > maxPlanAttemptSummaryRunes {
			return fmt.Errorf(
				"session: plan step %d attempt %d summary exceeds %d characters",
				i+1, j+1, maxPlanAttemptSummaryRunes,
			)
		}
	}
	return nil
}

// normalizeLoadedPlan decodes any snapshot this binary or a previous release
// wrote into the canonical runtime shape. It stays lenient: legacy limits for
// step prose, v2 bounds only for fields that are present, and no v2
// requirements (not even the result/closed_at pairing) — a session written by
// an older release must resume. A v2 snapshot larger than any this harness
// writes fails closed.
func normalizeLoadedPlan(plan Plan) (Plan, error) {
	switch plan.Schema {
	case 0:
		plan.Schema = PlanSchemaLegacy // files from before the schema field existed
	case PlanSchemaLegacy, PlanSchemaV2:
	default:
		return Plan{}, fmt.Errorf("session: plan schema %d is not supported", plan.Schema)
	}
	if err := boundPlanV2Fields(plan); err != nil {
		return Plan{}, err
	}
	items, err := validatePlanItems(plan.Items, legacyMaxPlanItems, legacyMaxPlanContentRunes, false)
	if err != nil {
		return Plan{}, err
	}
	seen := make(map[string]struct{}, len(items))
	for i := range items {
		if err := normalizeV2Step(&items[i], i, false, seen); err != nil {
			return Plan{}, err
		}
	}
	plan.Items = items
	if err := normalizeLoadedJITApprovals(plan); err != nil {
		return Plan{}, err
	}
	if plan.Schema == PlanSchemaV2 {
		encoded, err := json.Marshal(plan)
		if err != nil {
			return Plan{}, fmt.Errorf("session: encode plan for size validation: %w", err)
		}
		if len(encoded) > legacyMaxPlanV2SerializedBytes {
			return Plan{}, fmt.Errorf("session: plan is %d bytes; maximum is %d",
				len(encoded), legacyMaxPlanV2SerializedBytes)
		}
	}
	return plan, nil
}

// normalizeLoadedJITApprovals validates the user-owned grant list on load:
// bounded by the step budget, one entry per step, step ids drawn from the
// plan's own steps, epochs this harness could have written (a future epoch
// would be a standing pre-approval waiting to light up). Anything else is a
// file this harness never wrote, so it fails closed.
func normalizeLoadedJITApprovals(plan Plan) error {
	grants := plan.JITApprovals
	if len(grants) > maxPlanItems {
		return fmt.Errorf(
			"session: plan has %d just-in-time approvals; maximum is %d",
			len(grants),
			maxPlanItems,
		)
	}
	seen := make(map[string]struct{}, len(grants))
	for i, grant := range grants {
		if grant.StepID == "" || !planStepIDPattern.MatchString(grant.StepID) {
			return fmt.Errorf(
				"session: just-in-time approval %d has invalid step id %q",
				i+1,
				grant.StepID,
			)
		}
		if _, dup := seen[grant.StepID]; dup {
			return fmt.Errorf("session: just-in-time approval %d duplicates step %q", i+1, grant.StepID)
		}
		seen[grant.StepID] = struct{}{}
		if findStepByID(plan.Items, grant.StepID) < 0 {
			return fmt.Errorf(
				"session: just-in-time approval %d names unknown step %q",
				i+1,
				grant.StepID,
			)
		}
		if grant.Epoch > plan.ContractEpoch {
			return fmt.Errorf(
				"session: just-in-time approval %d for step %q is pinned to a future contract epoch %d (plan is at %d)",
				i+1,
				grant.StepID,
				grant.Epoch,
				plan.ContractEpoch,
			)
		}
	}
	return nil
}

// boundPlanV2Fields bounds every v2 contract field that is present. It is the
// single bounds table shared by the strict authoring path and the load path.
func boundPlanV2Fields(plan Plan) error {
	if utf8.RuneCountInString(plan.Goal) > maxPlanGoalRunes {
		return fmt.Errorf("session: plan goal exceeds %d characters", maxPlanGoalRunes)
	}
	if utf8.RuneCountInString(plan.Approach) > maxPlanApproachRunes {
		return fmt.Errorf("session: plan approach exceeds %d characters", maxPlanApproachRunes)
	}
	if utf8.RuneCountInString(plan.WorkingContext) > maxPlanWorkingContextRunes {
		return fmt.Errorf("session: plan working context exceeds %d characters", maxPlanWorkingContextRunes)
	}
	if err := boundDirectives(plan.SuccessCriteria, "success criterion"); err != nil {
		return err
	}
	if err := boundDirectives(plan.Constraints, "constraint"); err != nil {
		return err
	}
	if plan.Result != "" && !validPlanResult(plan.Result) {
		return fmt.Errorf("session: plan result %q is not one of %q, %q",
			plan.Result, PlanResultSuccess, PlanResultAbandoned)
	}
	return nil
}

func boundDirectives(entries []string, what string) error {
	if len(entries) > maxPlanDirectiveEntries {
		return fmt.Errorf("session: plan has %d %s entries; maximum is %d",
			len(entries), what, maxPlanDirectiveEntries)
	}
	for i, entry := range entries {
		if utf8.RuneCountInString(entry) > maxPlanDirectiveRunes {
			return fmt.Errorf("session: plan %s %d exceeds %d characters", what, i+1, maxPlanDirectiveRunes)
		}
	}
	return nil
}

func validatePlanItems(
	items []PlanItem,
	maxItems int,
	maxContentRunes int,
	enforceSerializedLimit bool,
) ([]PlanItem, error) {
	if len(items) > maxItems {
		return nil, fmt.Errorf("session: plan has %d items; maximum is %d", len(items), maxItems)
	}
	out := make([]PlanItem, len(items))
	inProgress := 0
	for i, item := range items {
		item.Content = strings.TrimSpace(item.Content)
		item.Note = strings.TrimSpace(item.Note)
		item.Evidence = strings.TrimSpace(item.Evidence)
		if item.Content == "" {
			return nil, fmt.Errorf("session: plan item %d content is empty", i+1)
		}
		if utf8.RuneCountInString(item.Content) > maxContentRunes {
			return nil, fmt.Errorf("session: plan item %d content exceeds %d characters", i+1, maxContentRunes)
		}
		if utf8.RuneCountInString(item.Note) > maxPlanNoteRunes {
			return nil, fmt.Errorf("session: plan item %d note exceeds %d characters", i+1, maxPlanNoteRunes)
		}
		if utf8.RuneCountInString(item.Evidence) > maxPlanEvidenceRunes {
			return nil, fmt.Errorf("session: plan item %d evidence exceeds %d characters", i+1, maxPlanEvidenceRunes)
		}
		// Drafting sends contract fields only: a step without a status starts
		// pending on every intake path (create, replace, load). Anything else
		// unknown still fails closed below.
		if item.Status == "" {
			item.Status = PlanPending
		}
		switch item.Status {
		case PlanPending, PlanBlocked, PlanCompleted, PlanCancelled:
		case PlanInProgress:
			inProgress++
		default:
			return nil, fmt.Errorf("session: plan item %d has invalid status %q", i+1, item.Status)
		}
		out[i] = item
	}
	if inProgress > 1 {
		return nil, fmt.Errorf("session: plan has %d in_progress items; maximum is 1", inProgress)
	}
	if enforceSerializedLimit {
		encoded, err := json.Marshal(out)
		if err != nil {
			return nil, fmt.Errorf("session: encode plan for size validation: %w", err)
		}
		if len(encoded) > maxPlanSerializedBytes {
			return nil, fmt.Errorf("session: plan is %d bytes; maximum is %d", len(encoded), maxPlanSerializedBytes)
		}
	}
	return out, nil
}
