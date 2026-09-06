// Package diag is cozyphi's read-only view of itself.
//
// It answers three questions about the running process — what can be
// observed (Catalog), what is observed now (Snapshot), and where one value
// came from (Explain) — and it answers them without owning any of that
// state. Owners keep their state; a Collector reaches them through accessor
// funcs handed in at construction, so nothing is copied out of its owner and
// held here to go stale.
//
// The package is deliberately UI-independent: it must not import the agent,
// the TUI or the tool packages, so the same contract serves a headless run,
// the TUI and anything later. Collectors stay dumb — they observe and
// return. The Registry does the work that must never be forgotten:
// sanitizing every string that leaves, bounding the answer, detaching every
// slice, and turning a failed collector into one unavailable category rather
// than a failed snapshot. That is the safety net for every collector added
// later: a collector cannot leak by omission.
//
// Nothing here has side effects. Collect spawns no process, opens no socket,
// runs no hook, reloads nothing and scans no filesystem.
package diag

// Category names one area of the harness that can be observed. The catalog
// is fixed: every category exists from the first release, and one without a
// collector says so rather than disappearing.
type Category string

// Category values, in catalog order.
const (
	CategoryRuntime      Category = "runtime"
	CategoryModel        Category = "model"
	CategoryContext      Category = "context"
	CategoryPermissions  Category = "permissions"
	CategoryPlan         Category = "plan"
	CategoryTools        Category = "tools"
	CategoryIntegrations Category = "integrations"
	CategoryAgents       Category = "agents"
	CategoryStorage      Category = "storage"
	CategoryUI           Category = "ui"
	CategoryDiagnostics  Category = "diagnostics"
)

// catalogOrder is the one order every answer follows, so two snapshots of
// the same process are byte-comparable.
var catalogOrder = []Category{
	CategoryRuntime,
	CategoryModel,
	CategoryContext,
	CategoryPermissions,
	CategoryPlan,
	CategoryTools,
	CategoryIntegrations,
	CategoryAgents,
	CategoryStorage,
	CategoryUI,
	CategoryDiagnostics,
}

// Categories returns the catalog in order. The slice is detached: callers
// may sort or trim it without touching package state.
func Categories() []Category {
	out := make([]Category, len(catalogOrder))
	copy(out, catalogOrder)
	return out
}

// ParseCategory validates a user-supplied category name. It does not guess:
// an unknown or differently-cased name is rejected rather than corrected,
// because a wrong category answered with the right-looking data is worse
// than an error.
func ParseCategory(name string) (Category, bool) {
	candidate := Category(name)
	for _, known := range catalogOrder {
		if known == candidate {
			return known, true
		}
	}
	return "", false
}

// Known reports whether the category is part of the catalog.
func (c Category) Known() bool {
	_, ok := ParseCategory(string(c))
	return ok
}
