package memory

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tree is the shape the harness finds on disk: a canonical store beside a
// projects root with one corpus per repository. A test drives one of those
// corpora through the store's public calls and reads the others back.
type tree struct {
	t         *testing.T
	root      string
	canonical string
	projects  string
}

// homeCorpus is the corpus a session in these tests reads; it is named first
// in every tree.
const homeCorpus = "alpha"

func newTree(t *testing.T, corpora ...string) tree {
	t.Helper()
	root := t.TempDir()
	tr := tree{
		t:         t,
		root:      root,
		canonical: filepath.Join(root, "cozyphi", "memory"),
		projects:  filepath.Join(root, "projects"),
	}
	require.NoError(t, os.MkdirAll(tr.canonical, 0o755))
	for _, name := range corpora {
		require.NoError(t, os.MkdirAll(tr.corpus(name), 0o755))
	}
	return tr
}

func (tr tree) corpus(name string) string { return filepath.Join(tr.projects, name, corpusDir) }

func (tr tree) registry() Registry {
	return Registry{Canonical: tr.canonical, Corpora: tr.projects}
}

// open gives the session its one readable corpus. Every tree here is laid out
// with homeCorpus first, and that is the one a session in these tests reads;
// the others are what the harness copies into.
func (tr tree) open() *Store {
	tr.t.Helper()
	store, err := Open(tr.corpus(homeCorpus), nil, tr.registry())
	require.NoError(tr.t, err)
	return store
}

// snapshot is every file in the tree with its size, its time and its bytes:
// what a pass that changed nothing has to leave exactly as it found it.
func (tr tree) snapshot() map[string]string {
	tr.t.Helper()
	files := make(map[string]string)
	require.NoError(tr.t, filepath.WalkDir(tr.root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[path] = fmt.Sprintf("%d %s %s", info.Size(), info.ModTime().UTC().Format(time.RFC3339Nano), data)
		return nil
	}))
	return files
}

// bornGlobal is that same memory carrying the scope key already, the way a
// file written by hand or by Claude Code does.
func bornGlobal(body string) string {
	return "---\nname: " + russianName + "\ndescription: " + russianAbout +
		"\nscope: global\nmetadata:\n  type: feedback\n---\n" + body + "\n"
}

func modTime(t *testing.T, path string) time.Time {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	return info.ModTime()
}

// stamped writes a file with a modification time of the test's choosing, which
// is how a test says "this edit came later".
func stamped(t *testing.T, path, content string, when time.Time) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	require.NoError(t, os.Chtimes(path, when, when))
}

// The fact these tests publish: one standing rule, in its local form.
const (
	russianName  = "speak-russian"
	russianFile  = "speak-russian.md"
	russianAbout = "The user writes in Russian."
	russianBody  = "Отвечай по-русски."
)

func russian() string { return fact(russianName, "feedback", russianAbout, russianBody) }

// publish puts one fact in the store and marks it global, which is the state
// most of these tests start from.
func publish(t *testing.T, store *Store, file, content string) Fanout {
	t.Helper()
	write(t, store.Dir(), file, content)
	store.Invalidate()
	_, report, err := store.MarkGlobal(strings.TrimSuffix(file, fileExt))
	require.NoError(t, err)
	return report
}

func TestMarkGlobalCopiesTheFactIntoEveryCorpusAndTheCanonicalStore(t *testing.T) {
	tr := newTree(t, "alpha", "beta", "gamma")
	store := tr.open()

	report := publish(t, store, russianFile, russian())
	assert.Equal(t, []string{"speak-russian"}, report.Facts)
	assert.Equal(t, 3, report.Dirs)
	assert.Empty(t, report.Clashes)

	source, err := os.ReadFile(filepath.Join(store.Dir(), russianFile))
	require.NoError(t, err)
	assert.Contains(t, string(source), "scope: global")

	entry, ok := store.Fact("speak-russian")
	require.True(t, ok)
	assert.True(t, entry.Global, "the fact stays here and reads as global")

	for _, dir := range []string{tr.corpus("beta"), tr.corpus("gamma"), tr.canonical} {
		copied, err := os.ReadFile(filepath.Join(dir, russianFile))
		require.NoError(t, err, dir)
		assert.Equal(t, string(source), string(copied), dir)
		assert.FileExists(t, filepath.Join(dir, IndexFile), dir)
	}
}

func TestACopyCarriesTheSourceTimeAndASettledTreeIsNotWrittenAgain(t *testing.T) {
	tr := newTree(t, "alpha", "beta")
	store := tr.open()
	publish(t, store, russianFile, russian())

	want := modTime(t, filepath.Join(store.Dir(), russianFile))
	for _, dir := range []string{tr.corpus("beta"), tr.canonical} {
		assert.True(t, modTime(t, filepath.Join(dir, russianFile)).Equal(want),
			"%s should carry the source's time", dir)
	}

	before := tr.snapshot()
	report := store.Reconcile()
	assert.Empty(t, report.Facts)
	assert.Zero(t, report.Dirs)
	assert.Empty(t, report.Clashes)
	assert.Equal(t, before, tr.snapshot(), "a settled tree is not rewritten")
}

func TestTheNewestCopyOfAGlobalFactWinsWhereverItWasWritten(t *testing.T) {
	cases := []struct {
		name  string
		where func(tr tree) string
	}{
		{"another corpus", func(tr tree) string { return tr.corpus("beta") }},
		{"the canonical store", func(tr tree) string { return tr.canonical }},
		{"this corpus", func(tr tree) string { return tr.corpus("alpha") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tr := newTree(t, "alpha", "beta", "gamma")
			store := tr.open()
			publish(t, store, russianFile, russian())

			edited := bornGlobal("Отвечай по-русски, всегда.")
			stamped(t, filepath.Join(tc.where(tr), russianFile), edited, time.Now().Add(time.Minute))

			store.Invalidate()
			report := store.Reconcile()
			assert.Equal(t, []string{"speak-russian"}, report.Facts)
			assert.Equal(t, 3, report.Dirs)

			for _, dir := range []string{store.Dir(), tr.corpus("beta"), tr.corpus("gamma"), tr.canonical} {
				data, err := os.ReadFile(filepath.Join(dir, russianFile))
				require.NoError(t, err, dir)
				assert.Equal(t, edited, string(data), dir)
			}
		})
	}
}

func TestACorpusKeepsItsOwnMemoryOfTheSameNameAndTheClashIsReported(t *testing.T) {
	tr := newTree(t, "alpha", "beta", "gamma")
	store := tr.open()

	local := fact("release-freeze", "project", "Beta freezes in December.", "Beta's own freeze.")
	write(t, tr.corpus("beta"), "release-freeze.md", local)

	report := publish(t, store, "release-freeze.md",
		fact("release-freeze", "feedback", "Never release on a Friday.", "The global rule."))

	assert.Equal(t, []Clash{{Name: "release-freeze", Dir: tr.corpus("beta")}}, report.Clashes)
	assert.Equal(t, []string{"release-freeze"}, report.Facts)
	assert.Equal(t, 2, report.Dirs, "the publish succeeds everywhere else")

	kept, err := os.ReadFile(filepath.Join(tr.corpus("beta"), "release-freeze.md"))
	require.NoError(t, err)
	assert.Equal(t, local, string(kept), "the local file is never overwritten")

	for _, dir := range []string{tr.corpus("gamma"), tr.canonical} {
		data, err := os.ReadFile(filepath.Join(dir, "release-freeze.md"))
		require.NoError(t, err, dir)
		assert.Contains(t, string(data), "The global rule.", dir)
	}
}

func TestAFileWrittenWithTheScopeKeyIsAdoptedWithoutACommand(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"flat key", bornGlobal(russianBody)},
		{"nested under metadata", "---\nname: speak-russian\ndescription: The user writes in Russian.\n" +
			"metadata:\n  type: feedback\n  scope: global\n---\nОтвечай по-русски.\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tr := newTree(t, "alpha", "beta")
			store := tr.open()
			write(t, store.Dir(), russianFile, tc.raw)
			store.Invalidate()

			report := store.Reconcile()
			assert.Equal(t, []string{"speak-russian"}, report.Facts)

			entry, ok := store.Fact("speak-russian")
			require.True(t, ok)
			assert.True(t, entry.Global)
			for _, dir := range []string{tr.corpus("beta"), tr.canonical} {
				data, err := os.ReadFile(filepath.Join(dir, russianFile))
				require.NoError(t, err, dir)
				assert.Equal(t, tc.raw, string(data), dir)
			}
		})
	}
}

func TestMarkLocalKeepsTheFactHereAndArchivesTheCopiesElsewhere(t *testing.T) {
	tr := newTree(t, "alpha", "beta", "gamma")
	store := tr.open()
	publish(t, store, russianFile, russian())

	entry, err := store.MarkLocal("speak-russian")
	require.NoError(t, err)
	assert.False(t, entry.Global)

	here, err := os.ReadFile(filepath.Join(store.Dir(), russianFile))
	require.NoError(t, err)
	assert.NotContains(t, frontmatterOf(here), "scope:")
	assert.Contains(t, string(here), "Отвечай по-русски.")

	for _, dir := range []string{tr.corpus("beta"), tr.corpus("gamma"), tr.canonical} {
		assert.NoFileExists(t, filepath.Join(dir, russianFile), dir)
		assert.FileExists(t, filepath.Join(dir, forgottenDir, russianFile), dir)
	}

	store.Invalidate()
	report := store.Reconcile()
	assert.Empty(t, report.Facts, "an un-published fact does not come back")
	assert.NoFileExists(t, filepath.Join(tr.corpus("beta"), russianFile))
}

// frontmatterOf is a file's frontmatter, the only place a scope key may appear.
func frontmatterOf(raw []byte) string {
	head, _, _ := strings.Cut(strings.TrimPrefix(string(raw), "---\n"), "\n---")
	return head
}

func TestMarkLocalRefusesAFactThatIsNotGlobalHere(t *testing.T) {
	tr := newTree(t, "alpha", "beta")
	store := tr.open()
	write(t, store.Dir(), russianFile, russian())
	store.Invalidate()

	_, err := store.MarkLocal("speak-russian")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not global here")
}

func TestForgettingAGlobalFactArchivesItInEveryCorpus(t *testing.T) {
	tr := newTree(t, "alpha", "beta", "gamma")
	store := tr.open()
	publish(t, store, russianFile, russian())

	entry, err := store.Forget("speak-russian")
	require.NoError(t, err)
	assert.True(t, entry.Global)

	for _, dir := range []string{store.Dir(), tr.corpus("beta"), tr.corpus("gamma"), tr.canonical} {
		assert.NoFileExists(t, filepath.Join(dir, russianFile), dir)
		assert.FileExists(t, filepath.Join(dir, forgottenDir, russianFile), dir)
	}

	assert.Empty(t, store.Reconcile().Facts, "a forgotten fact does not come back")
	assert.NoFileExists(t, filepath.Join(tr.corpus("beta"), russianFile))
}

func TestForgetRefusesAPinnedGlobalFactAndLeavesEveryCopyInPlace(t *testing.T) {
	tr := newTree(t, "alpha", "beta")
	store := tr.open()
	write(t, store.Dir(), russianFile, "---\nname: speak-russian\ndescription: The user writes in Russian.\n"+
		"scope: global\nmetadata:\n  type: feedback\n  pin: true\n---\nОтвечай по-русски.\n")
	store.Invalidate()
	store.Reconcile()

	_, err := store.Forget("speak-russian")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pinned")
	for _, dir := range []string{store.Dir(), tr.corpus("beta"), tr.canonical} {
		assert.FileExists(t, filepath.Join(dir, russianFile), dir)
	}
}

func TestACorpusTheFactNeverReachedIsSeededOnItsFirstPass(t *testing.T) {
	tr := newTree(t, "alpha", "beta")
	store := tr.open()
	publish(t, store, russianFile, russian())

	// A repository opened for the first time after the fact was published.
	require.NoError(t, os.MkdirAll(tr.corpus("delta"), 0o755))

	report := store.Reconcile()
	assert.Equal(t, []string{"speak-russian"}, report.Facts)
	assert.Equal(t, 1, report.Dirs)

	data, err := os.ReadFile(filepath.Join(tr.corpus("delta"), russianFile))
	require.NoError(t, err)
	assert.Contains(t, string(data), "Отвечай по-русски.")
	catalog, err := os.ReadFile(filepath.Join(tr.corpus("delta"), IndexFile))
	require.NoError(t, err)
	assert.Contains(t, string(catalog), "speak-russian")
}

func TestReconcileRemovesNothingItWasNotAskedTo(t *testing.T) {
	tr := newTree(t, "alpha", "beta")
	// The canonical store has never been written; it is not even there.
	require.NoError(t, os.RemoveAll(tr.canonical))

	store := tr.open()
	write(t, store.Dir(), "alpha-layout.md", fact("alpha-layout", "project", "Alpha keeps its docs in doc/.", "Body."))
	write(t, tr.corpus("beta"), "beta-layout.md",
		fact("beta-layout", "project", "Beta keeps its docs in docs/.", "Body."))
	publish(t, store, russianFile, russian())

	assert.FileExists(t, filepath.Join(store.Dir(), "alpha-layout.md"))
	assert.FileExists(t, filepath.Join(tr.corpus("beta"), "beta-layout.md"))
	assert.FileExists(t, filepath.Join(tr.canonical, russianFile), "the canonical store is created on demand")
	assert.NoFileExists(t, filepath.Join(tr.canonical, "alpha-layout.md"), "a local fact travels nowhere")

	// A copy deleted by hand is not a forget: the pass puts it back rather
	// than taking the deletion as an instruction for the other corpora.
	require.NoError(t, os.Remove(filepath.Join(store.Dir(), russianFile)))
	store.Invalidate()
	store.Reconcile()
	assert.FileExists(t, filepath.Join(store.Dir(), russianFile))
	assert.FileExists(t, filepath.Join(tr.corpus("beta"), russianFile))
}

func TestTheCatalogIsRewrittenWhereACopyLandedAndNowhereElse(t *testing.T) {
	tr := newTree(t, "alpha", "beta", "gamma")
	store := tr.open()

	// Beta holds its own memory of that name, so nothing is written there.
	write(t, tr.corpus("beta"), "release-freeze.md",
		fact("release-freeze", "project", "Beta freezes in December.", "Beta's own freeze."))
	require.NoError(t, syncIndexIn(tr.corpus("beta")))
	untouched := filepath.Join(tr.corpus("beta"), IndexFile)
	before, err := os.ReadFile(untouched)
	require.NoError(t, err)
	when := time.Now().Add(-time.Hour).Truncate(time.Second)
	require.NoError(t, os.Chtimes(untouched, when, when))

	publish(t, store, "release-freeze.md",
		fact("release-freeze", "feedback", "Never release on a Friday.", "The global rule."))
	_, err = store.SyncIndex()
	require.NoError(t, err)

	for _, dir := range []string{store.Dir(), tr.corpus("gamma"), tr.canonical} {
		catalog, err := os.ReadFile(filepath.Join(dir, IndexFile))
		require.NoError(t, err, dir)
		assert.Contains(t, string(catalog), "release-freeze", dir)
	}

	after, err := os.ReadFile(untouched)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))
	assert.True(t, modTime(t, untouched).Equal(when), "beta's catalog was not rewritten")
}

func TestPublishingAndTakingItBackLeavesTheFileAsItWas(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"flat keys", "---\nname: speak-russian\ndescription: The user writes in Russian.\n---\nОтвечай по-русски.\n"},
		{"nested metadata", russian()},
		{"folded description", "---\nname: speak-russian\ndescription: >\n  The user writes in Russian\n" +
			"  and expects an answer in it.\nmetadata:\n  type: feedback\n---\nОтвечай по-русски.\n"},
		{"crlf", strings.ReplaceAll(russian(), "\n", "\r\n")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tr := newTree(t, "alpha", "beta")
			store := tr.open()
			write(t, store.Dir(), russianFile, tc.raw)
			store.Invalidate()

			_, _, err := store.MarkGlobal("speak-russian")
			require.NoError(t, err)
			entry, ok := store.Fact("speak-russian")
			require.True(t, ok)
			assert.True(t, entry.Global, "the key the harness wrote reads back as global")
			assert.FileExists(t, filepath.Join(tr.corpus("beta"), russianFile))

			_, err = store.MarkLocal("speak-russian")
			require.NoError(t, err)
			back, err := os.ReadFile(filepath.Join(store.Dir(), russianFile))
			require.NoError(t, err)
			assert.Equal(t, tc.raw, string(back))
		})
	}
}

func TestAStoreWithoutARegistryCopiesNothingAnywhere(t *testing.T) {
	tr := newTree(t, "alpha", "beta")
	store, err := Open(tr.corpus(homeCorpus), nil, Registry{})
	require.NoError(t, err)
	write(t, store.Dir(), russianFile, bornGlobal(russianBody))
	store.Invalidate()

	report := store.Reconcile()
	assert.Empty(t, report.Facts)
	assert.Zero(t, report.Dirs)
	assert.NoFileExists(t, filepath.Join(tr.corpus("beta"), russianFile))
	assert.NoFileExists(t, filepath.Join(tr.canonical, russianFile))
}

func TestThePromptSaysWhatTheLastPassCopiedAndWhatItCouldNot(t *testing.T) {
	tr := newTree(t, "alpha", "beta", "gamma")
	store := tr.open()
	write(t, tr.corpus("beta"), "release-freeze.md",
		fact("release-freeze", "project", "Beta freezes in December.", "Beta's own freeze."))

	publish(t, store, "release-freeze.md",
		fact("release-freeze", "feedback", "Never release on a Friday.", "The global rule."))

	block := store.PromptBlock()
	assert.Contains(t, block, "## Memory needs attention")
	assert.Contains(t, block, "copied into 2 corpora just now: release-freeze.")
	assert.Contains(t, block, "has its own memory of that name and kept it")
	assert.Contains(t, block, tr.corpus("beta"))
	assert.NotContains(t, block, "Room here is finite", "a fan-out is news, not pressure")

	// The next pass copies nothing; the clash is still true, and still said.
	store.Reconcile()
	settled := store.PromptBlock()
	assert.NotContains(t, settled, "copied into")
	assert.Contains(t, settled, "has its own memory of that name and kept it")
}

func TestThePromptGoesQuietOnceTheCopiesHaveSettled(t *testing.T) {
	tr := newTree(t, "alpha", "beta")
	store := tr.open()
	publish(t, store, russianFile, russian())

	assert.Contains(t, store.PromptBlock(), "copied into 2 corpora just now: speak-russian.")

	store.Reconcile()
	assert.NotContains(t, store.PromptBlock(), "Memory needs attention",
		"a settled tree has nothing to report")
}

func TestCompactionNeverArchivesTheGlobalTwin(t *testing.T) {
	tr := newTree(t, "alpha", "beta")
	store := tr.open()
	write(t, store.Dir(), russianFile, bornGlobal(russianBody))
	// The same fact, saved again under another name and more recently.
	write(t, store.Dir(), "russian-answers.md",
		fact("russian-answers", "feedback", russianAbout, russianBody))
	store.Invalidate()
	store.Reconcile()

	assert.Equal(t, []string{"russian-answers"}, store.Compact(),
		"the local twin goes, not the fact every repository is reading")
	assert.FileExists(t, filepath.Join(store.Dir(), russianFile))
	assert.FileExists(t, filepath.Join(tr.corpus("beta"), russianFile))
	assert.FileExists(t, filepath.Join(store.Dir(), forgottenDir, "russian-answers.md"))
}
