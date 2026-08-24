package slot_test

import (
	"testing"

	"github.com/pulseaiclub/phi/internal/components/slot"
)

// TestArbitrate pins the editor's bottom-slot split with hand-worked rows
// from the shell's old inline arithmetic (overlay and composer branches),
// which this package now owns.
func TestArbitrate(t *testing.T) {
	tests := []struct {
		name      string
		totalH    int
		preferred int
		minH      int
		want      slot.Plan
	}{
		{"plain overlay split", 40, 12, 8, slot.Plan{ListHeight: 27, ChatHeight: 12}},
		{"composer split", 40, 24, 8, slot.Plan{ListHeight: 15, ChatHeight: 24}},
		{"ceiling caps a tall ask, list keeps its floor", 40, 44, 8, slot.Plan{ListHeight: 3, ChatHeight: 36}},
		{"floor raises a short ask", 40, 6, 8, slot.Plan{ListHeight: 31, ChatHeight: 8}},
		{"small floor does not inflate", 40, 4, 2, slot.Plan{ListHeight: 35, ChatHeight: 4}},
		{"short screen: widget floor wins over the list floor", 10, 2, 8, slot.Plan{ListHeight: 3, ChatHeight: 8}},
		{"tiny screen keeps the floor", 6, 5, 8, slot.Plan{ListHeight: 3, ChatHeight: 8}},
		{"degenerate screen still floors", 0, 4, 8, slot.Plan{ListHeight: 3, ChatHeight: 8}},
	}
	for _, tt := range tests {
		if got := slot.Arbitrate(tt.totalH, tt.preferred, tt.minH); got != tt.want {
			t.Errorf("%s: Arbitrate(%d, %d, %d) = %+v, want %+v",
				tt.name, tt.totalH, tt.preferred, tt.minH, got, tt.want)
		}
	}
}
