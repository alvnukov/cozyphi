package usage

import (
	"fmt"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// Observe reports the history's state for the harness view: whether a file
// backs it, whether that file loaded, and how many items each scope
// remembers.
//
// It reads what the store already holds, under the store's own lock, and
// does nothing else. The file is neither re-read nor written, no use is
// recorded, nothing is pruned and no ranking runs — which is why asking
// cannot change how the next picker orders itself.
//
// No item key leaves, and that matters more here than the usual care: a
// memory is keyed by the directory of the corpus it belongs to, and a model
// or a command by a name someone typed. What leaves is states and counts.
func Observe(s *Store) diag.UsageStoreFacts {
	facts := diag.UsageStoreFacts{
		Known:      s != nil,
		Scopes:     []diag.UsageScopeFacts{},
		Vocabulary: Scopes(),
	}
	if s == nil {
		return facts
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	facts.Persistent = s.path != ""
	facts.OpenFailed = s.openFailed
	for _, scope := range scopeOrder {
		items := len(s.entries[scope])
		facts.Items += items
		facts.Scopes = append(facts.Scopes, diag.UsageScopeFacts{Scope: scope, Items: items})
	}
	facts.Revision = fmt.Sprintf("i%d.f%t", facts.Items, facts.OpenFailed)
	return facts
}
