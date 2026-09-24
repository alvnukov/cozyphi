package skills

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Source is one directory the skill catalog is read from. The user's
// skill_path has no Namespace; a Claude Code plugin's skills directory
// carries the plugin name, so its skills read "<namespace>:<name>", and the
// placeholder values its bodies may reference.
type Source struct {
	Dir       string
	Namespace string
	// Vars maps a placeholder name (CLAUDE_PLUGIN_ROOT) to its value; every
	// "${NAME}" in a loaded body is replaced. The file on disk stays raw.
	Vars map[string]string
}

// Sources is the ordered catalog: skill_path first, then plugin directories.
// It is re-read on every Load; there is no cache to invalidate.
type Sources []Source

// Load reads every source in order. A file reached twice (a symlink, or two
// sources sharing a tree) belongs to the first source that reached it. A
// source that fails contributes its error and nothing else: the returned
// list holds every skill the other sources produced.
func (ss Sources) Load() ([]*Skill, error) {
	var (
		out  []*Skill
		errs []error
		seen = make(map[string]bool)
	)
	for _, src := range ss {
		found, err := src.load()
		if err != nil {
			errs = append(errs, err)
		}
		for _, skill := range found {
			key := skill.SkillFilePath
			if real, err := filepath.EvalSymlinks(key); err == nil {
				key = real
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, skill)
		}
	}
	return out, errors.Join(errs...)
}

// String lists the configured directories for error messages.
func (ss Sources) String() string {
	dirs := make([]string, 0, len(ss))
	for _, src := range ss {
		if src.Dir != "" {
			dirs = append(dirs, src.Dir)
		}
	}
	if len(dirs) == 0 {
		return "(no skill directories)"
	}
	return strings.Join(dirs, ", ")
}

func (src Source) load() ([]*Skill, error) {
	if src.Dir == "" {
		return nil, nil
	}
	st, err := os.Stat(src.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("skills: %w", err)
	}
	if !st.IsDir() {
		return nil, fmt.Errorf(
			"skills: %s is not a directory — point skill_path or the plugin at a directory", src.Dir)
	}
	w := walker{visited: make(map[string]bool)}
	w.walk(src.Dir)
	for _, skill := range w.found {
		src.adopt(skill)
	}
	return w.found, errors.Join(w.errs...)
}

// adopt applies the source's namespace and placeholder values to a parsed skill.
func (src Source) adopt(skill *Skill) {
	if src.Namespace != "" {
		skill.Name = src.Namespace + ":" + cmp.Or(skill.Name, filepath.Base(skill.Path))
	}
	skill.Body = expandVars(skill.Body, src.Vars)
}

func expandVars(body string, vars map[string]string) string {
	if len(vars) == 0 {
		return body
	}
	pairs := make([]string, 0, 2*len(vars))
	for _, k := range slices.Sorted(maps.Keys(vars)) {
		pairs = append(pairs, "${"+k+"}", vars[k])
	}
	return strings.NewReplacer(pairs...).Replace(body)
}

// walker is filepath.WalkDir plus directory symlinks. It remembers every
// directory by resolved real path, so a link back up the tree ends there
// instead of looping forever.
type walker struct {
	visited map[string]bool
	found   []*Skill
	errs    []error
}

func (w *walker) walk(dir string) {
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		w.errs = append(w.errs, fmt.Errorf("skills: %w", err))
		return
	}
	if w.visited[real] {
		return
	}
	w.visited[real] = true
	entries, err := os.ReadDir(dir)
	if err != nil {
		w.errs = append(w.errs, fmt.Errorf("skills: %w", err))
		return
	}
	for _, ent := range entries { // ReadDir sorts by name, as WalkDir did
		path := filepath.Join(dir, ent.Name())
		switch {
		case ent.IsDir():
			w.walk(path)
		case ent.Name() == SkillFileName:
			// Checked before the symlink case: a SKILL.md linked in from a
			// dotfiles store loaded under WalkDir and still does.
			if skill, err := Parse(path); err == nil {
				w.found = append(w.found, skill) // invalid files are skipped, as before
			}
		case ent.Type()&fs.ModeSymlink != 0:
			// A dangling link, or one to a file, is not a skill directory.
			if st, err := os.Stat(path); err == nil && st.IsDir() {
				w.walk(path)
			}
		}
	}
}
