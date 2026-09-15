package session

import (
	"crypto/rand"
	"encoding/hex"
)

// NewUserMessageID returns a unique transcript-row id for a submitted user
// message. The submitter stamps its UserAppend row with it; for a prompt
// queued behind a running turn the controller assigns one at enqueue, and
// the engine's UserPromoted carries it when the prompt is delivered.
func NewUserMessageID() string {
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
