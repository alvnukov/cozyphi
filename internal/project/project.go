package project

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
)

// GlobalLayout describes the global cozyphi home directory (~/.cozyphi).
type GlobalLayout struct {
	root string
}

// Root returns the global cozyphi home directory (~/.cozyphi).
func (g GlobalLayout) Root() string { return g.root }

// ConfigFile returns the path to the global config file.
func (g GlobalLayout) ConfigFile() string { return filepath.Join(g.root, "config.yaml") }

// UIStateFile returns the owner-local persisted TUI preferences file.
func (g GlobalLayout) UIStateFile() string { return filepath.Join(g.root, "ui.json") }

// UsageFile returns the owner-local picker usage history file.
func (g GlobalLayout) UsageFile() string { return filepath.Join(g.root, "usage.json") }

// ProviderCatalogFile returns the last-known-good provider catalog cache.
func (g GlobalLayout) ProviderCatalogFile() string { return filepath.Join(g.root, "providers.json") }

// CredentialsFile returns the owner-local provider credential store.
func (g GlobalLayout) CredentialsFile() string { return filepath.Join(g.root, "credentials.json") }

// LSPConfigFile returns the owner-controlled global LSP server config
// (~/.cozyphi/lsp.json). Project-local .cozyphi/lsp.json is intentionally
// unsupported: server argv, env, and settings are owner data, never project
// data, and must never travel with a repository.
func (g GlobalLayout) LSPConfigFile() string { return filepath.Join(g.root, "lsp.json") }

// BinDir returns the directory for downloaded tool binaries.
func (g GlobalLayout) BinDir() string { return filepath.Join(g.root, "bin") }

// LookBin returns name from BinDir if present, otherwise PATH.
func (g GlobalLayout) LookBin(name string) (string, error) {
	custom := filepath.Join(g.BinDir(), name)
	if _, err := os.Stat(custom); err == nil {
		return custom, nil
	}
	p, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s is not available: install to ~/.cozyphi/bin or PATH", name)
	}
	return p, nil
}

// SkillsDir returns the directory for SKILL.md files.
func (g GlobalLayout) SkillsDir() string { return filepath.Join(g.root, "skills") }

// HooksDir returns the directory for hook plugins (plugin.json).
func (g GlobalLayout) HooksDir() string { return filepath.Join(g.root, "hooks") }

// SessionBase returns the root directory for persisted sessions.
func (g GlobalLayout) SessionBase() string { return filepath.Join(g.root, "session") }

// JobsDir returns the directory for sub-agent job artifacts.
func (g GlobalLayout) JobsDir() string { return filepath.Join(g.root, "jobs") }

func (g GlobalLayout) claudeProjectsDir() string {
	return filepath.Join(filepath.Dir(g.root), ".claude", "projects")
}

// SessionDir returns the per-cwd session storage directory
// (~/.cozyphi/session/<encoded-cwd>/), matching panda's layout.
func (p *Project) SessionDir() string {
	return ProjectSessionDir(p.global.SessionBase(), p.root)
}

// MemoryDir returns Claude Code's auto-memory directory for the project.
// Cozyphi and Claude Code read and write the same MEMORY.md and topic files.
func (p *Project) MemoryDir() string {
	root := p.memoryRoot
	if root == "" {
		root = p.root
	}
	return filepath.Join(p.global.claudeProjectsDir(), claudeProjectDirName(root), "memory")
}

// JobsDir returns ~/.cozyphi/jobs for sub-agent job artifacts.
func (p *Project) JobsDir() string {
	return p.global.JobsDir()
}

// HooksDir returns <root>/.cozyphi/hooks, the per-project hooks directory
// (user hooks live under Global().HooksDir()).
func (p *Project) HooksDir() string {
	return filepath.Join(p.root, ".cozyphi", "hooks")
}

// MCPConfigFile returns <root>/.cozyphi/mcp.json, the per-project MCP config
// file (the user config is ~/.cozyphi/mcp.json).
func (p *Project) MCPConfigFile() string {
	return filepath.Join(p.root, ".cozyphi", "mcp.json")
}

// Project is the resolved cozyphi workspace: the current working directory plus
// the global layout and its loaded configuration.
type Project struct {
	root       string
	memoryRoot string
	global     GlobalLayout
	// config swaps atomically: LoadConfig may run while a sub-agent runner
	// goroutine reads Config() through the spawn seam.
	config atomic.Pointer[Config]
}

// Root returns the working directory the project was resolved from.
func (p *Project) Root() string { return p.root }

// Global returns the global cozyphi layout (~/.cozyphi).
func (p *Project) Global() GlobalLayout { return p.global }

// Config returns the loaded configuration, or nil before LoadConfig.
func (p *Project) Config() *Config { return p.config.Load() }

// LoadConfig reads, env-overrides and finalizes the global configuration.
// The result is cached on the Project until the next LoadConfig call.
func (p *Project) LoadConfig() error {
	cfg, err := loadConfig(p.global)
	if err != nil {
		return err
	}
	p.config.Store(cfg)
	return nil
}

// ensureGlobalDirs creates the global cozyphi home directories. Claude memory
// stays under ~/.claude and is created when the memory store opens.
func ensureGlobalDirs(global GlobalLayout) error {
	dirs := []string{
		global.Root(),
		global.BinDir(),
		global.SkillsDir(),
		global.HooksDir(),
		global.SessionBase(),
		global.JobsDir(),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create directory %q: %w", dir, err)
		}
	}
	return nil
}

// claudeMemoryRoot follows Claude Code's repository scope: subdirectories and
// linked worktrees use the main repository's auto-memory directory. Outside a
// Git repository, memory stays scoped to the project root passed to Discover.
func claudeMemoryRoot(startDir string) string {
	cmd := exec.CommandContext(
		context.Background(),
		"git",
		"-C",
		startDir,
		"rev-parse",
		"--path-format=absolute",
		"--git-common-dir",
	)
	output, err := cmd.Output()
	if err != nil {
		return startDir
	}
	commonDir := filepath.Clean(strings.TrimSpace(string(output)))
	if filepath.Base(commonDir) != ".git" {
		return startDir
	}
	return filepath.Dir(commonDir)
}

// Discover resolves the cozyphi workspace starting from startDir ("" = cwd) and
// ensures the global directory layout exists.
func Discover(startDir string) (*Project, error) {
	if startDir == "" {
		var err error
		startDir, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	absRoot, err := filepath.Abs(startDir)
	if err != nil {
		return nil, err
	}
	global := GlobalLayout{root: filepath.Join(home, ".cozyphi")}
	if err := ensureGlobalDirs(global); err != nil {
		return nil, err
	}
	return &Project{root: absRoot, memoryRoot: claudeMemoryRoot(absRoot), global: global}, nil
}
