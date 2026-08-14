package hooks

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Source labels where a discovered hook came from.
const (
	SourceUser    = "user"
	SourceProject = "project"
)

// EnvHooks is the environment variable that disables or filters hooks.
// Value "off" (case-insensitive) skips discovery entirely.
const EnvHooks = "PHI_HOOKS"

// Warning is a non-fatal discovery problem (bad hook.json, unreadable dir entry).
type Warning struct {
	Path    string
	Message string
}

func (w Warning) String() string {
	if w.Path == "" {
		return w.Message
	}
	return w.Path + ": " + w.Message
}

// Discovered is a validated, enabled manifest with an absolute run path.
type Discovered struct {
	Manifest Manifest
	RunPath  string // absolute path to the executable
	Source   string // SourceUser or SourceProject
}

// ProjectHooksDir returns <cwd>/.phi/hooks (first-cut: cwd only, no walk-up).
func ProjectHooksDir(cwd string) string {
	if cwd == "" {
		return ""
	}
	return filepath.Join(cwd, ".phi", "hooks")
}

// HooksDisabled reports whether PHI_HOOKS=off.
func HooksDisabled() bool {
	v := strings.TrimSpace(os.Getenv(EnvHooks))
	return strings.EqualFold(v, "off")
}

// Discover scans userDir then projectDir for hook directories.
// Same Name: project replaces user (whole-entry shadow).
// Missing directories are fine. Parse errors become Warnings; only unexpected
// I/O on a present directory returns err.
//
// When PHI_HOOKS=off, returns empty slices without reading disk.
func Discover(userDir, projectDir string) ([]Discovered, []Warning, error) {
	if HooksDisabled() {
		return nil, nil, nil
	}

	byName := make(map[string]Discovered)
	var warnings []Warning

	load := func(dir, source string) error {
		if dir == "" {
			return nil
		}
		found, warns, err := scanHooksDir(dir, source)
		warnings = append(warnings, warns...)
		if err != nil {
			return err
		}
		for _, d := range found {
			byName[d.Manifest.Name] = d
		}
		return nil
	}

	if err := load(userDir, SourceUser); err != nil {
		return nil, warnings, err
	}
	if err := load(projectDir, SourceProject); err != nil {
		return nil, warnings, err
	}

	out := make([]Discovered, 0, len(byName))
	for _, d := range byName {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Manifest.Name != out[j].Manifest.Name {
			return out[i].Manifest.Name < out[j].Manifest.Name
		}
		return out[i].Source < out[j].Source
	})
	return out, warnings, nil
}

// DiscoverForCwd discovers from a user hooks dir and <cwd>/.phi/hooks.
func DiscoverForCwd(userDir, cwd string) ([]Discovered, []Warning, error) {
	return Discover(userDir, ProjectHooksDir(cwd))
}

func scanHooksDir(dir, source string) ([]Discovered, []Warning, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("hooks: read dir %s: %w", dir, err)
	}

	var (
		out      []Discovered
		warnings []Warning
	)
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		hookDir := filepath.Join(dir, ent.Name())
		manifestPath := filepath.Join(hookDir, ManifestFileName)
		if _, err := os.Stat(manifestPath); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			warnings = append(warnings, Warning{Path: manifestPath, Message: err.Error()})
			continue
		}

		m, err := ParseManifest(manifestPath)
		if err != nil {
			warnings = append(warnings, Warning{Path: manifestPath, Message: err.Error()})
			continue
		}
		if m.Disabled {
			continue
		}

		runPath, err := resolveRunPath(m.Dir, m.Run)
		if err != nil {
			warnings = append(warnings, Warning{Path: manifestPath, Message: err.Error()})
			continue
		}

		out = append(out, Discovered{
			Manifest: m,
			RunPath:  runPath,
			Source:   source,
		})
	}
	return out, warnings, nil
}

func resolveRunPath(hookDir, run string) (string, error) {
	run = strings.TrimSpace(run)
	if run == "" {
		return "", errors.New("empty run path")
	}
	if filepath.IsAbs(run) {
		return filepath.Clean(run), nil
	}
	abs, err := filepath.Abs(filepath.Join(hookDir, run))
	if err != nil {
		return "", fmt.Errorf("resolve run %q: %w", run, err)
	}
	return abs, nil
}

// FormatDiscovered returns a one-line status for palette / logs.
func FormatDiscovered(d Discovered) string {
	m := d.Manifest
	extra := ""
	if m.FailClosed {
		extra += " fail_closed"
	}
	if m.Async {
		extra += " async"
	}
	return fmt.Sprintf("%s  %s  match=%s  [%s]%s", m.Name, m.Kind, m.Match, d.Source, extra)
}
