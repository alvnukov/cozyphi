package memory

import (
	"cmp"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/alvnukov/cozyphi/internal/debuglog"
)

const (
	// forgottenDir is where a forgotten memory goes. It is inside the memory
	// directory and it is not deleted: forgetting has to be reversible,
	// because the reason to forget is a judgement and judgements are wrong.
	forgottenDir = "forgotten"
	// usageWindow is the half-life of usefulness, matching the picker history
	// the rest of the harness ranks by.
	usageWindow = 30 * 24 * time.Hour
	// staleWindow is how long a memory may go unused before the harness
	// suggests looking at it. Long on purpose: a memory about a release freeze
	// is useless for a quarter and then decisive for a day.
	staleWindow = 90 * 24 * time.Hour
	// overlapThreshold is the term-set overlap two memories need before they
	// are worth merging.
	overlapThreshold = 0.6
	// overlapScanLimit is the directory size past which merge candidates are
	// not looked for on every prompt. Past it the maintenance block says to
	// ask for them instead.
	overlapScanLimit = 2000
	// overlapCandidates bounds the pairs one scan reports.
	overlapCandidates = 32
)

// Overlap is two memories that say much the same thing — a merge candidate.
// The harness finds them; only the model can merge them, because merging two
// facts without losing the nuance of either is not a mechanical operation.
type Overlap struct {
	A, B       Entry
	Similarity float64
}

// Forget moves one memory out of the directory and into forgotten/, where it
// leaves the index, the prompt and retrieval — but not the disk.
//
// A pinned memory is refused: unpinning is the deliberate step that says the
// fact really is finished.
func (s *Store) Forget(name string) (Entry, error) {
	if s == nil {
		return Entry{}, fmt.Errorf("memory: no memory directory to forget %q from", name)
	}
	entry, ok := s.Fact(name)
	if !ok {
		return Entry{}, fmt.Errorf("memory: no memory named %q", name)
	}
	if entry.Pinned {
		return Entry{}, fmt.Errorf("memory: %s is pinned; remove `pin: true` from the file first", entry.Name)
	}

	archive := filepath.Join(s.dir, forgottenDir)
	if err := os.MkdirAll(archive, 0o755); err != nil {
		return Entry{}, fmt.Errorf("memory: create %s: %w", archive, err)
	}
	target := filepath.Join(archive, entry.File)
	if _, err := os.Stat(target); err == nil {
		// Forgotten twice under the same name: keep both.
		stamp := time.Now().UTC().Format("20060102-150405")
		target = filepath.Join(archive, strings.TrimSuffix(entry.File, fileExt)+"-"+stamp+fileExt)
	}
	if err := os.Rename(entry.Path, target); err != nil {
		return Entry{}, fmt.Errorf("memory: forget %s: %w", entry.File, err)
	}
	s.Invalidate()
	return entry, nil
}

// Forgotten lists what is in the archive, newest first. Nothing reads it back
// into the prompt — it is there so a wrong call can be undone.
func (s *Store) Forgotten() []Entry {
	if s == nil {
		return nil
	}
	items, err := os.ReadDir(filepath.Join(s.dir, forgottenDir))
	if err != nil {
		return nil
	}
	entries := make([]Entry, 0, len(items))
	for _, item := range items {
		if item.IsDir() || filepath.Ext(item.Name()) != fileExt {
			continue
		}
		entry, err := ParseFile(filepath.Join(s.dir, forgottenDir, item.Name()))
		if err != nil {
			continue
		}
		if info, err := item.Info(); err == nil {
			entry.Modified = info.ModTime()
		}
		entries = append(entries, entry)
	}
	slices.SortFunc(entries, func(a, b Entry) int { return b.Modified.Compare(a.Modified) })
	return entries
}

// Stale lists memories nothing has used within staleWindow, least useful
// first. A pinned memory is never stale, and neither is one written recently:
// it has not had its chance yet.
//
// Nothing here is demoted or removed. It is what the maintenance block asks
// the model to look at, and the model decides.
func (s *Store) Stale() []Entry {
	if s == nil || s.use == nil {
		return nil
	}
	now := time.Now()
	var stale []Entry
	for _, entry := range s.Entries() {
		if entry.Pinned || now.Sub(entry.Modified) < staleWindow {
			continue
		}
		count, last := s.use.Seen(entry.Name)
		if count > 0 && now.Sub(last) < staleWindow {
			continue
		}
		stale = append(stale, entry)
	}
	slices.SortFunc(stale, func(a, b Entry) int {
		return cmp.Compare(s.priority(a, now), s.priority(b, now))
	})
	return stale
}

// Compact archives exact duplicates: one fact saved under two names, with the
// same description and the same body. The newer file stays.
//
// It is the only compaction the harness performs by itself, because it is the
// only one that cannot lose anything. A duplicate that is pinned, or that
// another memory links to, is left alone — the name is part of the fact then.
func (s *Store) Compact() []string {
	if s == nil {
		return nil
	}
	entries := s.Entries()
	if len(entries) < 2 {
		return nil
	}

	linked := make(map[string]bool)
	groups := make(map[string][]Entry, len(entries))
	for _, entry := range entries {
		for _, link := range entry.Links {
			linked[strings.ToLower(link)] = true
		}
		key := normalize(entry.Description) + "\x00" + normalize(entry.Body)
		groups[key] = append(groups[key], entry)
	}

	var archived []string
	for _, group := range groups {
		if len(group) < 2 {
			continue
		}
		slices.SortFunc(group, func(a, b Entry) int { return b.Modified.Compare(a.Modified) })
		for _, duplicate := range group[1:] {
			if duplicate.Pinned || linked[strings.ToLower(duplicate.Name)] {
				continue
			}
			if _, err := s.Forget(duplicate.Name); err != nil {
				debuglog.Logf("memory: compact %s: %v", duplicate.Name, err)
				continue
			}
			archived = append(archived, duplicate.Name)
		}
	}
	slices.Sort(archived)
	return archived
}

// Overlaps finds merge candidates: memories whose term sets overlap above the
// threshold. The pairs are computed from the inverted index — only memories
// that share a rare word are ever compared — and cached until the directory
// changes, because the prompt asks for them on every render.
func (s *Store) Overlaps(threshold float64, limit int) []Overlap {
	if s == nil || limit <= 0 {
		return nil
	}
	if threshold <= 0 {
		threshold = overlapThreshold
	}
	idx := s.index()
	if idx.live() > overlapScanLimit {
		return nil
	}

	s.mu.Lock()
	if s.overlapFor != idx.fingerprint || s.overlapping == nil {
		s.overlapFor = idx.fingerprint
		s.overlapping = idx.overlaps()
	}
	found := s.overlapping
	s.mu.Unlock()

	out := make([]Overlap, 0, min(limit, len(found)))
	for _, pair := range found {
		if pair.Similarity < threshold {
			break
		}
		if out = append(out, pair); len(out) == limit {
			break
		}
	}
	return out
}

// overlaps compares only the documents that share a discriminating term, which
// is what keeps this off the O(n²) path a directory would otherwise put it on.
func (x *index) overlaps() []Overlap {
	shared := make(map[[2]int32]int32, 256)
	common := x.commonCutoff()
	for _, list := range x.postings {
		if len(list) > common || len(list) < 2 {
			continue
		}
		for i, a := range list {
			for _, b := range list[i+1:] {
				pair := [2]int32{min(a.doc, b.doc), max(a.doc, b.doc)}
				shared[pair]++
			}
		}
	}

	found := make([]Overlap, 0, 16)
	for pair, count := range shared {
		a, b := x.docs[pair[0]], x.docs[pair[1]]
		if a.Path == "" || b.Path == "" {
			continue
		}
		sizeA, sizeB := len(x.terms[pair[0]]), len(x.terms[pair[1]])
		union := sizeA + sizeB - int(count)
		if union <= 0 {
			continue
		}
		similarity := float64(count) / float64(union)
		if similarity < overlapThreshold {
			continue
		}
		found = append(found, Overlap{A: a, B: b, Similarity: similarity})
	}
	slices.SortFunc(found, func(p, q Overlap) int {
		if c := cmp.Compare(q.Similarity, p.Similarity); c != 0 {
			return c
		}
		return byKindThenName(p.A, q.A)
	})
	if len(found) > overlapCandidates {
		found = found[:overlapCandidates]
	}
	return found
}

// normalize folds whitespace so two files that differ only in wrapping are
// recognized as the same fact.
func normalize(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(text), " "))
}

// round keeps a similarity readable where it is printed.
func round(value float64) float64 { return math.Round(value*100) / 100 }
