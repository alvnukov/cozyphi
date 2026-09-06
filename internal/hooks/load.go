package hooks

import (
	"fmt"

	"github.com/alvnukov/cozyphi/internal/debuglog"
)

// Load discovers hooks under userDir and projectDir and builds a Manager.
// Discovery warnings are returned; only unexpected I/O fails with err.
// When COZYPHI_HOOKS=off, returns an empty Manager and no warnings.
func Load(userDir, projectDir string) (*Manager, []Warning, error) {
	mgr, _, warns, err := LoadObserved(userDir, projectDir)
	return mgr, warns, err
}

// LoadObserved is Load together with a record of what the load saw. The
// Manager keeps only entries, so which directory defined one, whose
// definition it replaced and how many problems the load met survive nowhere
// else — and a caller that wants the harness view to answer those must carry
// the record alongside the manager it built.
func LoadObserved(userDir, projectDir string) (*Manager, LoadFacts, []Warning, error) {
	found, warns, err := Discover(userDir, projectDir)
	facts := ObserveLoad(userDir, projectDir, found, warns, err)
	if err != nil {
		return nil, facts, warns, err
	}
	return NewManager(EntriesFromDiscovered(found)...), facts, warns, nil
}

// LogWarnings writes each warning to the debug log (COZYPHI_DEBUG=1).
func LogWarnings(warns []Warning) {
	for _, w := range warns {
		debuglog.Logf("hooks: %s", w.String())
	}
	if n := len(warns); n > 0 {
		debuglog.Logf("hooks: %d warning(s) while loading", n)
	}
}

// FormatWarningsSummary is a one-line status for stderr / UI.
func FormatWarningsSummary(warns []Warning) string {
	if len(warns) == 0 {
		return ""
	}
	return fmt.Sprintf("hooks: %d warning(s); set COZYPHI_DEBUG=1 for details", len(warns))
}
