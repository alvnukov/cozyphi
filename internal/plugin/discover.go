package plugin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/alvnukov/cozyphi/internal/debuglog"
)

var (
	// validName keeps a plugin name usable as a skill namespace and as a
	// directory name: no ":" (the namespace separator), no path separators,
	// no leading dot.
	validName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	// unsafeDataChars mirrors Claude Code's data directory naming, so both
	// harnesses share one directory per plugin.
	unsafeDataChars = regexp.MustCompile(`[^A-Za-z0-9_-]`)
	// unsupported are plugin components cozyphi does not load yet.
	unsupported = []string{"commands/", "agents/", "output-styles/", "monitors/", ".mcp.json", ".lsp.json"}
)

type candidate struct {
	id      string // "" for a local plugin: the manifest names it
	root    string
	dataDir string // "" for a local plugin: derived from the name
}

// Discover returns the enabled plugins: Claude-installed ones sorted by ID,
// then plugins.paths in config order. It never fails: every problem is a
// Warning, and a missing directory means no plugins.
func Discover(cfg Config, projectRoot string) ([]Plugin, []Warning) {
	if !cfg.Enabled || Disabled() {
		return nil, nil
	}
	claude, warns := claudeCandidates(cfg.ClaudeDir, projectRoot)
	local, more := localCandidates(cfg.Paths)
	warns = append(warns, more...)

	var out []Plugin
	owner := make(map[string]string) // Name → ID of the plugin that took it
	for _, c := range append(claude, local...) {
		p, more, ok := build(c, cfg.LocalDataDir)
		warns = append(warns, more...)
		if !ok {
			continue
		}
		if prev, taken := owner[p.Name]; taken {
			warns = append(warns, Warning{Plugin: p.ID, Msg: fmt.Sprintf(
				"plugin name %q is already used by %s; skipped — disable one of them", p.Name, prev)})
			continue
		}
		owner[p.Name] = p.ID
		debuglog.Logf("plugin: loaded %s from %s (%d skill dirs, %d hook files)",
			p.ID, p.Root, len(p.SkillDirs), len(p.HookFiles))
		out = append(out, p)
	}
	for _, w := range warns {
		debuglog.Logf("plugin: %s", w)
	}
	return out, warns
}

type installedFile struct {
	Version int                       `json:"version"`
	Plugins map[string][]installEntry `json:"plugins"`
}

type installEntry struct {
	Scope       string `json:"scope"`
	InstallPath string `json:"installPath"`
	ProjectPath string `json:"projectPath"`
}

func claudeCandidates(claudeDir, projectRoot string) ([]candidate, []Warning) {
	if claudeDir == "" {
		return nil, nil
	}
	file := filepath.Join(claudeDir, "plugins", "installed_plugins.json")
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, []Warning{{Plugin: file, Msg: fmt.Sprintf(
			"read: %v — check its permissions or set plugins.claude_dir", err)}}
	}
	var inst installedFile
	if err := json.Unmarshal(data, &inst); err != nil {
		return nil, []Warning{{Plugin: file, Msg: fmt.Sprintf(
			"invalid JSON: %v — reinstall plugins from Claude Code or fix the file", err)}}
	}
	if inst.Version != 2 {
		return nil, []Warning{{Plugin: file, Msg: fmt.Sprintf(
			"version %d is not supported; Claude-installed plugins skipped — cozyphi reads version 2", inst.Version)}}
	}
	enabled, warns := enabledPlugins(claudeDir, projectRoot)
	var out []candidate
	for _, id := range slices.Sorted(maps.Keys(inst.Plugins)) {
		if !enabled[id] {
			continue
		}
		entry, ok := pickInstall(inst.Plugins[id], projectRoot)
		if !ok {
			continue
		}
		if !filepath.IsAbs(entry.InstallPath) {
			warns = append(warns, Warning{Plugin: id, Msg: fmt.Sprintf(
				"installPath %q is not absolute; skipped — reinstall the plugin in Claude Code", entry.InstallPath)})
			continue
		}
		out = append(out, candidate{
			id:      id,
			root:    filepath.Clean(entry.InstallPath),
			dataDir: filepath.Join(claudeDir, "plugins", "data", unsafeDataChars.ReplaceAllString(id, "-")),
		})
	}
	return out, warns
}

// pickInstall prefers the entry installed for this project, then the first
// entry without a projectPath; another project's entry never counts.
func pickInstall(entries []installEntry, projectRoot string) (installEntry, bool) {
	var (
		user  installEntry
		found bool
	)
	for _, e := range entries {
		if e.ProjectPath == "" {
			if !found {
				user, found = e, true
			}
			continue
		}
		if sameDir(e.ProjectPath, projectRoot) {
			return e, true
		}
	}
	return user, found
}

// sameDir compares directories by identity, so a symlinked or differently
// spelled projectPath still names this project.
func sameDir(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	sa, err := os.Stat(a)
	if err != nil {
		return false
	}
	sb, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(sa, sb)
}

// enabledPlugins merges enabledPlugins from the user, project and local
// settings, later files winning per key. Only a literal true enables.
func enabledPlugins(claudeDir, projectRoot string) (map[string]bool, []Warning) {
	files := []string{filepath.Join(claudeDir, "settings.json")}
	if projectRoot != "" {
		files = append(files,
			filepath.Join(projectRoot, ".claude", "settings.json"),
			filepath.Join(projectRoot, ".claude", "settings.local.json"))
	}
	merged := make(map[string]json.RawMessage)
	var warns []Warning
	for _, file := range files {
		data, err := os.ReadFile(file)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			warns = append(warns, Warning{Plugin: file, Msg: fmt.Sprintf("read: %v — check its permissions", err)})
			continue
		}
		var settings struct {
			EnabledPlugins map[string]json.RawMessage `json:"enabledPlugins"`
		}
		if err := json.Unmarshal(data, &settings); err != nil {
			warns = append(warns, Warning{Plugin: file, Msg: fmt.Sprintf(
				"invalid JSON: %v — its enabledPlugins are ignored until the file parses", err)})
			continue
		}
		maps.Copy(merged, settings.EnabledPlugins)
	}
	out := make(map[string]bool, len(merged))
	for id, raw := range merged {
		out[id] = bytes.Equal(bytes.TrimSpace(raw), []byte("true"))
	}
	return out, warns
}

func localCandidates(paths []string) ([]candidate, []Warning) {
	var (
		out   []candidate
		warns []Warning
	)
	for _, p := range paths {
		if !filepath.IsAbs(p) {
			warns = append(warns, Warning{Plugin: p, Msg: "relative plugin path skipped — use an absolute path or ~/…"})
			continue
		}
		st, err := os.Stat(p)
		switch {
		case err != nil:
			warns = append(warns, Warning{Plugin: p, Msg: fmt.Sprintf(
				"plugin path unavailable: %v — fix plugins.paths in config.yaml", err)})
		case !st.IsDir():
			warns = append(
				warns,
				Warning{Plugin: p, Msg: "plugin path is not a directory — point plugins.paths at the plugin root"},
			)
		default:
			out = append(out, candidate{root: filepath.Clean(p)})
		}
	}
	return out, warns
}

type manifest struct {
	Name   string          `json:"name"`
	Skills json.RawMessage `json:"skills"`
	Hooks  json.RawMessage `json:"hooks"`
}

func build(c candidate, localDataDir string) (Plugin, []Warning, bool) {
	label := c.id
	if label == "" {
		label = c.root
	}
	var warns []Warning
	warn := func(format string, args ...any) {
		warns = append(warns, Warning{Plugin: label, Msg: fmt.Sprintf(format, args...)})
	}
	if !isDir(c.root) {
		warn("plugin root %s is missing — reinstall the plugin in Claude Code or fix the path", c.root)
		return Plugin{}, warns, false
	}
	m, err := readManifest(c.root)
	if err != nil {
		warn("%v — fix .claude-plugin/plugin.json", err)
		return Plugin{}, warns, false
	}

	name := filepath.Base(c.root)
	switch {
	case c.id != "":
		name, _, _ = strings.Cut(c.id, "@")
	case m.Name != "":
		name = m.Name
	}
	if !validName.MatchString(name) {
		warn("plugin name %q cannot be a namespace; skipped — use letters, digits, '.', '_' or '-'", name)
		return Plugin{}, warns, false
	}
	p := Plugin{ID: c.id, Name: name, Root: c.root, DataDir: c.dataDir}
	if p.ID == "" {
		p.ID = name
		p.DataDir = filepath.Join(localDataDir, name)
	}

	if dir := filepath.Join(c.root, "skills"); isDir(dir) {
		p.SkillDirs = append(p.SkillDirs, dir)
	}
	if isFile(filepath.Join(c.root, "SKILL.md")) {
		p.SkillDirs = append(p.SkillDirs, c.root)
	}
	if file := filepath.Join(c.root, "hooks", "hooks.json"); isFile(file) {
		p.HookFiles = append(p.HookFiles, file)
	}
	p.SkillDirs = addComponents(p.SkillDirs, c.root, "skills", m.Skills, warn)
	p.HookFiles = addComponents(p.HookFiles, c.root, "hooks", m.Hooks, warn)

	for _, comp := range unsupported {
		if exists(filepath.Join(c.root, strings.TrimSuffix(comp, "/"))) {
			warn("%s is not supported by cozyphi; skipped — use it from Claude Code", comp)
		}
	}
	return p, warns, true
}

func readManifest(root string) (manifest, error) {
	file := filepath.Join(root, ".claude-plugin", "plugin.json")
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return manifest{}, nil
	}
	if err != nil {
		return manifest{}, fmt.Errorf("read %s: %w", file, err)
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return manifest{}, fmt.Errorf("invalid %s: %w", file, err)
	}
	return m, nil
}

// addComponents appends a manifest path or path list, confined to root.
func addComponents(dst []string, root, field string, raw json.RawMessage, warn func(string, ...any)) []string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return dst
	}
	var paths []string
	switch raw[0] {
	case '{':
		warn("inline %s in plugin.json are not supported; skipped — move them to %s/", field, field)
		return dst
	case '"':
		var one string
		if err := json.Unmarshal(raw, &one); err != nil {
			warn("manifest %s: %v — use a path or an array of paths", field, err)
			return dst
		}
		paths = []string{one}
	default:
		if err := json.Unmarshal(raw, &paths); err != nil {
			warn("manifest %s: %v — use a path or an array of paths", field, err)
			return dst
		}
	}
	for _, rel := range paths {
		abs, ok := confine(root, rel)
		if !ok {
			warn("manifest %s path %q leaves the plugin root; skipped — keep the path inside the plugin", field, rel)
			continue
		}
		if !slices.Contains(dst, abs) {
			dst = append(dst, abs)
		}
	}
	return dst
}

func confine(root, rel string) (string, bool) {
	if filepath.IsAbs(rel) {
		return "", false
	}
	abs := filepath.Join(root, rel)
	r, err := filepath.Rel(root, abs)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return "", false
	}
	return abs, true
}

func isDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

func isFile(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
