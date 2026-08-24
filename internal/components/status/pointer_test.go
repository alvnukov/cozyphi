package status_test

import (
	"testing"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/status"
)

func TestExpandablePointerShape(t *testing.T) {
	inert := &status.Expandable{Expanded: false}
	if got := inert.PointerShape(0, 0); got != "" {
		t.Fatalf("non-expandable shape = %q", got)
	}

	collapsed := &status.Expandable{Expandable: true}
	if got := collapsed.PointerShape(0, 3); got != components.ShapePointer {
		t.Fatalf("collapsed body shape = %q", got)
	}

	open := &status.Expandable{Expandable: true, Expanded: true}
	if got := open.PointerShape(0, 0); got != components.ShapePointer {
		t.Fatalf("open title shape = %q", got)
	}
	if got := open.PointerShape(0, 3); got != components.ShapeText {
		t.Fatalf("open body shape = %q", got)
	}
}

func TestListTilePointerShape(t *testing.T) {
	tappable := &status.ListTile{OnTap: func() {}}
	if got := tappable.PointerShape(0, 0); got != components.ShapePointer {
		t.Fatalf("tappable shape = %q", got)
	}
	plain := &status.ListTile{}
	if got := plain.PointerShape(0, 0); got != "" {
		t.Fatalf("passive tile shape = %q", got)
	}
}
