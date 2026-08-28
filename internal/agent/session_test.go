package agent

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
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

func TestSessionResumeClosesInterruptedTrailingToolRound(t *testing.T) {
	dir := t.TempDir()
	sess, err := NewSession(SessionOpts{Cwd: dir, SessionDir: dir, Persist: true})
	require.NoError(t, err)
	require.NoError(t, sess.Append(
		llm.Message{Role: llm.RoleUser, Content: "run tools"},
		llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			{ID: "call_1"},
			{ID: "call_2"},
		}},
	))

	resumed, err := NewSession(SessionOpts{ResumePath: sess.File()})
	require.NoError(t, err)
	raw := resumed.buildRawContext()
	require.Len(t, raw, 4)
	require.Equal(t, llm.RoleTool, raw[2].Role)
	require.Equal(t, "call_1", raw[2].ToolCallID)
	require.Equal(t, llm.RoleTool, raw[3].Role)
	require.Equal(t, "call_2", raw[3].ToolCallID)

	reopened, err := NewSession(SessionOpts{ResumePath: sess.File()})
	require.NoError(t, err)
	require.Len(t, reopened.buildRawContext(), 4, "repair results must be durable and idempotent")
}

func TestSessionBuildContextRepairsOlderMalformedToolRound(t *testing.T) {
	sess, err := NewSession(SessionOpts{Cwd: t.TempDir()})
	require.NoError(t, err)
	require.NoError(t, sess.Append(
		llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "call_1"}}},
		llm.Message{Role: llm.RoleUser, Content: "retry after provider error"},
	))

	context := sess.BuildContext()
	require.Len(t, context, 3)
	require.Equal(t, llm.RoleAssistant, context[0].Role)
	require.Equal(t, llm.RoleTool, context[1].Role)
	require.Equal(t, "call_1", context[1].ToolCallID)
	require.Equal(t, llm.RoleUser, context[2].Role)
	require.Len(t, sess.buildRawContext(), 2, "repair must not rewrite the append-only audit trail")
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
	n := eng.session.manager.Len()

	require.NoError(t, eng.SetModel(llm.ModelConfig{
		Name:          "model-b",
		APIKey:        "k",
		BaseURL:       "http://example",
		ContextWindow: 8192,
		SkillPath:     dir,
	}))
	assert.Equal(t, id, eng.SessionID())
	assert.Equal(t, file, eng.SessionFile())
	assert.Equal(t, n, eng.session.manager.Len())
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
