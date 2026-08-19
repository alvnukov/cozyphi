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

// Warning is a non-fatal discovery problem (bad plugin.json, unreadable dir).
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

// Discovered is a validated, enabled hook with an absolute run path.
type Discovered struct {
	Manifest Manifest
	RunPath  string // absolute path to the executable
	Source   string // SourceUser or SourceProject
}

// HooksDisabled reports whether PHI_HOOKS=off.
func HooksDisabled() bool {
	v := strings.TrimSpace(os.Getenv(EnvHooks))
	return strings.EqualFold(v, "off")
}

// Discover loads plugin.json from userDir then projectDir.
// Same hook Name: project replaces user (whole-entry shadow).
// Missing directories are fine. Parse errors become Warnings; only unexpected
// I/O on a present directory returns err.
//
// Layout: <hooksDir>/plugin.json and <hooksDir>/<plugin>/plugin.json.
// Relative run paths resolve against the directory that contains plugin.json.
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
		seen     = make(map[string]string) // hook name → plugin path
	)

	loadFile := func(pluginPath string) {
		found, warns := loadPluginFile(pluginPath, source, seen)
		warnings = append(warnings, warns...)
		out = append(out, found...)
	}

	rootPlugin := filepath.Join(dir, PluginFileName)
	if st, err := os.Stat(rootPlugin); err == nil && !st.IsDir() {
		loadFile(rootPlugin)
	} else if err != nil && !os.IsNotExist(err) {
		warnings = append(warnings, Warning{Path: rootPlugin, Message: err.Error()})
	}

	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		pluginPath := filepath.Join(dir, ent.Name(), PluginFileName)
		st, err := os.Stat(pluginPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			warnings = append(warnings, Warning{Path: pluginPath, Message: err.Error()})
			continue
		}
		if st.IsDir() {
			continue
		}
		loadFile(pluginPath)
	}
	return out, warnings, nil
}

func loadPluginFile(pluginPath, source string, seen map[string]string) ([]Discovered, []Warning) {
	manifests, err := ParsePlugin(pluginPath)
	if err != nil {
		return nil, []Warning{{Path: pluginPath, Message: err.Error()}}
	}

	var (
		out      []Discovered
		warnings []Warning
	)
	for _, m := range manifests {
		if m.Disabled {
			continue
		}
		if prev, dup := seen[m.Name]; dup {
			warnings = append(warnings, Warning{
				Path:    pluginPath,
				Message: fmt.Sprintf("duplicate hook name %q (already defined in %s); skipped", m.Name, prev),
			})
			continue
		}
		runPath, err := resolveRunPath(m.Dir, m.Run)
		if err != nil {
			warnings = append(warnings, Warning{Path: pluginPath, Message: m.Name + ": " + err.Error()})
			continue
		}
		seen[m.Name] = pluginPath
		out = append(out, Discovered{
			Manifest: m,
			RunPath:  runPath,
			Source:   source,
		})
	}
	return out, warnings
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
	if m.Kind == KindCommand {
		return fmt.Sprintf("%s  %s  [%s]%s", m.Name, m.Kind, d.Source, extra)
	}
	return fmt.Sprintf("%s  %s  match=%s  [%s]%s", m.Name, m.Kind, m.Match, d.Source, extra)
}
