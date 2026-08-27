// Package harnesssettings owns editable harness defaults and their durable,
// live application. Callers work with detached snapshots and drafts; YAML merge,
// conflict detection, validation, and atomic publication stay behind Manager.
package harnesssettings

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"

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
}

// Draft returns an independently editable copy of the snapshot.
func (s Snapshot) Draft() Draft {
	return Draft{BaseToken: s.Token, Plan: normalizeDefaults(s.Plan)}
}

// Draft is submitted to Apply. BaseToken scopes optimistic concurrency to the
// plan.defaults YAML section, so unrelated edits can merge automatically.
// TypeRenames carries explicit UI intent for current-plan migration; it is not
// persisted as configuration.
type Draft struct {
	BaseToken   string
	Plan        plangate.Defaults
	TypeRenames map[session.StepType]session.StepType
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
	doc, err := readDocument(path)
	if err != nil {
		return nil, err
	}
	defaultsNode := lookupPath(doc, "plan", "defaults")
	defaults, err := decodeDefaults(defaultsNode)
	if err != nil {
		return nil, err
	}
	if err := runtime.Apply(defaults); err != nil {
		return nil, fmt.Errorf("harness settings: publish initial plan defaults: %w", err)
	}
	policy := runtime.Current()
	manager := &Manager{path: path, runtime: runtime, plans: plans}
	manager.snapshot = Snapshot{Token: nodeToken(defaultsNode), Path: path, Plan: policy.Defaults()}
	return manager, nil
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
	policy, err := plangate.Compile(draft.Plan)
	if err != nil {
		return Snapshot{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	doc, err := readDocument(m.path)
	if err != nil {
		return Snapshot{}, err
	}
	currentNode := lookupPath(doc, "plan", "defaults")
	if nodeToken(currentNode) != draft.BaseToken {
		return Snapshot{}, ErrConflict
	}
	defaults := policy.Defaults()
	renames, err := m.validatePlanMigration(defaults, draft.TypeRenames)
	if err != nil {
		return Snapshot{}, err
	}
	var reverse map[session.StepType]session.StepType
	if len(renames) > 0 {
		if _, err := m.plans.RenamePlanStepTypes(ctx, renames); err != nil {
			return Snapshot{}, fmt.Errorf("harness settings: migrate current plan step types: %w", err)
		}
		reverse = reverseRenames(renames)
	}
	rollbackPlan := func(cause error) error {
		if len(reverse) == 0 {
			return cause
		}
		if _, rollbackErr := m.plans.RenamePlanStepTypes(context.Background(), reverse); rollbackErr != nil {
			return errors.Join(cause, fmt.Errorf("rollback current plan step types: %w", rollbackErr))
		}
		return cause
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, rollbackPlan(err)
	}
	var replacement yaml.Node
	if err := replacement.Encode(defaults); err != nil {
		return Snapshot{}, rollbackPlan(fmt.Errorf("harness settings: encode plan defaults: %w", err))
	}
	setPath(doc, &replacement, "plan", "defaults")
	data, err := encodeDocument(doc)
	if err != nil {
		return Snapshot{}, rollbackPlan(err)
	}
	if err := writeAtomicOwnerOnly(m.path, data); err != nil {
		return Snapshot{}, rollbackPlan(err)
	}
	// Compilation already succeeded. Publishing after the durable rename makes
	// every observed live policy correspond to a config that reached disk.
	if err := m.runtime.Apply(defaults); err != nil {
		return Snapshot{}, fmt.Errorf("harness settings: publish committed plan defaults: %w", err)
	}
	committedNode := lookupPath(doc, "plan", "defaults")
	m.snapshot = Snapshot{Token: nodeToken(committedNode), Path: m.path, Plan: m.runtime.Current().Defaults()}
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
// project.parseConfigFile gives the same file; an explicit `types: []` is a
// real zero-type policy and stays zero.
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

func readDocument(path string) (*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("harness settings: read %s: %w", path, err)
		}
		return emptyDocument(), nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("harness settings: parse %s: %w", path, err)
	}
	if len(doc.Content) == 0 {
		return emptyDocument(), nil
	}
	if doc.Kind != yaml.DocumentNode || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("harness settings: config %s must be a YAML mapping", path)
	}
	return &doc, nil
}

func emptyDocument() *yaml.Node {
	return &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
}

func lookupPath(doc *yaml.Node, path ...string) *yaml.Node {
	if doc == nil || len(doc.Content) == 0 {
		return nil
	}
	current := doc.Content[0]
	for _, key := range path {
		if current.Kind != yaml.MappingNode {
			return nil
		}
		var next *yaml.Node
		for i := 0; i+1 < len(current.Content); i += 2 {
			if current.Content[i].Value == key {
				next = current.Content[i+1]
				break
			}
		}
		if next == nil {
			return nil
		}
		current = next
	}
	return current
}

func setPath(doc, value *yaml.Node, path ...string) {
	current := doc.Content[0]
	for i, key := range path {
		last := i == len(path)-1
		var child *yaml.Node
		for j := 0; j+1 < len(current.Content); j += 2 {
			if current.Content[j].Value != key {
				continue
			}
			if last {
				current.Content[j+1] = value
				return
			}
			child = current.Content[j+1]
			break
		}
		if child == nil {
			keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
			if last {
				current.Content = append(current.Content, keyNode, value)
				return
			}
			child = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			current.Content = append(current.Content, keyNode, child)
		}
		if child.Kind != yaml.MappingNode {
			child.Kind = yaml.MappingNode
			child.Tag = "!!map"
			child.Content = nil
		}
		current = child
	}
}

func nodeToken(node *yaml.Node) string {
	var data []byte
	if node != nil {
		data, _ = yaml.Marshal(node)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func encodeDocument(doc *yaml.Node) ([]byte, error) {
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(doc); err != nil {
		return nil, fmt.Errorf("harness settings: encode config: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("harness settings: finish config: %w", err)
	}
	return out.Bytes(), nil
}

func writeAtomicOwnerOnly(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("harness settings: create config directory: %w", err)
	}
	file, err := os.CreateTemp(dir, ".config-*.yaml")
	if err != nil {
		return fmt.Errorf("harness settings: create temporary config: %w", err)
	}
	tmp := file.Name()
	defer os.Remove(tmp)
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return fmt.Errorf("harness settings: protect temporary config: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("harness settings: write temporary config: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("harness settings: sync temporary config: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("harness settings: close temporary config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("harness settings: replace config: %w", err)
	}
	return nil
}
