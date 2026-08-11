// Package project provides the phi workspace layout and configuration.
//
// Discover creates the global phi home (~/.phi) with its standard
// subdirectories (bin, skills, hooks, session, jobs) so downloaded tool
// binaries, SKILL.md files, hook manifests, and persisted sessions have a
// known home. This mirrors panda's internal/project: startup ensures the
// layout exists, then tools such as fd/ripgrep are downloaded into the bin
// directory when missing.
package project

import (
	"fmt"
	"os"
	"path/filepath"
)

// GlobalLayout describes the global phi home directory (~/.phi).
type GlobalLayout struct {
	root string
}

func (g GlobalLayout) Root() string        { return g.root }
func (g GlobalLayout) ConfigFile() string  { return filepath.Join(g.root, "config.yaml") }
func (g GlobalLayout) BinDir() string      { return filepath.Join(g.root, "bin") }
func (g GlobalLayout) SkillsDir() string   { return filepath.Join(g.root, "skills") }
func (g GlobalLayout) HooksDir() string    { return filepath.Join(g.root, "hooks") }
func (g GlobalLayout) SessionBase() string { return filepath.Join(g.root, "session") }
func (g GlobalLayout) JobsDir() string     { return filepath.Join(g.root, "jobs") }

// SessionDir returns the per-cwd session storage directory
// (~/.phi/session/<encoded-cwd>/), matching panda's layout.
func (p *Project) SessionDir() string {
	return ProjectSessionDir(p.global.SessionBase(), p.root)
}

// JobsDir returns ~/.phi/jobs for sub-agent job artifacts.
func (p *Project) JobsDir() string {
	return p.global.JobsDir()
}

// Project is the resolved phi workspace: the current working directory plus
// the global layout and its loaded configuration.
type Project struct {
	root   string
	global GlobalLayout
	config *Config
}

func (p *Project) Root() string         { return p.root }
func (p *Project) Global() GlobalLayout { return p.global }
func (p *Project) Config() *Config      { return p.config }

// LoadConfig reads, env-overrides and finalizes the global configuration.
// The result is cached on the Project until the next LoadConfig call.
func (p *Project) LoadConfig() error {
	cfg, err := loadConfig(p.global)
	if err != nil {
		return err
	}
	p.config = cfg
	return nil
}

// ensureGlobalDirs creates the global phi home directories. It is what makes
// ~/.phi/{bin,skills,hooks,session,jobs} exist from the very first startup.
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

// Discover resolves the phi workspace starting from startDir ("" = cwd) and
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
	global := GlobalLayout{root: filepath.Join(home, ".phi")}
	if err := ensureGlobalDirs(global); err != nil {
		return nil, err
	}
	return &Project{root: absRoot, global: global}, nil
}
