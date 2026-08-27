package memory

import (
	"hash/fnv"
	"maps"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/alvnukov/cozyphi/internal/debuglog"
)

const (
	// maxIndexedRunes caps how much of one body is indexed. A reminder shows
	// the first maxBodyRunes anyway, and a fact that needs more than this to be
	// recognizable is a document, not a memory.
	maxIndexedRunes = 4000
	// commonTermFraction skips a term carried by more than this share of a
	// directory: it cannot discriminate, and its posting list is the one thing
	// that would grow with the corpus.
	commonTermFraction = 0.5
	// commonTermFloor is the corpus size below which every term counts: with
	// five memories, a word in three of them is still the word that matters.
	commonTermFloor = 20
	// verifyInterval is how often the cheap staleness check is backed by a full
	// scan of names, sizes and mtimes. It is the window in which a memory file
	// edited outside cozyphi is still served from the cache.
	verifyInterval = 30 * time.Second
)

// posting is one term's presence in one memory, weighted by the best field it
// lands in: name 3, description 2, body 1.
type posting struct {
	doc    int32
	weight float32
}

// fileRef is one memory file as the directory listing describes it, before
// anything reads it. Comparable on purpose: it is the key an update reuses a
// parsed entry by, and three fields that all match mean the same bytes.
type fileRef struct {
	name string
	mod  int64 // modification time, UnixNano
	size int64
}

// index is the searchable form of a memory directory: an inverted index whose
// document ids are stable slots, so a directory that changes by one file is
// updated by one file.
//
// Every update returns a new index that shares what did not change — posting
// lists, parsed entries — and never mutates the old one, because a turn may be
// scoring against it on another goroutine while the update runs.
type index struct {
	fingerprint uint64
	docs        []Entry           // by document id; a tombstone has an empty Path
	terms       [][]string        // by document id: what each contributed
	slots       map[fileRef]int32 // file → document id
	free        []int32           // slots left by removed memories
	postings    map[string][]posting
	entries     []Entry // the live documents, kind then name, for display
}

func newIndex() *index {
	return &index{slots: make(map[fileRef]int32), postings: make(map[string][]posting)}
}

var emptyIndex = newIndex()

// live is how many memories the index holds.
func (x *index) live() int { return len(x.slots) }

// commonCutoff is the posting-list length past which a term stops meaning
// anything. Below commonTermFloor documents no term is common enough to skip.
func (x *index) commonCutoff() int {
	if total := x.live(); total >= commonTermFloor {
		return int(float64(total) * commonTermFraction)
	}
	return x.live()
}

// score walks the posting lists of the query and returns every document it
// touched, weighted. A term carried by most of the directory is skipped: it
// cannot discriminate, and its list is the longest to walk.
func (x *index) score(wanted map[string]float32) map[int32]float32 {
	total := float64(x.live())
	common := x.commonCutoff()
	scores := make(map[int32]float32, 32)
	for term, want := range wanted {
		list := x.postings[term]
		if len(list) == 0 || len(list) > common {
			continue
		}
		// A word is worth what it is rare in this directory. Computed here
		// rather than stored, so an index update stays a local edit.
		idf := float32(math.Log(1 + total/float64(len(list))))
		for _, hit := range list {
			scores[hit.doc] += want * hit.weight * idf
		}
	}
	return scores
}

// index returns the current index, rebuilding it only for what changed.
// Safe for concurrent use: the TUI and a turn share one store.
func (s *Store) index() *index {
	if s == nil {
		return emptyIndex
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cached != nil && !s.dirty && !s.stale() {
		return s.cached
	}

	items, err := os.ReadDir(s.dir)
	if err != nil {
		debuglog.Logf("memory: read %s: %v", s.dir, err)
		return emptyIndex
	}
	refs, fingerprint := scan(items)
	s.dirty = false
	if s.cached == nil {
		s.cached = newIndex()
	}
	s.cached = s.cached.update(s.dir, refs, fingerprint)
	s.mark()
	return s.cached
}

// stale reports whether the directory may have changed under the cached index.
//
// The cheap check is the directory's own mtime: one stat, whatever the
// directory holds, and it moves whenever a file is added, removed or replaced.
// A file edited in place under the same name moves nothing, so a full scan
// runs as the backstop, at most once every verifyInterval — and Invalidate,
// which the engine calls when a turn ends, is what makes a memory the agent
// just wrote visible on the next one.
//
// Callers hold s.mu.
func (s *Store) stale() bool {
	info, err := os.Stat(s.dir)
	if err != nil {
		return true
	}
	if !info.ModTime().Equal(s.dirMod) {
		return true
	}
	if time.Since(s.verified) < verifyInterval {
		return false
	}
	items, err := os.ReadDir(s.dir)
	if err != nil {
		return true
	}
	_, fingerprint := scan(items)
	s.verified = time.Now()
	return fingerprint != s.cached.fingerprint
}

// Invalidate marks the cached index for re-checking. The engine calls it when
// a turn ends: the agent may have rewritten a memory in place, which no cheap
// check can see. The index itself is kept, so the re-check costs the files
// that changed — usually one, often none.
func (s *Store) Invalidate() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.dirty = true
	s.mu.Unlock()
}

// mark records the directory state the cached index was built from.
// Callers hold s.mu.
func (s *Store) mark() {
	if info, err := os.Stat(s.dir); err == nil {
		s.dirMod = info.ModTime()
	}
	s.verified = time.Now()
}

// scan lists the memory files and fingerprints the directory in one pass.
// MEMORY.md is left out of both: the harness rewrites it after every turn, and
// a rebuild triggered by its own index file would never settle.
func scan(items []os.DirEntry) ([]fileRef, uint64) {
	refs := make([]fileRef, 0, len(items))
	hash := fnv.New64a()
	var scratch []byte
	writeInt := func(value int64) {
		scratch = strconv.AppendInt(scratch[:0], value, 10)
		_, _ = hash.Write(scratch)
	}
	for _, item := range items {
		if item.IsDir() || filepath.Ext(item.Name()) != fileExt || item.Name() == IndexFile {
			continue
		}
		info, err := item.Info()
		if err != nil {
			debuglog.Logf("memory: stat %s: %v", item.Name(), err)
			continue
		}
		ref := fileRef{name: item.Name(), mod: info.ModTime().UnixNano(), size: info.Size()}
		refs = append(refs, ref)
		_, _ = hash.Write([]byte(ref.name))
		writeInt(ref.size)
		writeInt(ref.mod)
	}
	return refs, hash.Sum64()
}

// update returns the index for refs: files whose name, size and mtime are
// unchanged are carried over unread, the rest are parsed across the machine's
// cores. Reading the files is what a rebuild costs; everything else is
// arithmetic on what is already in memory.
func (x *index) update(dir string, refs []fileRef, fingerprint uint64) *index {
	var added []fileRef
	keep := make(map[fileRef]bool, len(refs))
	for _, ref := range refs {
		keep[ref] = true
		if _, known := x.slots[ref]; !known {
			added = append(added, ref)
		}
	}
	var removed []fileRef
	for ref := range x.slots {
		if !keep[ref] {
			removed = append(removed, ref)
		}
	}
	if len(added) == 0 && len(removed) == 0 {
		next := *x
		next.fingerprint = fingerprint
		return &next
	}

	parsed := parseFiles(dir, added)
	next := x.copyForWrite()
	next.fingerprint = fingerprint
	// owned names the posting lists this update has already replaced. The
	// first write to a term copies the list the previous index still shares;
	// every write after that appends to one this update owns, which is what
	// keeps a bulk load linear.
	owned := make(map[string]bool, len(added)*8)
	for _, ref := range removed {
		next.drop(ref, owned)
	}
	for i, ref := range added {
		if parsed[i].Path == "" {
			continue
		}
		next.put(ref, parsed[i], owned)
	}
	next.view()
	return next
}

// parseFiles reads and parses one batch. A file that fails to parse is skipped
// and logged — one bad file must never hide the rest — and comes back with an
// empty Path.
func parseFiles(dir string, refs []fileRef) []Entry {
	parsed := make([]Entry, len(refs))
	var wait sync.WaitGroup
	// Reading a file is a syscall wait, not work: more workers than cores is
	// what keeps the disk busy.
	slot := make(chan struct{}, max(runtime.GOMAXPROCS(0)*4, 8))
	for i, ref := range refs {
		wait.Go(func() {
			slot <- struct{}{}
			defer func() { <-slot }()

			entry, err := ParseFile(filepath.Join(dir, ref.name))
			if err != nil {
				debuglog.Logf("memory: skip %s: %v", ref.name, err)
				return
			}
			entry.Modified = time.Unix(0, ref.mod)
			parsed[i] = entry
		})
	}
	wait.Wait()
	return parsed
}

// copyForWrite shallow-copies what an update mutates. Posting lists are shared
// until a term is actually touched, so the copy costs distinct terms, not
// postings.
func (x *index) copyForWrite() *index {
	return &index{
		docs:     slices.Clone(x.docs),
		terms:    slices.Clone(x.terms),
		slots:    maps.Clone(x.slots),
		free:     slices.Clone(x.free),
		postings: maps.Clone(x.postings),
	}
}

// put files one memory into a free slot and adds it to the posting lists.
func (x *index) put(ref fileRef, entry Entry, owned map[string]bool) {
	var doc int32
	if last := len(x.free) - 1; last >= 0 {
		doc, x.free = x.free[last], x.free[:last]
	} else {
		//nolint:gosec // G115: a directory of two billion memory files is not a case
		doc = int32(len(x.docs))
		x.docs = append(x.docs, Entry{})
		x.terms = append(x.terms, nil)
	}
	x.docs[doc] = entry
	x.slots[ref] = doc

	weights := make(map[string]float32, 128)
	place := func(text string, weight float32) {
		eachTerm(text, maxIndexedRunes, func(term string) {
			if weights[term] < weight {
				weights[term] = weight
			}
		})
	}
	place(entry.Body, 1)
	place(entry.Description, 2)
	place(entry.Name, 3)

	terms := make([]string, 0, len(weights))
	for term, weight := range weights {
		terms = append(terms, term)
		list := x.postings[term]
		if !owned[term] {
			// Clip so append copies instead of writing into a list the
			// previous index still shares.
			list = slices.Clip(list)
			owned[term] = true
		}
		x.postings[term] = append(list, posting{doc: doc, weight: weight})
	}
	x.terms[doc] = terms
}

// drop removes one memory, leaving a tombstone its slot can be reused from.
func (x *index) drop(ref fileRef, owned map[string]bool) {
	doc, known := x.slots[ref]
	if !known {
		return
	}
	for _, term := range x.terms[doc] {
		// without copies, so the term is this update's afterwards.
		if list := without(x.postings[term], doc); len(list) == 0 {
			delete(x.postings, term)
			delete(owned, term)
		} else {
			x.postings[term] = list
			owned[term] = true
		}
	}
	delete(x.slots, ref)
	x.docs[doc] = Entry{}
	x.terms[doc] = nil
	x.free = append(x.free, doc)
}

// without returns the list minus one document, as a new slice: the old one may
// still be read by a turn scoring against the previous index.
func without(list []posting, doc int32) []posting {
	for i, hit := range list {
		if hit.doc != doc {
			continue
		}
		out := make([]posting, 0, len(list)-1)
		out = append(out, list[:i]...)
		return append(out, list[i+1:]...)
	}
	return list
}

// view rebuilds the ordered, tombstone-free slice everything that renders
// memory reads.
func (x *index) view() {
	entries := make([]Entry, 0, len(x.slots))
	for _, entry := range x.docs {
		if entry.Path != "" {
			entries = append(entries, entry)
		}
	}
	slices.SortFunc(entries, byKindThenName)
	x.entries = entries
}
