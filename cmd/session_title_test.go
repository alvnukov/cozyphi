package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
)

func TestSessionListLineTitleAndIdentity(t *testing.T) {
	meta := session.SessionMeta{ID: "durable-session-id", Title: "fix build", TitleSource: "user", Active: true}
	line := sessionListLine(meta)
	require.Contains(t, line, "durable-session-id [active]")
	require.Contains(t, line, "fix build")
	meta.Title = ""
	meta.FirstPrompt = "first prompt fallback"
	require.Contains(t, sessionListLine(meta), "first prompt fallback")
}
