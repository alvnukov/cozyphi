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

// Instruction scopes name where a project-instruction file was found,
// without naming the directory it was found in. They are what a read-only
// observer may be told about the prompt's inputs: how far a file's authority
// reaches, not where on the user's disk it lives.
const (
	// ScopeAgentDir is the global agent directory (~/.cozyphi): instructions
	// in force for every workspace this user opens.
	ScopeAgentDir = "agent_dir"
	// ScopeAncestor is a directory above the working one: instructions a
	// parent project sets for everything under it.
	ScopeAncestor = "ancestor"
	// ScopeWorkspace is the working directory itself.
	ScopeWorkspace = "workspace"
)

// ContextFile is one loaded project-instruction file.
type ContextFile struct {
	Path    string
	Content string
	// Scope names where the file was found — one of the scopes above. It is
	// carried so the prompt's facts can report the shape of what was loaded
	// without carrying the path.
	Scope string
}

func loadContextFileFromDir(dir, scope string) *ContextFile {
	for _, name := range contextFileCandidates {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		return &ContextFile{Path: path, Content: string(data), Scope: scope}
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
		add(loadContextFileFromDir(agentDir, ScopeAgentDir))
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
		scope := ScopeAncestor
		if dir == cwd {
			scope = ScopeWorkspace
		}
		if f := loadContextFileFromDir(dir, scope); f != nil {
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

// contextScopes lists where each loaded file was found, in load order. It is
// the safe half of the file list: the scopes say what kind of authority the
// prompt took in, and the paths stay behind.
func contextScopes(files []ContextFile) []string {
	scopes := make([]string, 0, len(files))
	for _, f := range files {
		scopes = append(scopes, f.Scope)
	}
	return scopes
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
