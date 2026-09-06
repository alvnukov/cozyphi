package agent

import (
	"hash/fnv"
	"slices"
	"strconv"

	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/plantel"
	"github.com/alvnukov/cozyphi/internal/session"
)

// planStatusOrder is the order the step breakdown is reported in: the
// statuses a step moves through, then the three ways it stops owing work.
// A fixed order is what makes two snapshots of the same plan comparable.
var planStatusOrder = []session.PlanStatus{
	session.PlanPending,
	session.PlanInProgress,
	session.PlanBlocked,
	session.PlanCompleted,
	session.PlanCancelled,
	session.PlanSuperseded,
}

// PlanObservation is this engine's own account of the durable plan it runs
// under: the posture, the gate, the policy at its three layers, where the
// plan stands, what the step in progress may do, and the session's counters.
//
// It is assembled from state the harness already holds. The plan snapshot is
// read as the session manager keeps it, the policy is read from the pointers
// the runtime has already published, and the counters are read from the
// telemetry the harness incremented on its own operations. Nothing here
// approves a plan, starts or finishes a step, files evidence, patches a
// snapshot or spends a mutation id — so the plan's revision stands exactly
// where it stood before the question was asked.
//
// What it leaves behind is the whole of what a plan durably holds that a
// read-only view must not carry: the goal, the approach, the working
// context, the success criteria and constraints, every step's text, note,
// evidence, blocker and resume condition, the transition audit trail with
// its reasons, and the mutation ledger with the replay tokens in it.
func (engine *Engine) PlanObservation() diag.PlanState {
	if engine == nil {
		return diag.PlanState{}
	}
	engine.mu.RLock()
	defer engine.mu.RUnlock()

	state := diag.PlanState{
		Known:     true,
		Enabled:   engine.planEnabled,
		Mode:      string(normalizeMode(engine.mode)),
		GatePhase: engine.planPhaseLocked(),
		Policy:    engine.planPolicyLocked(),
	}
	if !engine.planEnabled {
		// No gate means no plan, no step and no counters: reporting a plan a
		// sub-agent cannot address would invent state it does not have.
		return state
	}
	state.Telemetry = planTelemetryFacts(engine.planTelemetryManager())
	state.SkillsPending = len(engine.planSkills)
	if engine.session == nil {
		return state
	}

	plan := engine.session.Plan()
	state.Present = len(plan.Items) > 0
	state.Approved = plan.Approved
	state.Result = string(plan.Result)
	state.Revision = plan.Revision
	state.ContractEpoch = plan.ContractEpoch
	if state.Present {
		state.Schema = planSchemaName(plan.Schema)
	}
	state.StepsTotal = len(plan.Items)
	state.Statuses = planStatusCounts(plan)
	for _, item := range plan.Items {
		if !item.Status.Terminal() {
			state.StepsRemaining++
		}
	}
	state.Step = engine.planStepFactsLocked(plan)
	return state
}

// planPolicyLocked reads the three compiled policies side by side: the one
// this harness ships, the one the runtime has published, and the one baked
// into the prompt and tool schemas the model is currently looking at. The
// third is a pointer the engine kept at its last rebind, so the gap between
// it and the second is a real lag rather than a rounding of the same read.
func (engine *Engine) planPolicyLocked() diag.PlanPolicy {
	policy := diag.PlanPolicy{Default: planPolicySet(plangate.DefaultPolicy())}
	if !engine.planEnabled || engine.planRuntime == nil {
		return policy
	}
	loaded := engine.planRuntime.Current()
	policy.Loaded = planPolicySet(loaded)
	policy.Projected = planPolicySet(engine.projectedPlanPolicy)
	policy.Revision = planPolicyRevision(policy.Loaded)
	return policy
}

// planPolicySet reduces one compiled policy to the names it is made of. A nil
// policy is a layer that does not exist — an engine that has not rebound yet
// has projected nothing — and says so rather than borrowing the built-in.
func planPolicySet(policy *plangate.Policy) diag.PlanPolicySet {
	if policy == nil {
		return diag.PlanPolicySet{}
	}
	defaults := policy.Defaults()
	set := diag.PlanPolicySet{
		Known:      true,
		Types:      defaults.StepTypeNames(),
		Tools:      planPolicyTools(defaults, ""),
		Exemptions: policy.ExemptTools(),
		Authoring:  string(policy.AuthoringPolicy()),
		Actions:    planActionNames(defaults.Actions),
	}
	for _, typ := range defaults.Types {
		if typ.Model == "" {
			continue
		}
		set.TypeModels = append(set.TypeModels, string(typ.Name)+"="+typ.Model)
	}
	return set
}

// planPolicyTools lists the gateable tools a policy assigns, in the order the
// type hierarchy introduces them. An empty target names every assigned tool;
// a named one stops at that type, which is exactly the inherited set a step
// of that type reaches — a type the policy does not know reaches nothing.
func planPolicyTools(defaults plangate.Defaults, target session.StepType) []string {
	tools := make([]string, 0, len(defaults.Types))
	for _, typ := range defaults.Types {
		tools = append(tools, typ.Tools...)
		if target != "" && typ.Name == target {
			return tools
		}
	}
	if target != "" {
		return nil
	}
	return tools
}

// planActionNames renders an action list as "event:type" pairs. The skills an
// inject_skill action names are reported by the step field that can say what
// became of each of them, not here.
func planActionNames(actions []session.PlanAction) []string {
	if len(actions) == 0 {
		return nil
	}
	names := make([]string, 0, len(actions))
	for _, action := range actions {
		names = append(names, string(action.Event)+":"+string(action.Type))
	}
	return names
}

// planStepFactsLocked describes the step in progress. A plan with no step in
// progress — nothing started, or everything finished — reports none rather
// than promoting the next pending step, which the gate has not started and
// the model has not named.
func (engine *Engine) planStepFactsLocked(plan session.Plan) diag.PlanStep {
	index := slices.IndexFunc(plan.Items, func(item session.PlanItem) bool {
		return item.Status == session.PlanInProgress
	})
	if index < 0 {
		return diag.PlanStep{}
	}
	item := plan.Items[index]
	step := diag.PlanStep{
		Known:      true,
		ID:         item.ID,
		Type:       string(item.Type),
		JIT:        item.JIT,
		JITGranted: item.JIT && plan.JITGranted(item.ID),
		Attempts:   len(item.Attempts),
		Actions:    planActionNames(item.Actions),
		Skills:     engine.planStepSkillsLocked(item),
	}
	if engine.planRuntime != nil {
		step.Tools = planPolicyTools(engine.planRuntime.Current().Defaults(), item.Type)
	}
	return step
}

// planStepSkillsLocked accounts for every skill the step names and says what
// became of it: switched off by the user, already delivered to the model,
// loaded and parked for the next boundary, or named and not yet loaded.
//
// It reads the names off the step's own actions and the engine's own queues.
// No skill is looked up in the catalog and no body is read, so a name the
// catalog cannot supply is still accounted for here, and a skill this session
// has delivered in full is still only a name and a state.
func (engine *Engine) planStepSkillsLocked(item session.PlanItem) []diag.PlanSkill {
	var skills []diag.PlanSkill
	queued := make(map[string]struct{}, len(engine.planSkills))
	for _, preload := range engine.planSkills {
		queued[preload.name] = struct{}{}
	}
	for _, action := range item.Actions {
		if action.Type != session.PlanActionInjectSkill {
			continue
		}
		disabled := make(map[string]struct{}, len(action.DisabledSkills))
		for _, name := range action.DisabledSkills {
			disabled[name] = struct{}{}
		}
		for _, name := range action.Skills {
			skills = append(skills, diag.PlanSkill{
				Name:  name,
				State: planSkillState(name, disabled, queued, engine.planSkillsDelivered),
			})
		}
	}
	return skills
}

// planSkillState resolves one name against the three sets that can hold it.
// The order is the order the states supersede one another: a name the user
// switched off is off however far it once got, one already delivered is not
// sent again, and one still parked has not reached the model yet.
func planSkillState(name string, disabled, queued, delivered map[string]struct{}) string {
	if _, off := disabled[name]; off {
		return diag.PlanSkillDisabled
	}
	if _, sent := delivered[name]; sent {
		return diag.PlanSkillDelivered
	}
	if _, parked := queued[name]; parked {
		return diag.PlanSkillQueued
	}
	return diag.PlanSkillPending
}

// planStatusCounts is the step breakdown in the fixed status order. A status
// no step stands in is left out: the answer describes the plan that exists,
// not the vocabulary it could have used.
func planStatusCounts(plan session.Plan) []diag.PlanStatusCount {
	counts := make(map[session.PlanStatus]int, len(planStatusOrder))
	for _, item := range plan.Items {
		counts[item.Status]++
	}
	out := make([]diag.PlanStatusCount, 0, len(planStatusOrder))
	for _, status := range planStatusOrder {
		if counts[status] == 0 {
			continue
		}
		out = append(out, diag.PlanStatusCount{Status: string(status), Count: counts[status]})
	}
	return out
}

// planSchemaName names the authoring contract a stored snapshot was written
// under. A snapshot written before the field existed carries the zero value
// and is loaded as legacy, so it is reported as legacy rather than as a
// schema nobody wrote.
func planSchemaName(schema session.PlanSchema) string {
	if schema.IsV2() {
		return "v2"
	}
	return "legacy"
}

// planTelemetryFacts carries the plan's counters across the diagnostics seam.
// Both sides are fixed numeric schemas and this is the whole of the mapping
// between them, so a counter added to one and not the other is a compile
// error rather than a silent gap.
func planTelemetryFacts(manager *session.Manager) diag.PlanTelemetry {
	if manager == nil {
		return diag.PlanTelemetry{}
	}
	return planTelemetryFrom(manager.PlanTelemetry())
}

func planTelemetryFrom(snapshot plantel.Snapshot) diag.PlanTelemetry {
	return diag.PlanTelemetry{
		Known:               true,
		Misses:              snapshot.PlanMisses,
		TransitionConflicts: snapshot.TransitionConflicts,
		IdempotentRetries:   snapshot.IdempotentRetries,
		StandaloneStarts:    snapshot.StandaloneStarts,
		PlanOnlyRounds:      snapshot.PlanOnlyRounds,

		ApprovalChurn:       snapshot.ApprovalChurn,
		MaterialRevisions:   snapshot.MaterialRevisions,
		MaterialReapprovals: snapshot.MaterialReapprovals,
		ApprovalLatency1s:   snapshot.ApprovalLatency1s,
		ApprovalLatency10s:  snapshot.ApprovalLatency10s,
		ApprovalLatency60s:  snapshot.ApprovalLatency60s,
		ApprovalLatencySlow: snapshot.ApprovalLatencySlow,

		ProjectionInjections: snapshot.ProjectionInjections,
		ProjectionBytes:      snapshot.ProjectionBytes,
		ProjectionBytesLast:  snapshot.ProjectionBytesLast,

		CompletionsSuccess:         snapshot.CompletionsSuccess,
		CompletionsAbandoned:       snapshot.CompletionsAbandoned,
		CompletionsWithoutEvidence: snapshot.CompletionsWithoutEvidence,
		Archives:                   snapshot.Archives,
		ArchiveLatencyLastMS:       snapshot.ArchiveLatencyLastMS,
		ArchiveLatencyMaxMS:        snapshot.ArchiveLatencyMaxMS,

		DraftsAdaptive: snapshot.DraftsAdaptive,
		DraftsLegacy:   snapshot.DraftsLegacy,
		PatchRetries:   snapshot.PatchRetries,
	}
}

// planPolicyRevision fingerprints the published policy: two observations
// carrying the same one were gated by the same policy, which comparing the
// name lists alone cannot establish once a type is renamed back.
func planPolicyRevision(set diag.PlanPolicySet) string {
	digest := fnv.New64a()
	_, _ = digest.Write([]byte(set.Authoring))
	for _, names := range [][]string{set.Types, set.Tools, set.Exemptions, set.TypeModels, set.Actions} {
		_, _ = digest.Write([]byte{0})
		for _, name := range names {
			_, _ = digest.Write([]byte(name))
			_, _ = digest.Write([]byte{0x1f})
		}
	}
	return strconv.FormatUint(digest.Sum64(), 16)
}
