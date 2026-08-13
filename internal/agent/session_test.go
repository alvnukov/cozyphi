package agent

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/llm"
)

func TestSessionPersistFlush(t *testing.T) {
	dir := t.TempDir()
	sess, err := NewSession(SessionOpts{
		Cwd:        dir,
		SessionDir: dir,
		Persist:    true,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, sess.ID())
	assert.NotEmpty(t, sess.File())

	require.NoError(t, sess.Append(llm.Message{Role: llm.RoleUser, Content: "hi"}))
	_, err = os.Stat(sess.File())
	assert.True(t, os.IsNotExist(err), "should not flush before first assistant")

	require.NoError(t, sess.Append(llm.Message{Role: llm.RoleAssistant, Content: "hello"}))
	require.FileExists(t, sess.File())

	b, err := os.ReadFile(sess.File())
	require.NoError(t, err)
	var header struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}
	first := splitFirstJSONL(b)
	require.NoError(t, json.Unmarshal(first, &header))
	assert.Equal(t, "EntrySession", header.Type)
	assert.Equal(t, sess.ID(), header.ID)
}

func TestSessionPersistFalseNoDisk(t *testing.T) {
	sess, err := NewSession(SessionOpts{Persist: false, Cwd: t.TempDir()})
	require.NoError(t, err)
	assert.Empty(t, sess.File())
	require.NoError(t, sess.Append(llm.Message{Role: llm.RoleUser, Content: "a"}))
	require.NoError(t, sess.Append(llm.Message{Role: llm.RoleAssistant, Content: "b"}))
	assert.Empty(t, sess.File())
}

func TestEngineSetModelKeepsSession(t *testing.T) {
	dir := t.TempDir()
	eng, err := NewEngine(EngineOpts{
		Model: llm.ModelConfig{Name: "model-a", APIKey: "k", BaseURL: "http://example"},
		SessionOpts: SessionOpts{
			Cwd:        dir,
			SessionDir: dir,
			Persist:    true,
		},
	})
	require.NoError(t, err)

	id := eng.SessionID()
	file := eng.SessionFile()
	require.NotEmpty(t, id)
	require.NotEmpty(t, file)

	require.NoError(t, eng.session.Append(llm.Message{Role: llm.RoleUser, Content: "keep me"}))
	require.NoError(t, eng.session.Append(llm.Message{Role: llm.RoleAssistant, Content: "ok"}))
	n := eng.session.Len()

	require.NoError(t, eng.SetModel(llm.ModelConfig{
		Name:          "model-b",
		APIKey:        "k",
		BaseURL:       "http://example",
		ContextWindow: 8192,
		SkillPath:     dir,
	}))
	assert.Equal(t, id, eng.SessionID())
	assert.Equal(t, file, eng.SessionFile())
	assert.Equal(t, n, eng.session.Len())
	assert.Equal(t, 8192, eng.contextWindow)
	assert.Equal(t, dir, eng.skillPath)
}

func splitFirstJSONL(b []byte) []byte {
	for i, c := range b {
		if c == '\n' {
			return b[:i]
		}
	}
	return b
}
