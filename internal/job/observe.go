package job

import (
	"fmt"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// OwnerFacts is what the session knows about its own place in the job layer
// and the Manager cannot answer for it: whether the configuration leaves
// sub-agents on at all, which owner identity its assignments are admitted
// under, how deep it sits, what role it runs as, and what the agents.models
// pins resolve to against the catalog it can reach.
//
// The pins arrive already resolved because resolving one is a lookup in a
// model catalog, and a catalog is not something this package has or an
// observation may build. The session's own resolver settles them — the same
// one a spawn goes through — so the answer describes what would actually
// happen rather than a second opinion about it.
type OwnerFacts struct {
	// Enabled is whether the configuration leaves sub-agents on. A session
	// with it off carries no spawn tool at all.
	Enabled bool
	// OwnerID is the identity this session's assignments are admitted under.
	// Empty is the unscoped legacy identity the tool layer itself uses, so
	// what is reported is exactly the scope the tools are bound to and never
	// wider.
	OwnerID string
	// Depth is how deep this session sits: zero for one the process opened,
	// one for a sub-agent.
	Depth int
	// Role is the role this session runs under. Empty for a session the
	// process opened rather than a spawn.
	Role Role
	// Pins is one entry per role, resolved by the session's own resolver.
	Pins []diag.RolePin
}

// Observe reports the sub-agent layer's state for the harness view: whether
// sub-agents are on, whether a manager is in force, the roles and pins a
// spawn resolves against, the ceilings it is admitted under, and what this
// session has out right now.
//
// It reads the manager's own in-memory record under the manager's lock, and
// does nothing else. No job is spawned, cancelled, recovered or waited for;
// no job directory, meta file, result or event log is read; no slot is taken
// and no admission is made. Only live assignments are described, because
// only live assignments are in memory: a finished job is a file on disk, and
// an observation does not read disk.
//
// Nothing that could carry a secret is copied out: not a prompt, a
// description, a summary, a result, a stop reason, a working directory, a
// job id, a child session id or the text of an error. What leaves is a role,
// a status, a model name and a handful of counts.
func Observe(m *Manager, owner OwnerFacts) diag.AgentsState {
	state := diag.AgentsState{
		Known:         true,
		Enabled:       owner.Enabled,
		Roles:         roleNames(),
		Role:          string(owner.Role),
		Pins:          owner.Pins,
		Depth:         owner.Depth,
		MaxDepth:      defaultedDepth(0),
		MaxConcurrent: defaultedConcurrency(0),
		Assignments:   []diag.AssignmentFacts{},
	}
	if m == nil {
		state.Revision = ownerRevision(state)
		return state
	}
	state.Managed = true
	state.MaxDepth = m.maxDepth
	state.MaxConcurrent = m.maxConcurrent
	state.LiveProcess, state.LiveSession, state.Assignments = m.observeOwner(owner.OwnerID)
	state.Revision = ownerRevision(state)
	return state
}

// observeOwner is one locked read of everything the manager can say about
// one owner: how many assignments the process has out altogether, how many
// of them are this owner's — an admission still waiting on its slot
// included, exactly as [Manager.LiveCountForOwner] counts it — and what each
// of this owner's live jobs is doing.
//
// One read rather than three so the three answers describe one moment: a
// spawn landing between two locked reads would otherwise produce a count
// that no list accounts for.
func (m *Manager) observeOwner(ownerID string) (process, session int, assignments []diag.AssignmentFacts) {
	m.mu.Lock()
	defer m.mu.Unlock()
	assignments = make([]diag.AssignmentFacts, 0, len(m.jobs))
	process = len(m.jobs)
	for _, lj := range m.jobs {
		if lj.meta.OwnerID != ownerID {
			continue
		}
		session++
		assignments = append(assignments, diag.AssignmentFacts{
			Role:   string(NormalizeRole(string(lj.meta.Role))),
			Status: string(lj.meta.Status),
		})
	}
	for _, pending := range m.pending {
		if pending.ownerID == ownerID {
			session++
		}
	}
	return process, session, assignments
}

// roleNames is every sub-agent role this build offers, in canonical order.
// It is read from [Roles] rather than kept as a copy, so the harness view and
// the spawn validator cannot drift apart.
func roleNames() []string {
	roles := Roles()
	out := make([]string, 0, len(roles))
	for _, role := range roles {
		out = append(out, string(role))
	}
	return out
}

// defaultedDepth and defaultedConcurrency are the ceilings a process with no
// manager would get if it built one. Reporting them is not a guess about a
// manager that does not exist: they are what [New] applies, and a reader
// asking why a spawn was refused is better served by the rule than by a
// blank.
func defaultedDepth(configured int) int {
	if configured <= 0 {
		return defaultMaxDepth
	}
	return configured
}

func defaultedConcurrency(configured int) int {
	if configured <= 0 {
		return defaultMaxConcurrent
	}
	return configured
}

// ownerRevision fingerprints what this observation describes: how many pins
// the configuration declares, how many of them still resolve, how many
// assignments the process has out and how many are this session's. Nothing
// in this package counts these — it is only a way to see that two snapshots
// taken across a spawn are of two different states.
func ownerRevision(state diag.AgentsState) string {
	pinned, resolved := 0, 0
	for _, pin := range state.Pins {
		if pin.Ref != "" {
			pinned++
		}
		if pin.Resolved {
			resolved++
		}
	}
	return fmt.Sprintf("p%d.r%d.l%d.s%d", pinned, resolved, state.LiveProcess, state.LiveSession)
}
