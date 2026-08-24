// Package slot arbitrates the editor shell's bottom-slot row split: how many
// rows the transcript list keeps and how many the bottom widget — the
// composer, or the overlay replacing it — gets on a screen of a given height.
//
// The split is pure arithmetic on ints so every caller (composer, permission
// overlay, tests) agrees on one set of floors, gaps and ceilings.
package slot

// FooterRows is the status line the split always reserves at the bottom.
const FooterRows = 1

// GapRows is the blank row between the transcript and the bottom widget —
// opencode's paddingBottom on the message container. It is the first thing
// sacrificed when the screen cannot fit both floors.
const GapRows = 1

// ListFloor is the fewest rows the transcript keeps on any screen.
const ListFloor = 3

// Plan is the row split of the editor screen.
type Plan struct {
	ListHeight int // rows granted to the transcript list
	ChatHeight int // rows granted to the composer or the overlay replacing it
	// ChatY is the screen row where the bottom widget starts; the rows
	// between ListHeight and ChatY are the breathing gap when afforded.
	ChatY int
}

// Arbitrate splits totalH rows between the transcript and a bottom widget
// that wants preferred rows but never fewer than minH. The widget's ceiling
// is totalH - FooterRows - GapRows - ListFloor; on screens too short for both
// floors the gap collapses first, the widget keeps minH and may draw past the
// status line, which layers above it.
func Arbitrate(totalH, preferred, minH int) Plan {
	ceiling := max(totalH-FooterRows-GapRows-ListFloor, minH)
	chatH := min(max(preferred, minH), ceiling)
	listH := totalH - chatH - FooterRows - GapRows
	chatY := listH + GapRows
	if listH < ListFloor {
		listH = ListFloor
		chatH = max(totalH-listH-FooterRows, minH)
		chatY = listH
	}
	return Plan{ListHeight: listH, ChatHeight: chatH, ChatY: chatY}
}
