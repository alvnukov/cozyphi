package keys

import (
	"slices"

	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/editmode"
)

// ObserveConfig projects the keybinds section into the shape the diagnostics
// registry reports: the vocabulary this build understands and which of its
// commands the configuration names. The chord each one is given stays here —
// which keys a command answers to is a question for the help screen, where a
// person can see the answer without a tool being asked for it.
//
// It observes and returns. It compiles nothing, rebinds nothing and reads no
// configuration file: overrides are handed in by whoever already loaded them.
func ObserveConfig(overrides map[string]string) diag.KeybindConfigFacts {
	vocabulary := make([]string, 0, len(commands))
	for _, cmd := range commands {
		vocabulary = append(vocabulary, string(cmd))
	}
	named := make([]string, 0, len(overrides))
	for id := range overrides {
		named = append(named, id)
	}
	slices.Sort(named)
	return diag.KeybindConfigFacts{Known: true, Commands: vocabulary, Overrides: named}
}

// Observe projects the compiled binding table into the same shape: the
// dialect it was built for, which commands took an override, and which ones
// actually ended up spelled differently from this dialect's own defaults —
// switching dialects moves the defaults, so the two lists come apart.
//
// The table and the dialect are package globals owned by the UI goroutine,
// which is where dispatch and every rebind run. Call this from there and hand
// the result over detached; reading them from a tool goroutine would be a
// race.
//
// It observes and returns. It compiles no table, changes no profile and
// dispatches nothing.
func Observe() diag.KeybindRuntimeFacts {
	accepted := make([]string, 0, len(userBinds))
	for id := range userBinds {
		accepted = append(accepted, id)
	}
	slices.Sort(accepted)
	return diag.KeybindRuntimeFacts{
		Known:    true,
		Profile:  activeProfile.String(),
		Accepted: accepted,
		Diverged: diverged(activeProfile),
	}
}

// diverged lists the commands the table in force spells differently from what
// this dialect would have given them. It compares parsed chords rather than
// the specs they were written as, so a spelling difference that parses to the
// same chord is not reported as a rebinding.
func diverged(mode editmode.Mode) []string {
	defaults := profileDefaults(mode)
	out := make([]string, 0, len(userBinds))
	for _, cmd := range commands {
		want, err := parseChordList(defaults[cmd])
		if err != nil || slices.Equal(table[cmd], want) {
			continue
		}
		out = append(out, string(cmd))
	}
	slices.Sort(out)
	return out
}
