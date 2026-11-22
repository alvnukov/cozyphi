package funcadvisor

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/alvnukov/cozyphi/internal/funclib"
	"github.com/alvnukov/cozyphi/internal/llm"
)

type countingCatalog struct {
	catalog
	loads int
}

func (c *countingCatalog) Load(ctx context.Context, digest string) (funclib.Package, error) {
	c.loads++
	return c.catalog.Load(ctx, digest)
}

func TestReleaseIndexSurvivesRestartAndRebuildsFromGit(t *testing.T) {
	root := t.TempDir()
	store := funclib.New(root)
	p := builds()
	p.Digest = strings.Repeat("a", 40)
	if cards := IndexRelease(store, p); len(cards) != 2 {
		t.Fatal(cards)
	}
	path := slotPath(root, p.Digest)
	saved, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(saved), "const documentation") {
		t.Fatalf("stored raw source or failed to store cards: %v", err)
	}
	cat := &countingCatalog{catalog: catalog{packages: []funclib.Package{p}}}
	query := func(wantLoads int) {
		t.Helper()
		s := New(store, nil, 0)
		s.store = cat
		got := decodeHints(t, s.Hints(t.Context(), "owner", "work", "failedPackages", nil))
		if len(got) != 1 || got[0].Digest != p.Digest || cat.loads != wantLoads {
			t.Fatalf("cards=%+v, loads=%d, want %d", got, cat.loads, wantLoads)
		}
	}
	query(0)
	other := p
	other.Digest = "aaa" + strings.Repeat("b", 37)
	other.Name = "neighbor"
	IndexRelease(store, other)
	query(1) // The other commit occupies the same bounded slot.
	query(1) // Rebuilding the original release repopulated its slot.
	for i, broken := range []string{strings.Repeat("x", maxSlotBytes+1), strings.ReplaceAll(string(saved), "failedPackages", "changedName")} {
		if err := os.WriteFile(path, []byte(broken), 0o600); err != nil {
			t.Fatal(err)
		}
		query(i + 2)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	query(4)
}

func TestSessionDraftsNeverEnterTheDiskIndex(t *testing.T) {
	store := funclib.New(t.TempDir())
	p := builds()
	p.Digest = strings.Repeat("a", 40)
	IndexRelease(store, p)
	path := slotPath(store.Root(), p.Digest)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	draft := funclib.Package{Name: "drafted", Source: "export function hiddenWork() {}", Digest: "aaa" + strings.Repeat("c", 37)}
	cat := &countingCatalog{catalog: catalog{private: map[string][]funclib.Package{"owner": {draft}}}}
	s := New(store, nil, 0)
	s.store = cat
	private := decodeHints(t, s.Hints(t.Context(), "owner", "work", "hiddenWork", nil))
	if len(private) != 1 || !private[0].Draft || cat.loads != 1 {
		t.Fatal(private, cat.loads)
	}
	if hints := s.Hints(t.Context(), "other", "work", "hiddenWork", nil); hints != "" {
		t.Fatal(hints)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatalf("draft changed persisted release metadata: %v", err)
	}
}

func TestCatalogReflectsCompiledExportsAndAvailableTools(t *testing.T) {
	p := funclib.Package{Name: "helpers", Version: "1.0.0", Digest: "v1", Source: `export type Hidden = number;
function impl() { return 42; }
export {impl as alias, impl as "non-standard"}; export default impl;
export async function change() { return tools.call("write", {}); }`}
	var names []string
	for _, card := range Exports(p) {
		names = append(names, card.Name)
		if card.Name == "non-standard" && !strings.Contains(card.Use, `"non-standard" as libraryExport`) {
			t.Fatal(card.Use)
		}
	}
	if !reflect.DeepEqual(names, []string{"alias", "change", "default", "non-standard"}) {
		t.Fatal(names)
	}
	s := New(nil, nil, 0)
	s.store = &catalog{packages: []funclib.Package{p}}
	if hints := s.Hints(t.Context(), "owner", "work", "change", nil); hints != "" {
		t.Fatal(hints)
	}
	if hints := s.Hints(t.Context(), "owner", "work", "change", []llm.ToolDefinition{{Name: "write"}}); !strings.Contains(hints, `"name":"change"`) {
		t.Fatal(hints)
	}
}
