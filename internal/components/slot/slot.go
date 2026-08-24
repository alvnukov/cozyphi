// Package slot arbitrates the editor shell's bottom-slot row split: how many
// rows the transcript list keeps and how many the bottom widget — the
// composer, or the overlay replacing it — gets on a screen of a given height.
//
// The split is pure arithmetic on ints so every caller (composer, permission
// overlay, tests) agrees on one set of floors and ceilings.
package slot

// FooterRows is the status line the split always reserves at the bottom.
const FooterRows = 1

// ListFloor is the fewest rows the transcript keeps on any screen.
const ListFloor = 3

// Plan is the row split of the editor screen.
type Plan struct {
	ListHeight int // rows granted to the transcript list
	ChatHeight int // rows granted to the composer or the overlay replacing it
}

// Arbitrate splits totalH rows between the transcript and a bottom widget
// that wants preferred rows but never fewer than minH. The widget's ceiling
// is totalH - FooterRows - ListFloor; on screens too short for both floors
// the widget keeps minH and may draw past the status line, which layers
// above it.
func Arbitrate(totalH, preferred, minH int) Plan {
	ceiling := max(totalH-FooterRows-ListFloor, minH)
	chatH := min(max(preferred, minH), ceiling)
	listH := totalH - chatH - FooterRows
	if listH < ListFloor {
		listH = ListFloor
		chatH = max(totalH-listH-FooterRows, minH)
	}
	return Plan{ListHeight: listH, ChatHeight: chatH}
}
