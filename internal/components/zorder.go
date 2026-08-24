package components

// Z layers of the editor shell: SubSurfaces appended to the editor root draw
// over lower layers. Only the root's sibling group shares these names —
// widgets keep their own local ladders for children inside their surfaces.
const (
	// ZList is the transcript list, the implicit bottom layer.
	ZList = 0
	// ZChat is the composer, or the bottom overlay replacing it.
	ZChat = 1
	// ZFooter is the status line.
	ZFooter = 2
	// ZPicker is the slash-command and @-mention pickers above the composer.
	ZPicker = 15
	// ZPalette is the Ctrl+K command palette above the pickers.
	ZPalette = 20
	// ZToast is the transient toast above everything else.
	ZToast = 40
)
