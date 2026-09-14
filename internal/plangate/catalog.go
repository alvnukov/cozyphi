package plangate

import "sort"

// ToolInfo describes one tool understood by the plan gate. Mandatory
// exemptions are shown by editors but cannot be assigned to a step type.
type ToolInfo struct {
	Name               string
	MandatoryExemption bool
}

// KnownTools returns every tool understood by the plan gate in a stable,
// capability-oriented order. The returned slice is detached from package
// state and is safe for callers to edit.
func KnownTools() []ToolInfo {
	out := make([]ToolInfo, 0, len(toolLevel)+len(exemptTools))
	seen := make(map[string]struct{}, cap(out))
	for _, typ := range DefaultDefaults().Types {
		for _, name := range typ.Tools {
			out = append(out, ToolInfo{Name: name})
			seen[name] = struct{}{}
		}
	}

	remaining := make([]string, 0)
	for name := range toolLevel {
		if _, ok := seen[name]; !ok {
			remaining = append(remaining, name)
		}
	}
	sort.Slice(remaining, func(i, j int) bool {
		if toolLevel[remaining[i]] != toolLevel[remaining[j]] {
			return toolLevel[remaining[i]] < toolLevel[remaining[j]]
		}
		return remaining[i] < remaining[j]
	})
	for _, name := range remaining {
		out = append(out, ToolInfo{Name: name})
		seen[name] = struct{}{}
	}

	// The floor is plan alone; the other excludable names stay listed so an
	// editor can toggle their exemption against the shipped defaults.
	for name := range exemptTools {
		out = append(out, ToolInfo{Name: name, MandatoryExemption: true})
		seen[name] = struct{}{}
	}
	remaining = remaining[:0]
	for name := range knownExemptTools {
		if _, ok := seen[name]; !ok {
			remaining = append(remaining, name)
		}
	}
	sort.Strings(remaining)
	for _, name := range remaining {
		out = append(out, ToolInfo{Name: name})
	}
	return out
}
