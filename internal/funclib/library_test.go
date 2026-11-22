package funclib_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/funclib"
	"github.com/alvnukov/cozyphi/internal/tools/writetool"
)

func validation(_ context.Context, p funclib.Package, modules map[string]string) error {
	if p.Source == "" || modules[p.Name+"/index.ts"] != p.Source || modules[p.Name+"/index.test.ts"] != p.Tests {
		return errors.New("wrong test snapshot")
	}
	return nil
}

func create(t *testing.T, s *funclib.Store, session, pkg, src, tests string) {
	t.Helper()
	_, err := s.Draft(t.Context(), session, pkg, "")
	require.NoError(t, err)
	_, err = s.CreateFile(t.Context(), session, pkg, "index.ts", src)
	require.NoError(t, err)
	_, err = s.CreateFile(t.Context(), session, pkg, "index.test.ts", tests)
	require.NoError(t, err)
}

func replace(t *testing.T, s *funclib.Store, session, name, file, old, new string) {
	t.Helper()
	body, err := s.ReadEditable(t.Context(), session, name, file)
	require.NoError(t, err)
	lines := strings.Split(body, "\n")
	_, hash, _ := strings.Cut(lines[0], "#")
	for _, line := range lines[1:] {
		ref, text, ok := strings.Cut(line, "|")
		if text == old && ok {
			_, err = s.Edit(
				t.Context(),
				session,
				name,
				file,
				hash,
				[]writetool.FlatEdit{{From: ref, To: ref, Content: &new}},
			)
			require.NoError(t, err)
			return
		}
	}
	t.Fatalf("line %q missing from %s", old, body)
}

func TestImmutableReleasesAndProjectPins(t *testing.T) {
	ctx := t.Context()
	root := filepath.Join(t.TempDir(), "functions")
	a, b := t.TempDir(), t.TempDir()
	s := funclib.New(root)
	list, err := s.Catalog(ctx, a)
	require.NoError(t, err)
	require.Empty(t, list)
	_, err = os.Stat(root)
	require.ErrorIs(t, err, os.ErrNotExist)
	create(t, s, "author", "builds", "one", "test")
	draft, err := s.Draft(ctx, "author", "builds", "")
	require.NoError(t, err)
	require.Equal(t, "one", draft.Source)
	_, err = s.Publish(ctx, "author", "builds", "1.0.0", nil)
	require.Error(t, err)
	first, err := s.Publish(ctx, "author", "builds", "1.0.0", validation)
	require.NoError(t, err)
	mods, pins, err := s.ResolvePinned(ctx, a, []string{"builds"})
	require.NoError(t, err)
	require.Equal(t, "one", mods["builds/index.ts"])
	require.Equal(t, first.Digest, pins["builds"].Digest)
	_, err = s.Draft(ctx, "author", "builds", "")
	require.NoError(t, err)
	replace(t, s, "author", "builds", "index.ts", "one", "two")
	_, err = s.Publish(ctx, "author", "builds", "1.0.0", validation)
	require.ErrorContains(t, err, "already exists")
	second, err := s.Publish(ctx, "author", "builds", "1.1.0", validation)
	require.NoError(t, err)
	fresh := funclib.New(root)
	am, err := fresh.Resolve(ctx, a, []string{"builds"})
	require.NoError(t, err)
	require.Equal(t, "one", am["builds/index.ts"])
	bm, err := fresh.Resolve(ctx, b, []string{"builds"})
	require.NoError(t, err)
	require.Equal(t, "two", bm["builds/index.ts"])
	versions, err := fresh.Catalog(ctx, a)
	require.NoError(t, err)
	require.Equal(t, first.Digest, versions[0].Digest)
	p, err := fresh.Pin(ctx, a, "builds", "1.1.0")
	require.NoError(t, err)
	require.Equal(t, second.Digest, p.Digest)
	_, err = fresh.Pin(ctx, a, "builds", "1.0.0")
	require.NoError(t, err)
	pm, err := fresh.Preview(ctx, a, []string{"builds"})
	require.NoError(t, err)
	require.Equal(t, "one", pm["builds/index.ts"])
}

func TestCompetingPublisherAndHashlineGrants(t *testing.T) {
	ctx := t.Context()
	root := filepath.Join(t.TempDir(), "functions")
	a, b := funclib.New(root), funclib.New(root)
	create(t, a, "initial", "x", "hello\nneutral\nlast", "test")
	_, err := a.Publish(ctx, "initial", "x", "1.0.0", validation)
	require.NoError(t, err)
	for _, session := range []string{"a", "b"} {
		_, err = a.Draft(ctx, session, "x", "")
		require.NoError(t, err)
	}
	body, err := a.ReadEditable(ctx, "a", "x", "index.ts")
	require.NoError(t, err)
	lines := strings.Split(body, "\n")
	_, hash, _ := strings.Cut(lines[0], "#")
	ref, _, _ := strings.Cut(lines[1], "|")
	fromA := []writetool.FlatEdit{{From: ref, To: ref, Content: new("new")}}
	_, err = a.Edit(ctx, "b", "x", "index.ts", hash, fromA)
	require.ErrorContains(t, err, "no_capability")
	_, err = a.Edit(ctx, "a", "x", "index.ts", hash, fromA)
	require.NoError(t, err)
	_, err = a.Edit(ctx, "a", "x", "index.ts", hash, fromA)
	require.Error(t, err)
	replace(t, b, "b", "x", "index.ts", "last", "end")
	start, finish := make(chan struct{}), make(chan struct{})
	result := make(chan error, 1)
	go func() {
		_, publishErr := b.Publish(
			ctx,
			"b",
			"x",
			"1.2.0",
			func(context.Context, funclib.Package, map[string]string) error { close(start); <-finish; return nil },
		)
		result <- publishErr
	}()
	<-start
	_, err = a.Publish(ctx, "a", "x", "1.1.0", validation)
	close(finish)
	require.NoError(t, err)
	require.Error(t, <-result)
	_, err = a.Get(ctx, "x", "1.2.0")
	require.Error(t, err)
	rebased, err := b.Rebase(ctx, "b", "x")
	require.NoError(t, err)
	require.Equal(t, "new\nneutral\nend", rebased.Source)
	_, err = b.Publish(ctx, "b", "x", "1.2.0", validation)
	require.NoError(t, err)
}

func TestDependenciesValidatePinnedSourceAndKeepFailedCandidate(t *testing.T) {
	ctx := t.Context()
	s := funclib.New(filepath.Join(t.TempDir(), "functions"))
	create(t, s, "s", "dep", "one", "tests")
	d, err := s.Publish(ctx, "s", "dep", "1.0.0", validation)
	require.NoError(t, err)
	create(t, s, "s", "caller", "call", "checks")
	_, err = s.CreateFile(
		ctx,
		"s",
		"caller",
		"package.json",
		`{"description":"call dep","dependencies":{"dep":"1.0.0"}}`,
	)
	require.NoError(t, err)
	_, err = s.Publish(ctx, "s", "caller", "1.0.0", validation)
	require.NoError(t, err)
	_, err = s.Draft(ctx, "s", "dep", "")
	require.NoError(t, err)
	replace(t, s, "s", "dep", "index.ts", "one", "wrong")
	_, err = s.Publish(ctx, "s", "dep", "1.1.0", func(_ context.Context, p funclib.Package, m map[string]string) error {
		if p.Name == "caller" && m["dep/index.ts"] == "wrong" {
			return errors.New("incompatible result")
		}
		return nil
	})
	require.ErrorContains(t, err, "dependent caller")
	p, err := s.Get(ctx, "dep", "")
	require.NoError(t, err)
	require.Equal(t, d.Digest, p.Digest)
	_, err = s.Get(ctx, "dep", "1.1.0")
	require.Error(t, err)
	ds, err := s.Drafts(ctx, "s")
	require.NoError(t, err)
	require.Len(t, ds, 1)
	mods, pins, err := s.ResolvePinned(ctx, t.TempDir(), []string{"caller"})
	require.NoError(t, err)
	require.Len(t, mods, 2)
	require.Equal(t, "one", mods["dep/index.ts"])
	require.Equal(t, d.Digest, pins["dep"].Digest)
}

func TestMatchingDirectAndTransitivePins(t *testing.T) {
	ctx := t.Context()
	s := funclib.New(filepath.Join(t.TempDir(), "functions"))
	project := t.TempDir()
	create(t, s, "a", "base", "one", "test")
	_, err := s.Publish(ctx, "a", "base", "1.0.0", validation)
	require.NoError(t, err)
	create(t, s, "a", "use", "caller", "tests")
	_, err = s.CreateFile(ctx, "a", "use", "package.json", `{"dependencies":{"base":"1.0.0"}}`)
	require.NoError(t, err)
	_, err = s.Publish(ctx, "a", "use", "1.0.0", validation)
	require.NoError(t, err)
	_, err = s.Draft(ctx, "b", "base", "")
	require.NoError(t, err)
	replace(t, s, "b", "base", "index.ts", "one", "two")
	_, err = s.Publish(ctx, "b", "base", "2.0.0", validation)
	require.NoError(t, err)
	modules, pins, err := s.ResolvePinned(ctx, project, []string{"base", "use"})
	require.NoError(t, err)
	require.Equal(t, "one", modules["base/index.ts"])
	require.Equal(t, "1.0.0", pins["base"].Version)
	_, err = s.Draft(ctx, "b", "use", "")
	require.NoError(t, err)
	replace(t, s, "b", "use", "package.json", `{"dependencies":{"base":"1.0.0"}}`, `{"dependencies":{"base":"2.0.0"}}`)
	_, err = s.Publish(ctx, "b", "use", "2.0.0", validation)
	require.NoError(t, err)
	_, err = s.Pin(ctx, project, "use", "2.0.0")
	require.Error(t, err)
	pins, err = s.PinSet(ctx, project, map[string]string{"base": "2.0.0", "use": "2.0.0"})
	require.NoError(t, err)
	require.Equal(t, "2.0.0", pins["base"].Version)
}

func TestLibraryBoundaryAndMissingFiles(t *testing.T) {
	ctx := t.Context()
	s := funclib.New(filepath.Join(t.TempDir(), "functions"))
	_, err := s.Draft(ctx, "s", "../../oops", "")
	require.Error(t, err)
	_, err = s.Draft(ctx, "s", "pkg", "")
	require.NoError(t, err)
	_, err = s.ReadRange(ctx, "s", "pkg", "index.ts", 1, 10)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = s.CreateFile(ctx, "s", "pkg", "../index.ts", "bad")
	require.Error(t, err)
	_, err = s.CreateFile(ctx, "s", "pkg", "index.ts", "ok")
	require.NoError(t, err)
	_, err = s.CreateFile(ctx, "s", "pkg", "index.ts", "override")
	require.Error(t, err)
	_, err = s.Publish(ctx, "s", "pkg", "1.0.0", validation)
	require.ErrorContains(t, err, "nonempty")
}

func TestPortableLockAndDigestBoundApproval(t *testing.T) {
	ctx := t.Context()
	s := funclib.New(filepath.Join(t.TempDir(), "functions"))
	create(t, s, "s", "pkg", "one", "test")
	before, diff, err := s.DraftDiff(ctx, "s", "pkg")
	require.NoError(t, err)
	require.Contains(t, diff, "+one")
	replace(t, s, "s", "pkg", "index.ts", "one", "two")
	_, err = s.PublishAt(ctx, "s", "pkg", "1.0.0", before.Digest, validation)
	require.ErrorContains(t, err, "changed since preview")
	current, err := s.DraftState(ctx, "s", "pkg")
	require.NoError(t, err)
	pkg, err := s.PublishAt(ctx, "s", "pkg", "1.0.0", current.Digest, validation)
	require.NoError(t, err)
	a := t.TempDir()
	insert := func(oid string) {
		t.Helper()
		require.NoError(
			t,
			os.WriteFile(
				filepath.Join(a, "cozyphi.functions.lock"),
				[]byte(`{"packages":{"pkg":{"version":"1.0.0","digest":"`+oid+`"}}}`),
				0o600,
			),
		)
	}
	insert(pkg.Digest)
	_, err = s.Draft(ctx, "s", "pkg", "")
	require.NoError(t, err)
	_, err = s.Publish(ctx, "s", "pkg", "2.0.0", validation)
	require.NoError(t, err)
	r, d, err := s.Inventory(ctx, a, "s")
	require.NoError(t, err)
	require.Empty(t, d)
	require.Len(t, r, 1)
	require.Equal(t, pkg.Digest, r[0].Digest)
	require.Empty(t, r[0].Source)
	_, err = s.Pin(ctx, a, "pkg", "2.0.0")
	require.ErrorContains(t, err, "cozyphi.functions.lock")
	other, err := s.Get(ctx, "pkg", "2.0.0")
	require.NoError(t, err)
	insert(other.Digest)
	_, err = s.Preview(ctx, a, []string{"pkg"})
	require.Error(t, err)
}
