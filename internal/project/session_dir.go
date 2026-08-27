package project

import (
	"path/filepath"
	"strings"
)

// ProjectDirName returns a filesystem-safe directory name derived from cwd,
// matching panda's coding-agent:
//
//	--<cwd with leading path sep stripped and / \ : replaced by ->--
func ProjectDirName(cwd string) string {
	s := filepath.Clean(cwd)
	if s == "." {
		return "--unknown--"
	}
	if s != "" && (s[0] == '/' || s[0] == '\\') {
		s = s[1:]
	}
	out := replaceSeparators(s)
	if out == "" {
		out = "unknown"
	}
	return "--" + out + "--"
}

// replaceSeparators turns path and volume separators into hyphens, the encoding
// both ProjectDirName and claudeProjectDirName build on.
func replaceSeparators(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '/', '\\', ':':
			b.WriteByte('-')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// claudeProjectDirName encodes a project path the same way Claude Code names
// entries below ~/.claude/projects: path separators and volume separators are
// hyphens, including the leading separator of an absolute Unix path.
func claudeProjectDirName(root string) string {
	s := filepath.Clean(root)
	if s == "." {
		return "-unknown"
	}
	out := replaceSeparators(s)
	if out == "" {
		return "-unknown"
	}
	return out
}

// ProjectSessionDir is where jsonl session files for cwd live under baseDir
// (e.g. ~/.cozyphi/session/--Users-me-proj--/).
func ProjectSessionDir(baseDir, cwd string) string {
	return filepath.Join(baseDir, ProjectDirName(cwd))
}
