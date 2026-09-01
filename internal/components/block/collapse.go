package block

import "strings"

// A plain click — press and release with no drag-selection in between —
// folds an expanded block, wherever it lands. Only the transcript pane can
// tell such a click from the start of a selection, so the pane calls
// CollapseOnClick on a clean release; the press-on-title toggle in each
// block's Handle still serves collapsed blocks, whose only surface is the
// title row (that press is consumed, so no clean release follows it).

// foldExpanded collapses when the block is expanded and has something to
// fold, notifies onToggle, and reports whether it collapsed.
func foldExpanded(expanded *bool, foldable bool, onToggle func(bool)) bool {
	if !foldable || !*expanded {
		return false
	}
	*expanded = false
	if onToggle != nil {
		onToggle(false)
	}
	return true
}

// CollapseOnClick folds the expanded block and reports whether it did.
func (bashBlock *BashBlock) CollapseOnClick() bool {
	return foldExpanded(&bashBlock.Expanded, bashBlock.hasBody(), bashBlock.OnToggle)
}

// CollapseOnClick folds the expanded block and reports whether it did.
func (toolBlock *ToolBlock) CollapseOnClick() bool {
	return foldExpanded(&toolBlock.Expanded, toolBlock.hasBody(), toolBlock.OnToggle)
}

// CollapseOnClick folds the expanded block and reports whether it did.
func (diffBlock *DiffBlock) CollapseOnClick() bool {
	return foldExpanded(&diffBlock.Expanded, diffBlock.hasBody(), diffBlock.OnToggle)
}

// CollapseOnClick folds the expanded block and reports whether it did.
func (a *AgentBlock) CollapseOnClick() bool {
	return foldExpanded(&a.Expanded, a.hasBody(), a.OnToggle)
}

// CollapseOnClick folds the expanded block and reports whether it did.
func (t *ThinkingBlock) CollapseOnClick() bool {
	return foldExpanded(&t.Expanded, true, t.OnToggle)
}

// CollapseOnClick folds the expanded block and reports whether it did.
func (b *CompactionBlock) CollapseOnClick() bool {
	return foldExpanded(&b.Expanded, strings.TrimSpace(b.Summary) != "", b.OnToggle)
}
