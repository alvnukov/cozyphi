package slot_test

import (
	"testing"

	"github.com/alvnukov/cozyphi/internal/components/slot"
)

// TestArbitrate pins the editor's bottom-slot split with hand-worked rows
// from the shell's old inline arithmetic (overlay and composer branches),
// which this package now owns. The gap row between transcript and bottom
// widget is part of the split: it costs the widget's ceiling one row and
// collapses on screens too short for both floors.
func TestArbitrate(t *testing.T) {
	tests := []struct {
		name      string
		totalH    int
		preferred int
		minH      int
		want      slot.Plan
	}{
		{"plain overlay split", 40, 12, 8, slot.Plan{ListHeight: 26, ChatHeight: 12, ChatY: 27}},
		{"composer split", 40, 24, 8, slot.Plan{ListHeight: 14, ChatHeight: 24, ChatY: 15}},
		{
			"ceiling caps a tall ask, list keeps its floor",
			40,
			44,
			8,
			slot.Plan{ListHeight: 3, ChatHeight: 35, ChatY: 4},
		},
		{"floor raises a short ask", 40, 6, 8, slot.Plan{ListHeight: 30, ChatHeight: 8, ChatY: 31}},
		{"small floor does not inflate", 40, 4, 2, slot.Plan{ListHeight: 34, ChatHeight: 4, ChatY: 35}},
		{"short screen: widget floor wins, gap collapses", 10, 2, 8, slot.Plan{ListHeight: 3, ChatHeight: 8, ChatY: 3}},
		{"tiny screen keeps the floor", 6, 5, 8, slot.Plan{ListHeight: 3, ChatHeight: 8, ChatY: 3}},
		{"degenerate screen still floors", 0, 4, 8, slot.Plan{ListHeight: 3, ChatHeight: 8, ChatY: 3}},
	}
	for _, tt := range tests {
		if got := slot.Arbitrate(tt.totalH, tt.preferred, tt.minH); got != tt.want {
			t.Errorf("%s: Arbitrate(%d, %d, %d) = %+v, want %+v",
				tt.name, tt.totalH, tt.preferred, tt.minH, got, tt.want)
		}
	}
}
