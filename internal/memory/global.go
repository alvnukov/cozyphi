package memory

import (
	"bytes"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/alvnukov/cozyphi/internal/atomicfile"
	"github.com/alvnukov/cozyphi/internal/debuglog"
)

const (
	// globalScope is the one value of the `scope` key that means anything.
	// Any other value, and its absence, means the fact is about this
	// repository.
	globalScope = "global"
	// scopeKey is the frontmatter key publishing writes and un-publishing
	// removes.
	scopeKey = "scope"
	// corpusDir is the directory inside one of Claude Code's project
	// directories that holds its memory — the one shape the registry looks for.
	corpusDir = "memory"
	// clashNames bounds how many name clashes the maintenance block spells out
	// before it counts the rest: a misconfigured tree must not grow the prompt.
	clashNames = 3
)

// Registry is where a global fact lives outside this corpus.
//
// It is a parameter of the store rather than a constant so a test can point
// the fan-out at a temporary tree, and so a store opened without one — a
// sub-agent's — copies nothing anywhere.
type Registry struct {
	// Canonical is the store that keeps a copy of every global fact whatever
	// happens to the repositories (~/.cozyphi/memory). No session reads it:
	// it is the copy that outlives a deleted checkout and seeds a corpus that
	// has never seen the fact.
	Canonical string
	// Corpora is Claude Code's projects root. Every `memory/` one level below
	// it is a corpus the harness knows, whether or not its repository still
	// exists — so a re-clone finds its memory waiting.
	Corpora string
}

func (r Registry) off() bool { return r.Canonical == "" && r.Corpora == "" }

// Clash is a corpus that would not take a global fact: it already holds a file
// of that name that is not global. The local file wins — publishing must never
// overwrite something somebody wrote — and the clash is named in the prompt,
// because a rule silently not applying in one repository is the thing this
// whole feature exists to prevent.
type Clash struct {
	Name string // the global fact
	Dir  string // the corpus that kept its own
}

// Fanout is what one reconciliation did.
type Fanout struct {
	Facts   []string // names of the global facts copied somewhere
	Dirs    int      // directories written
	Clashes []Clash
}

// quiet reports a pass with nothing to say.
func (f Fanout) quiet() bool { return len(f.Facts) == 0 && len(f.Clashes) == 0 }

func (f Fanout) clone() Fanout {
	return Fanout{Facts: slices.Clone(f.Facts), Dirs: f.Dirs, Clashes: slices.Clone(f.Clashes)}
}

// scanned is what a pass saw of one file, kept so the next pass can skip a
// file whose modification time and size have not moved. The bytes are not
// kept: only a winner is ever read again, and only when a copy is due.
type scanned struct {
	mod    time.Time
	size   int64
	global bool
}

// LastFanout reports what the last reconciliation copied and what it could not.
func (s *Store) LastFanout() Fanout {
	if s == nil {
		return Fanout{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fanout.clone()
}

// Reconcile copies every global fact into every corpus that does not already
// hold it, the canonical store included, in whichever direction is needed: the
// newest copy of a name wins and is written over the older ones. That is what
// makes a global fact editable from any repository and from either harness —
// whoever wrote last is the source.
//
// A copy carries the winner's modification time, so the next pass sees a
// settled set rather than a fresh edit; without that the copies would leapfrog
// each other forever. A pass over an unchanged tree parses nothing and writes
// nothing.
//
// It never deletes and never moves. Removing a copy is always an explicit
// operation, so a corpus that vanished cannot cascade into losing the fact
// everywhere.
func (s *Store) Reconcile() Fanout {
	if s == nil || s.registry.off() {
		return Fanout{}
	}
	dirs := s.corpora()

	s.mu.Lock()
	before := s.scanned
	s.mu.Unlock()

	after := make(map[string]scanned, len(before)+len(dirs))
	listings := make([]map[string]scanned, len(dirs))
	for i, dir := range dirs {
		listings[i] = survey(dir, before, after)
	}

	report := s.copyWinners(dirs, listings, after)
	s.mu.Lock()
	s.scanned, s.fanout = after, report
	s.mu.Unlock()
	return report.clone()
}

// source is where one name's newest global copy lives.
type source struct {
	dir string
	mod time.Time
}

// copyWinners is the pass itself: pick the newest copy of each global name,
// write it wherever it is missing or older, and rewrite the catalog of every
// directory that changed.
func (s *Store) copyWinners(dirs []string, listings []map[string]scanned, after map[string]scanned) Fanout {
	winners := make(map[string]source)
	for i, files := range listings {
		for file, seen := range files {
			// A tie goes to the directory listed first, which is the store's
			// own: the pass has to be the same twice for the copies to settle.
			if best, found := winners[file]; !seen.global || (found && !seen.mod.After(best.mod)) {
				continue
			}
			winners[file] = source{dir: dirs[i], mod: seen.mod}
		}
	}

	var report Fanout
	written := make(map[string]bool, len(dirs))
	for _, file := range slices.Sorted(maps.Keys(winners)) {
		win := winners[file]
		var data []byte // read once per fact, and only if a copy is due
		copied := false
		for i, dir := range dirs {
			if dir == win.dir {
				continue
			}
			held, found := listings[i][file]
			if found && !held.global {
				report.Clashes = append(report.Clashes, Clash{Name: factName(file), Dir: dir})
				continue
			}
			if found && !held.mod.Before(win.mod) {
				continue
			}
			if data == nil {
				var err error
				if data, err = os.ReadFile(filepath.Join(win.dir, file)); err != nil {
					debuglog.Logf("memory: read %s: %v", filepath.Join(win.dir, file), err)
					break
				}
			}
			target := filepath.Join(dir, file)
			done, err := place(target, data, win.mod, held, found)
			if err != nil {
				debuglog.Logf("memory: copy %s: %v", target, err)
				continue
			}
			if info, err := os.Stat(target); err == nil {
				after[target] = scanned{mod: info.ModTime(), size: info.Size(), global: true}
			}
			if done {
				written[dir], copied = true, true
			}
		}
		if copied {
			report.Facts = append(report.Facts, factName(file))
		}
	}

	for _, dir := range dirs {
		if !written[dir] {
			continue
		}
		report.Dirs++
		if dir == s.dir {
			// This store's own catalog is rendered from its index, which has
			// to re-read the directory first; SyncIndex writes it.
			s.Invalidate()
			continue
		}
		if err := syncIndexIn(dir); err != nil {
			debuglog.Logf("memory: index %s: %v", dir, err)
		}
	}
	return report
}

// survey lists one directory's memory files, parsing only what has moved since
// the previous pass. The result is keyed by file name, which is a fact's
// identity across corpora; after collects the same by path, for the next pass.
func survey(dir string, before, after map[string]scanned) map[string]scanned {
	items, err := os.ReadDir(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			debuglog.Logf("memory: read %s: %v", dir, err)
		}
		return nil
	}
	files := make(map[string]scanned, len(items))
	for _, item := range items {
		file := item.Name()
		if item.IsDir() || filepath.Ext(file) != fileExt || file == IndexFile {
			continue
		}
		info, err := item.Info()
		if err != nil {
			continue
		}
		path := filepath.Join(dir, file)
		seen, known := before[path]
		if !known || !seen.mod.Equal(info.ModTime()) || seen.size != info.Size() {
			entry, err := ParseFile(path)
			if err != nil {
				// A file that will not parse is not a global fact, and it is
				// counted as one of this directory's own: nothing is written
				// over it.
				debuglog.Logf("memory: skip %s: %v", path, err)
			}
			seen = scanned{global: err == nil && entry.Global}
		}
		seen.mod, seen.size = info.ModTime(), info.Size()
		after[path] = seen
		files[file] = seen
	}
	return files
}

// place writes one copy and stamps it with the source's modification time.
// When the target already holds those bytes only the stamp is fixed, and the
// copy does not count as a write: a clock too coarse to keep the stamp must
// not turn every pass into a rewrite of the same content.
//
// A stamp that will not stick is logged and no more than that. The copy is
// then the newest of its name and wins the next pass, which copies the same
// bytes back over the same bytes and settles there.
func place(path string, data []byte, mod time.Time, held scanned, found bool) (bool, error) {
	stamp := func() {
		if err := os.Chtimes(path, mod, mod); err != nil {
			debuglog.Logf("memory: stamp %s: %v", path, err)
		}
	}
	if found && held.size == int64(len(data)) {
		if current, err := os.ReadFile(path); err == nil && bytes.Equal(current, data) {
			stamp()
			return false, nil
		}
	}
	// Owner-only and atomic, like the catalog: memory is the user's data, and
	// Claude Code may be reading the directory at this moment.
	if err := atomicfile.Write(path, 0o600, data); err != nil {
		return false, err
	}
	stamp()
	return true, nil
}

// corpora lists every directory a global fact belongs in: this store's own,
// the canonical store, and every corpus under Claude Code's projects root.
// The store's own directory is normally one of those already; naming it first
// keeps a corpus kept somewhere else in the pass, and settles ties in its
// favor.
func (s *Store) corpora() []string {
	dirs := make([]string, 0, 16)
	add := func(dir string) {
		if dir != "" && !slices.Contains(dirs, dir) {
			dirs = append(dirs, dir)
		}
	}
	add(s.dir)
	add(s.registry.Canonical)
	if s.registry.Corpora == "" {
		return dirs
	}
	items, err := os.ReadDir(s.registry.Corpora)
	if err != nil {
		if !os.IsNotExist(err) {
			debuglog.Logf("memory: read %s: %v", s.registry.Corpora, err)
		}
		return dirs
	}
	for _, item := range items {
		if !item.IsDir() {
			continue
		}
		dir := filepath.Join(s.registry.Corpora, item.Name(), corpusDir)
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			add(dir)
		}
	}
	return dirs
}

// MarkGlobal publishes one fact: `scope: global` goes into the frontmatter of
// the file that is already here, and the reconciliation that follows puts a
// copy of it in every corpus the harness knows. The file does not move — the
// repository it was written in keeps it, and Claude Code there keeps reading
// it.
//
// A corpus that holds a local file of the same name keeps its own; the publish
// succeeds everywhere else and reports the corpus that refused, because
// refusing outright would make a common name unpublishable forever.
func (s *Store) MarkGlobal(name string) (Entry, Fanout, error) {
	if s == nil {
		return Entry{}, Fanout{}, fmt.Errorf("memory: no memory directory to publish %q from", name)
	}
	entry, ok := s.Fact(name)
	if !ok {
		return Entry{}, Fanout{}, fmt.Errorf("memory: no memory named %q", name)
	}
	if !entry.Global {
		if err := rewriteFrontmatter(entry.Path, withScope); err != nil {
			return Entry{}, Fanout{}, fmt.Errorf("memory: publish %s: %w", entry.File, err)
		}
		entry.Global = true
		s.Invalidate()
	}
	return entry, s.Reconcile(), nil
}

// MarkLocal takes one fact back to this repository: the `scope` key leaves the
// file here, and every copy elsewhere — the canonical store included — moves
// into that directory's forgotten/. The fact itself stays where it is, so
// "this is about one repository after all" never means "this is deleted".
//
// Stripping and sweeping happen in one call on purpose. Reconciliation never
// deletes, so a copy left behind would be the newest of its name on the next
// pass and would republish the fact.
func (s *Store) MarkLocal(name string) (Entry, error) {
	if s == nil {
		return Entry{}, fmt.Errorf("memory: no memory directory to un-publish %q from", name)
	}
	entry, ok := s.Fact(name)
	if !ok {
		return Entry{}, fmt.Errorf("memory: no memory named %q", name)
	}
	if !entry.Global {
		return Entry{}, fmt.Errorf("memory: %s is not global here; nothing to un-publish", entry.Name)
	}
	if err := rewriteFrontmatter(entry.Path, withoutScope); err != nil {
		return Entry{}, fmt.Errorf("memory: un-publish %s: %w", entry.File, err)
	}
	entry.Global = false
	s.Invalidate()
	s.archiveCopies(entry.File)
	return entry, nil
}

// archiveCopies moves every other corpus's copy of one global fact into that
// corpus's forgotten/ and rewrites the catalogs it changed. A file of that
// name that is not global belongs to that repository and is left alone.
//
// Nothing is deleted here either: the copies are moved aside, so a wrong call
// costs a move back rather than a retype.
func (s *Store) archiveCopies(file string) {
	if s == nil || s.registry.off() {
		return
	}
	cleared := 0
	for _, dir := range s.corpora() {
		if dir == s.dir {
			continue
		}
		path := filepath.Join(dir, file)
		entry, err := ParseFile(path)
		if err != nil || !entry.Global {
			continue
		}
		if err := archive(dir, file, path); err != nil {
			debuglog.Logf("memory: archive %s: %v", path, err)
			continue
		}
		if err := syncIndexIn(dir); err != nil {
			debuglog.Logf("memory: index %s: %v", dir, err)
		}
		cleared++
	}
	if cleared > 0 {
		// The survey cache describes files that are no longer there.
		s.mu.Lock()
		s.scanned = nil
		s.mu.Unlock()
	}
}

// syncIndexIn rewrites one directory's catalog from the files in it. It is the
// renderer the store uses for its own MEMORY.md, applied to a directory no
// session holds a store for — the copies have to be cataloged there too, or
// the catalog a Claude Code session reads is missing rows the directory has.
func syncIndexIn(dir string) error {
	items, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	entries := make([]Entry, 0, len(items))
	for _, item := range items {
		if item.IsDir() || filepath.Ext(item.Name()) != fileExt || item.Name() == IndexFile {
			continue
		}
		entry, err := ParseFile(filepath.Join(dir, item.Name()))
		if err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	slices.SortFunc(entries, byKindThenName)
	want := renderIndex(entries)
	path := filepath.Join(dir, IndexFile)
	if current, err := os.ReadFile(path); err == nil && string(current) == want {
		return nil
	}
	return atomicfile.Write(path, 0o600, []byte(want))
}

// factName is how a file name reads in a message: the way the index names it.
func factName(file string) string { return strings.TrimSuffix(file, fileExt) }

// rewriteFrontmatter applies edit to a memory file's frontmatter and writes it
// back in place, keeping the file's permissions: the file may be one Claude
// Code wrote, and marking it must not quietly change what it is.
func rewriteFrontmatter(path string, edit func(string) (string, error)) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	next, err := edit(string(raw))
	if err != nil {
		return err
	}
	if next == string(raw) {
		return nil
	}
	mode := fs.FileMode(0o600)
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	}
	return atomicfile.Write(path, mode, []byte(next))
}

// withScope adds `scope: global` to a memory file's frontmatter and leaves
// every other line where it was. The key lands after the last flat scalar,
// which is where this format keeps `name` and `description` and before any
// nested block — so a folded description keeps its own continuation lines.
func withScope(raw string) (string, error) {
	lines, eol, open, shut, err := frontmatter(raw)
	if err != nil {
		return "", err
	}
	at := open + 1
	for i := open + 1; i < shut; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || lines[i] != trimmed {
			continue
		}
		key, value, ok := strings.Cut(trimmed, ":")
		value = strings.TrimSpace(value)
		if !ok || value == "" || strings.HasPrefix(value, ">") || strings.HasPrefix(value, "|") {
			// A key with no value opens a nested block, and one whose value is
			// `>` or `|` opens a folded one. Either way the lines under it
			// belong to that key, and the flat keys are above it.
			continue
		}
		if strings.EqualFold(strings.TrimSpace(key), scopeKey) {
			return raw, nil
		}
		at = i + 1
	}
	return strings.Join(slices.Insert(lines, at, scopeKey+": "+globalScope), eol), nil
}

// withoutScope removes the `scope` key from a memory file's frontmatter,
// wherever it was written: flat, or nested under metadata like `type` and
// `pin` may be.
func withoutScope(raw string) (string, error) {
	lines, eol, open, shut, err := frontmatter(raw)
	if err != nil {
		return "", err
	}
	kept := make([]string, 0, len(lines))
	kept = append(kept, lines[:open+1]...)
	for i := open + 1; i < shut; i++ {
		key, _, ok := strings.Cut(strings.TrimSpace(lines[i]), ":")
		if ok && strings.EqualFold(strings.TrimSpace(key), scopeKey) {
			continue
		}
		kept = append(kept, lines[i])
	}
	return strings.Join(append(kept, lines[shut:]...), eol), nil
}

// frontmatter locates a memory file's frontmatter block: the lines, the line
// ending to write back with, and the indices of the opening and closing
// delimiters.
func frontmatter(raw string) (lines []string, eol string, open, shut int, err error) {
	eol = "\n"
	if strings.Contains(raw, "\r\n") {
		eol = "\r\n"
	}
	lines = strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	open = -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.TrimSpace(strings.TrimPrefix(line, bom)) != frontmatterDelim {
			return nil, "", 0, 0, ErrNoFrontmatter
		}
		open = i
		break
	}
	if open < 0 {
		return nil, "", 0, 0, ErrNoFrontmatter
	}
	for i := open + 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == frontmatterDelim {
			return lines, eol, open, i, nil
		}
	}
	return nil, "", 0, 0, ErrOpenFrontmatter
}
