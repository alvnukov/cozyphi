package opencode

import "github.com/alvnukov/cozyphi/internal/diag"

// ImportObservation reports the state of the read-only opencode import for
// the harness view. The caller passes what it knows at the one moment the
// import happens — whether the setting was on, what Load returned, and the
// error it returned — because after startup none of that is recoverable
// without reading the files again, and an observation never reads anything.
//
// The error is taken and dropped on purpose. It names the file it failed on
// and, for a parse failure, can quote what it could not parse; the state
// below says that the import failed, and none of the error's text, and no
// fragment of any file it read, leaves this call.
func ImportObservation(enabled bool, source *Source, loadErr error) diag.ImportFacts {
	switch {
	case !enabled:
		return diag.ImportFacts{Known: true, State: diag.ImportDisabled}
	case loadErr != nil:
		return diag.ImportFacts{Known: true, State: diag.ImportFailed}
	case source == nil:
		// Enabled, no error and nothing to show: the import never ran,
		// because what it resolves against failed before it was reached.
		return diag.ImportFacts{Known: true, State: diag.ImportNotLoaded}
	default:
		return diag.ImportFacts{Known: true, State: diag.ImportLoaded, Models: len(source.models)}
	}
}
