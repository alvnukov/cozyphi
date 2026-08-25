package prompt

import (
	"os"
	"path/filepath"
	"strings"
)

// contextFileCandidates are checked in order within a single directory;
// the first readable match wins.
var contextFileCandidates = []string{
	"AGENTS.md",
	"AGENTS.MD",
	"CLAUDE.md",
	"CLAUDE.MD",
}

// ContextFile is one loaded project-instruction file.
type ContextFile struct {
	Path    string
	Content string
}

func loadContextFileFromDir(dir string) *ContextFile {
	for _, name := range contextFileCandidates {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		return &ContextFile{Path: path, Content: string(data)}
	}
	return nil
}

// loadProjectContextFiles discovers AGENTS.md / CLAUDE.md in the workspace:
//  1. global agent dir (~/.cozyphi) first
//  2. then every ancestor from filesystem root down to cwd (cwd last)
//
// Each directory contributes at most one file. Paths are deduped.
func loadProjectContextFiles(cwd, agentDir string) []ContextFile {
	seen := make(map[string]struct{})
	var out []ContextFile

	add := func(f *ContextFile) {
		if f == nil {
			return
		}
		key := f.Path
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, *f)
	}

	if agentDir != "" {
		if abs, err := filepath.Abs(agentDir); err == nil {
			agentDir = abs
		}
		add(loadContextFileFromDir(agentDir))
	}

	if cwd == "" {
		return out
	}
	if abs, err := filepath.Abs(cwd); err == nil {
		cwd = abs
	}

	var ancestors []ContextFile
	dir := cwd
	for {
		if f := loadContextFileFromDir(dir); f != nil {
			if _, ok := seen[f.Path]; !ok {
				seen[f.Path] = struct{}{}
				ancestors = append([]ContextFile{*f}, ancestors...)
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	out = append(out, ancestors...)
	return out
}

func formatProjectContext(files []ContextFile) string {
	if len(files) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("<project_context>\n\n")
	sb.WriteString("Project-specific instructions and guidelines:\n\n")
	for _, f := range files {
		sb.WriteString("<project_instructions path=\"")
		sb.WriteString(f.Path)
		sb.WriteString("\">\n")
		sb.WriteString(f.Content)
		if !strings.HasSuffix(f.Content, "\n") {
			sb.WriteByte('\n')
		}
		sb.WriteString("</project_instructions>\n\n")
	}
	sb.WriteString("</project_context>")
	return sb.String()
}

func cozyPhiAgentDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".cozyphi")
}
