package render

// Exported control sequences for engine / capability setup.
const (
	CSI = "\x1b["
	OSC = "\x1b]"
	ST  = "\x1b\\"

	SeqSGRReset    = CSI + "m"
	SeqHideCursor  = CSI + "?25l"
	SeqShowCursor  = CSI + "?25h"
	SeqClearScreen = CSI + "2J"
	SeqHome        = CSI + "H"

	SeqSyncSet   = CSI + "?2026h"
	SeqSyncReset = CSI + "?2026l"

	SeqAltEnter = CSI + "?1049h"
	SeqAltExit  = CSI + "?1049l"

	SeqMouseSet   = CSI + "?1002;1003;1004;1006h"
	SeqMouseReset = CSI + "?1002;1003;1004;1006;1016l"

	SeqBracketedPasteSet   = CSI + "?2004h"
	SeqBracketedPasteReset = CSI + "?2004l"

	SeqUnicodeSet   = CSI + "?2027h"
	SeqUnicodeReset = CSI + "?2027l"

	SeqInBandResizeSet   = CSI + "?2048h"
	SeqInBandResizeReset = CSI + "?2048l"

	SeqKittyKBPop = CSI + "<u"
	// default with reportEventTypes: disambiguate|events|alternates = 7.
	SeqKittyKBPush = CSI + ">7u"

	// xterm modifyOtherKeys mode 2 — fallback when Kitty keyboard is unavailable (tmux).
	SeqModifyOtherKeysSet   = CSI + ">4;2m"
	SeqModifyOtherKeysReset = CSI + ">4;0m"

	SeqPrimaryDA     = CSI + "c"
	SeqXTVersion     = CSI + ">0q"
	SeqKittyKBQuery  = CSI + "?u"
	SeqDECRQMSync    = CSI + "?2026$p"
	SeqDECRQMUnicode = CSI + "?2027$p"

	SeqFGReset = CSI + "39m"
	SeqBGReset = CSI + "49m"

	// OSC 52 clipboard write: \x1b]52;c;<base64>\x1b\
	SeqOSC52ClipboardPrefix = OSC + "52;c;"
	SeqOSC52ClipboardSuffix = ST
)

// Internal aliases used by render.go
const (
	csi = CSI
	osc = OSC
	st  = ST

	seqSGRReset    = SeqSGRReset
	seqHideCursor  = SeqHideCursor
	seqShowCursor  = SeqShowCursor
	seqClearScreen = SeqClearScreen
	seqHome        = SeqHome
	seqSyncSet     = SeqSyncSet
	seqSyncReset   = SeqSyncReset
	seqAltEnter    = SeqAltEnter
	seqAltExit     = SeqAltExit
	seqFGReset     = SeqFGReset
	seqBGReset     = SeqBGReset
)
