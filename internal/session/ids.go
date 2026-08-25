package session

import (
	"crypto/rand"
	"encoding/hex"
)

// NewUserMessageID returns a unique transcript-row id for a submitted user
// message. The submitter hands it to both the UserAppend row and the
// controller so a queued prompt can be promoted out of the queued state when it
// dequeues.
func NewUserMessageID() string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		panic(err)
	}
	return hex.EncodeToString(bytes)
}
