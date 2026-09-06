package project

import "github.com/alvnukov/cozyphi/internal/diag"

// StoreAnchors reports where this workspace's layout puts each store, for
// the harness view to compose locators from.
//
// It answers from the layout alone: nothing here creates a directory, stats
// one, lists one or opens a file, so asking where a store would be does not
// bring it into existence. Only the parts the layout fixes travel — a
// directory named after the working directory or after the checkout encodes
// a path into its own name, and the view fills those in with a placeholder
// rather than spelling them out.
func (p *Project) StoreAnchors() diag.StorageAnchors {
	if p == nil {
		return diag.StorageAnchors{}
	}
	return diag.StorageAnchors{
		Known:       true,
		SessionBase: p.global.SessionBase(),
		MemoryBase:  p.global.claudeProjectsDir(),
		UsageDir:    p.global.Root(),
		UsageFile:   p.global.UsageFile(),
		// Inside Git the corpus is keyed by the checkout, so every worktree
		// made from it reads and writes one set of memories; outside Git
		// there is no checkout and the working directory keys its own.
		CorpusShared:  p.memoryRoot != "",
		CorpusForeign: p.corpusForeign,
	}
}
