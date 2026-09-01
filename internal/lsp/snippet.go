package lsp

import (
	"path/filepath"
	"strings"
)

// attachSnippets fills each location's Snippet with the trimmed, bounded
// source line so the model reads the answer without a follow-up file read.
// It runs after finalize: snippets never participate in dedup identity. A
// file that cannot be read (gone, too large) simply leaves its snippets
// empty — the locations stay correct without them.
func (m *Manager) attachSnippets(locs []Location) {
	cache := map[string][]string{}
	for i := range locs {
		locs[i].Snippet = m.snippetLine(cache, locs[i].File, locs[i].Line)
	}
}

// attachCallSnippets does the same for call-site locations.
func (m *Manager) attachCallSnippets(calls []Call) {
	cache := map[string][]string{}
	for i := range calls {
		calls[i].Location.Snippet = m.snippetLine(cache, calls[i].Location.File, calls[i].Location.Line)
	}
}

// snippetLine returns the trimmed bounded 1-based line of the
// workspace-relative file, caching whole files per result set.
func (m *Manager) snippetLine(cache map[string][]string, file string, line int) string {
	lines, ok := cache[file]
	if !ok {
		raw, err := readSnapshot(filepath.Join(m.workspace, filepath.FromSlash(file)))
		if err == nil {
			lines = strings.Split(string(raw), "\n")
		}
		cache[file] = lines
	}
	if line < 1 || line > len(lines) {
		return ""
	}
	snippet, _ := boundText(strings.TrimSpace(lines[line-1]))
	return snippet
}
