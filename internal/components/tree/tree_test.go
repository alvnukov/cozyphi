package tree_test

import (
	"testing"

	"github.com/pulseaiclub/phi/internal/components/tree"
)

func TestPrefixForSiblings(t *testing.T) {
	st := tree.DefaultStyle()
	if got := tree.PrefixForSiblings(3, 0, st); got != "├── " {
		t.Fatalf("first: %q", got)
	}
	if got := tree.PrefixForSiblings(3, 1, st); got != "├── " {
		t.Fatalf("mid: %q", got)
	}
	if got := tree.PrefixForSiblings(3, 2, st); got != "╰── " {
		t.Fatalf("last: %q", got)
	}
}

func TestFlattenNested(t *testing.T) {
	roots := []tree.Node[string]{
		{
			Item: "a",
			Children: []tree.Node[string]{
				{Item: "a1"},
				{Item: "a2"},
			},
		},
		{Item: "b"},
	}
	flat := tree.Flatten(roots)
	if len(flat) != 4 {
		t.Fatalf("len=%d", len(flat))
	}
	// a2 under a: ancestor a is not last among roots? a is first of 2, so ancestor last=false
	// Wait: a2's parent walk: when walking a's children, ancestors = [isLast of a among roots] = [false]
	p := tree.Prefix(flat[2], tree.DefaultStyle()) // a2, last child of a
	if p != "│   ╰── " {
		t.Fatalf("nested last under non-last parent: %q", p)
	}
	pLast := tree.Prefix(flat[3], tree.DefaultStyle()) // b
	if pLast != "╰── " {
		t.Fatalf("root last: %q", pLast)
	}
}
