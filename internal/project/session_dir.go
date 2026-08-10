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
	if len(s) > 0 && (s[0] == '/' || s[0] == '\\') {
		s = s[1:]
	}
	var b strings.Builder
	b.Grow(len(s) + 4)
	for _, r := range s {
		switch r {
		case '/', '\\', ':':
			b.WriteByte('-')
		default:
			b.WriteRune(r)
		}
	}
	out := b.String()
	if out == "" {
		out = "unknown"
	}
	return "--" + out + "--"
}

// ProjectSessionDir is where jsonl session files for cwd live under baseDir
// (e.g. ~/.phi/session/--Users-me-proj--/).
func ProjectSessionDir(baseDir, cwd string) string {
	return filepath.Join(baseDir, ProjectDirName(cwd))
}
