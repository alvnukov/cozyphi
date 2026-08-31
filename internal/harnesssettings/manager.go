// Package harnesssettings owns editable harness defaults and their durable,
// live application. Callers work with detached snapshots and drafts; YAML merge,
// conflict detection, validation, and atomic publication stay behind Manager.
package harnesssettings

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/alvnukov/cozyphi/internal/configfile"
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/session"
)

var (
	// ErrConflict means plan.defaults changed since the draft was opened.
	ErrConflict = errors.New("harness settings: plan defaults changed on disk")
	// ErrTypeInUse means a draft removes a type still referenced by the current plan.
	ErrTypeInUse = errors.New("harness settings: step type is used by the current plan")
)

// PlanMigrator is the current-session seam needed only for type renames and
// delete validation. Implementations preserve plan approval and all non-type fields.
type PlanMigrator interface {
	Plan() session.Plan
	RenamePlanStepTypes(context.Context, map[session.StepType]session.StepType) (session.Plan, error)
}

// Snapshot is one detached view of global harness settings.
type Snapshot struct {
	Token string
	Path  string
	Plan  plangate.Defaults
	// Compaction carries the user-tuned compaction policy — today, the
	// reminder threshold the engine advises the model at.
	Compaction Compaction
	// AgentModels carries the agents.models pins — role → model name.
	// Empty entries were dropped at load; nil means no pins configured and
	// every role inherits the session model.
	AgentModels map[string]string
}

// Compaction is the compaction section of the config. ReminderTokens is the
// context-token count at which the engine starts advising the model to
// compact; 0 keeps the default — advice starts where compaction used to
// fire on its own.
type Compaction struct {
	ReminderTokens int `yaml:"reminder_tokens"`
}

// Draft returns an independently editable copy of the snapshot, seeded with
// the step types that existed when it was opened (see Draft.RecordRename).
func (s Snapshot) Draft() Draft {
	draft := Draft{
		BaseToken:             s.Token,
		Plan:                  normalizeDefaults(s.Plan),
		CompactReminderTokens: s.Compaction.ReminderTokens,
		AgentModels:           maps.Clone(s.AgentModels),
	}
	draft.openedNames = make(map[session.StepType]struct{}, len(s.Plan.Types))
	for _, typ := range s.Plan.Types {
		draft.openedNames[typ.Name] = struct{}{}
	}
	return draft
}

// Manager owns one config path and publishes successful commits to Runtime.
type Manager struct {
	mu       sync.Mutex
	path     string
	runtime  *plangate.Runtime
	plans    PlanMigrator
	snapshot Snapshot
}

// Open loads plan.defaults (or built-in defaults when absent), validates it,
// and publishes it as the initial live policy.
func Open(path string, runtime *plangate.Runtime, plans PlanMigrator) (*Manager, error) {
	if path == "" {
		return nil, errors.New("harness settings: empty config path")
	}
	if runtime == nil {
		return nil, errors.New("harness settings: nil plan runtime")
	}
	defaultsNode, defaults, err := loadPlanNode(path)
	if err != nil {
		return nil, err
	}
	if err := runtime.Apply(defaults); err != nil {
		return nil, fmt.Errorf("harness settings: publish initial plan defaults: %w", err)
	}
	compactionCfg, err := loadCompaction(path)
	if err != nil {
		return nil, err
	}
	agentModels, err := loadAgentModels(path)
	if err != nil {
		return nil, err
	}
	policy := runtime.Current()
	manager := &Manager{path: path, runtime: runtime, plans: plans}
	manager.snapshot = Snapshot{
		Token: configfile.Token(defaultsNode), Path: path,
		Plan: policy.Defaults(), Compaction: compactionCfg, AgentModels: agentModels,
	}
	return manager, nil
}

// LoadPlanDefaults reads plan.defaults through the same decoder the settings
// manager uses — the single interpretation of the section shared by every
// consumer in the process.
func LoadPlanDefaults(path string) (plangate.Defaults, error) {
	_, defaults, err := loadPlanNode(path)
	return defaults, err
}

// loadPlanNode returns the plan.defaults YAML node and its decoded value. A
// missing file is an empty document, so the node can be nil; decoding decides
// what that means.
func loadPlanNode(path string) (*yaml.Node, plangate.Defaults, error) {
	doc, err := configfile.Read(path)
	if err != nil {
		return nil, plangate.Defaults{}, err
	}
	node := configfile.Lookup(doc, "plan", "defaults")
	defaults, err := decodeDefaults(node)
	if err != nil {
		return nil, plangate.Defaults{}, err
	}
	return node, defaults, nil
}

// Snapshot returns a detached immutable view of the last successful load/apply.
func (m *Manager) Snapshot() Snapshot {
	if m == nil {
		return Snapshot{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return cloneSnapshot(m.snapshot)
}

// Apply validates a complete draft, merges it into the latest YAML document,
// commits owner-only data atomically, then publishes the policy. A same-section
// external edit fails closed; unrelated sections come from the latest file.
func (m *Manager) Apply(ctx context.Context, draft Draft) (Snapshot, error) {
	if m == nil || m.runtime == nil {
		return Snapshot{}, errors.New("harness settings: manager unavailable")
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	if draft.CompactReminderTokens < 0 {
		return Snapshot{}, errors.New("harness settings: compaction reminder_tokens must be >= 0")
	}
	agentModels, err := job.NormalizeModels(draft.AgentModels)
	if err != nil {
		return Snapshot{}, fmt.Errorf("harness settings: %w", err)
	}
	policy, err := plangate.Compile(draft.Plan)
	if err != nil {
		return Snapshot{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	defaults := policy.Defaults()
	var replacement yaml.Node
	if err := replacement.Encode(defaults); err != nil {
		return Snapshot{}, fmt.Errorf("harness settings: encode plan defaults: %w", err)
	}
	// The whole check-migrate-write cycle runs as one configfile.Edit cycle, so
	// the conflict check, the current-plan migration, and the commit see the
	// same document and no other config writer can interleave.
	var rollback func() error
	if err := configfile.Edit(m.path, func(doc *yaml.Node) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if configfile.Token(configfile.Lookup(doc, "plan", "defaults")) != draft.BaseToken {
			return ErrConflict
		}
		if err := setCompaction(doc, draft.CompactReminderTokens); err != nil {
			return err
		}
		if err := setAgentModels(doc, agentModels); err != nil {
			return err
		}
		renames, err := m.validatePlanMigration(defaults, draft.TypeRenames)
		if err != nil {
			return err
		}
		if len(renames) == 0 {
			configfile.Set(doc, &replacement, "plan", "defaults")
			return nil
		}
		if _, err := m.plans.RenamePlanStepTypes(ctx, renames); err != nil {
			return fmt.Errorf("harness settings: migrate current plan step types: %w", err)
		}
		reverse := reverseRenames(renames)
		rollback = func() error {
			if _, err := m.plans.RenamePlanStepTypes(context.Background(), reverse); err != nil {
				return fmt.Errorf("rollback current plan step types: %w", err)
			}
			return nil
		}
		configfile.Set(doc, &replacement, "plan", "defaults")
		return nil
	}); err != nil {
		if rollback != nil {
			if rollbackErr := rollback(); rollbackErr != nil {
				return Snapshot{}, errors.Join(err, rollbackErr)
			}
		}
		return Snapshot{}, err
	}
	// Compilation already succeeded. Publishing after the durable rename makes
	// every observed live policy correspond to a config that reached disk.
	if err := m.runtime.Apply(defaults); err != nil {
		return Snapshot{}, fmt.Errorf("harness settings: publish committed plan defaults: %w", err)
	}
	committedNode := &replacement
	m.snapshot = Snapshot{
		Token: configfile.Token(committedNode), Path: m.path,
		Plan: m.runtime.Current().Defaults(), Compaction: Compaction{ReminderTokens: draft.CompactReminderTokens},
		AgentModels: agentModels,
	}
	return cloneSnapshot(m.snapshot), nil
}

func (m *Manager) validatePlanMigration(
	defaults plangate.Defaults,
	requested map[session.StepType]session.StepType,
) (map[session.StepType]session.StepType, error) {
	oldTypes := make(map[session.StepType]struct{}, len(m.snapshot.Plan.Types))
	for _, typ := range m.snapshot.Plan.Types {
		oldTypes[typ.Name] = struct{}{}
	}
	newTypes := make(map[session.StepType]struct{}, len(defaults.Types))
	for _, typ := range defaults.Types {
		newTypes[typ.Name] = struct{}{}
	}
	seenTargets := make(map[session.StepType]struct{}, len(requested))
	for from, to := range requested {
		if _, ok := oldTypes[from]; !ok {
			return nil, fmt.Errorf("harness settings: rename source %q is not a current step type", from)
		}
		if _, remains := newTypes[from]; remains {
			return nil, fmt.Errorf("harness settings: renamed step type %q still exists in the draft", from)
		}
		if _, ok := newTypes[to]; !ok {
			return nil, fmt.Errorf("harness settings: rename target %q is not configured", to)
		}
		if _, duplicate := seenTargets[to]; duplicate {
			return nil, fmt.Errorf("harness settings: more than one step type renames to %q", to)
		}
		seenTargets[to] = struct{}{}
	}
	if m.plans == nil {
		return nil, nil
	}
	used := make(map[session.StepType]session.StepType)
	for _, item := range m.plans.Plan().Items {
		if _, stillConfigured := newTypes[item.Type]; stillConfigured {
			continue
		}
		to, renamed := requested[item.Type]
		if !renamed {
			return nil, fmt.Errorf("%w: %q", ErrTypeInUse, item.Type)
		}
		used[item.Type] = to
	}
	return used, nil
}

func reverseRenames(renames map[session.StepType]session.StepType) map[session.StepType]session.StepType {
	reverse := make(map[session.StepType]session.StepType, len(renames))
	for from, to := range renames {
		reverse[to] = from
	}
	return reverse
}

// loadCompaction reads the compaction section. A missing file or section is
// "not configured": the default policy applies.
func loadCompaction(path string) (Compaction, error) {
	doc, err := configfile.Read(path)
	if err != nil {
		return Compaction{}, err
	}
	node := configfile.Lookup(doc, "compaction")
	var cfg Compaction
	if node == nil || node.Tag == "!!null" {
		return cfg, nil
	}
	if err := node.Decode(&cfg); err != nil {
		return Compaction{}, fmt.Errorf("harness settings: decode compaction: %w", err)
	}
	if cfg.ReminderTokens < 0 {
		return Compaction{}, errors.New("harness settings: compaction reminder_tokens must be >= 0")
	}
	return cfg, nil
}

// setCompaction writes the compaction section inside a configfile.Edit cycle.
func setCompaction(doc *yaml.Node, reminderTokens int) error {
	var node yaml.Node
	if err := node.Encode(Compaction{ReminderTokens: reminderTokens}); err != nil {
		return fmt.Errorf("harness settings: encode compaction: %w", err)
	}
	configfile.Set(doc, &node, "compaction")
	return nil
}

// loadAgentModels reads the agents.models pins. Role keys fail closed here
// the same way they do at project load; model names are resolved at spawn
// time, not here.
func loadAgentModels(path string) (map[string]string, error) {
	doc, err := configfile.Read(path)
	if err != nil {
		return nil, err
	}
	node := configfile.Lookup(doc, "agents", "models")
	if node == nil || node.Tag == "!!null" {
		return nil, nil
	}
	var models map[string]string
	if err := node.Decode(&models); err != nil {
		return nil, fmt.Errorf("harness settings: decode agents.models: %w", err)
	}
	models, err = job.NormalizeModels(models)
	if err != nil {
		return nil, fmt.Errorf("harness settings: %w", err)
	}
	return models, nil
}

// setAgentModels writes the agents.models pins inside a configfile.Edit
// cycle. Only the pins live here — agents.enabled belongs to the command
// palette and is left untouched. An empty set removes the section.
func setAgentModels(doc *yaml.Node, models map[string]string) error {
	if len(models) == 0 {
		configfile.Remove(doc, "agents", "models")
		return nil
	}
	var node yaml.Node
	if err := node.Encode(models); err != nil {
		return fmt.Errorf("harness settings: encode agents.models: %w", err)
	}
	configfile.Set(doc, &node, "agents", "models")
	return nil
}

func cloneSnapshot(snapshot Snapshot) Snapshot {
	snapshot.Plan = normalizeDefaults(snapshot.Plan)
	return snapshot
}

// normalizeDefaults re-canonicalizes defaults through the policy compiler so
// every copy the manager hands out carries the same shape Compile produced.
// Input that cannot compile (only possible for hand-built zero values) is
// returned unchanged to keep Snapshot/Draft total.
func normalizeDefaults(defaults plangate.Defaults) plangate.Defaults {
	policy, err := plangate.Compile(defaults)
	if err != nil {
		return defaults
	}
	return policy.Defaults()
}

// decodeDefaults reads the plan.defaults node. A missing or null node means
// "not configured" and yields the built-in defaults — the same reading
// LoadPlanDefaults gives the same file; an explicit `types: []` is a real
// zero-type policy and stays zero.
func decodeDefaults(node *yaml.Node) (plangate.Defaults, error) {
	if node == nil || node.Tag == "!!null" {
		return plangate.DefaultDefaults(), nil
	}
	var defaults plangate.Defaults
	if err := node.Decode(&defaults); err != nil {
		return plangate.Defaults{}, fmt.Errorf("harness settings: decode plan defaults: %w", err)
	}
	if _, err := plangate.Compile(defaults); err != nil {
		return plangate.Defaults{}, err
	}
	return defaults, nil
}
