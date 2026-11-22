package tooldef

import "slices"

var libraryReads = []string{"list", "search", "versions", "get", "read", "diff", "test"}

// LibraryReadAction identifies operations that observe a package or run its
// tests in a fixture-only cell. Unknown operations receive the mutation policy.
func LibraryReadAction(action string) bool {
	return slices.Contains(libraryReads, action)
}

// LibraryActions returns a detached action catalog for a session.
func LibraryActions(writable bool) []string {
	actions := slices.Clone(libraryReads)
	if writable {
		actions = append(actions, "open", "create_file", "edit", "rebase", "publish", "pin")
	}
	return actions
}
