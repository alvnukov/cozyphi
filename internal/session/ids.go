package session

import (
	"crypto/rand"
	"encoding/hex"
)

// NewEntryID returns a unique id for one session entry. Whoever draws the
// transcript row mints it first (the submitter for a typed prompt, the
// controller for a prompt queued behind a running turn, the streaming round
// for an assistant turn), and the same id travels on the message so the
// manager records the entry under it. One id regime keeps a live row and the
// entry a resumed session replays identical, which is what the rewind, fork
// and aside anchors address.
func NewEntryID() string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		panic(err)
	}
	return hex.EncodeToString(bytes)
}

// NewMutationID returns a fresh unique slug for a harness-authored lifecycle
// move (the auto-start behind a gateable tool call). The model names its own
// mutation ids when it asks for a transition explicitly; the harness mints a
// new one per application so a step reopened later is started for real
// instead of replaying an earlier recorded start.
func NewMutationID() string {
	bytes := make([]byte, 4)
	if _, err := rand.Read(bytes); err != nil {
		panic(err)
	}
	return "autostart-" + hex.EncodeToString(bytes)
}
