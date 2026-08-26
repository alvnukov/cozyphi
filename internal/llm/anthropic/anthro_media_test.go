package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
)

func TestBuildRequestMediaImage(t *testing.T) {
	cfg := llm.ModelConfig{Name: "claude-sonnet-4-20250514", APIKey: "k", BaseURL: "https://api.anthropic.com"}
	req := BuildRequest(cfg, "", []llm.Message{
		{Role: llm.RoleUser, Content: "look", Media: []llm.Media{{MediaType: "image/png", Data: "AAECAw=="}}},
	}, nil)

	body, err := json.Marshal(req)
	require.NoError(t, err)

	var raw struct {
		Messages []struct {
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(body, &raw))
	require.Equal(t, 1, len(raw.Messages))

	var blocks []struct {
		Type   string `json:"type"`
		Text   string `json:"text,omitempty"`
		Source *struct {
			Type      string `json:"type"`
			MediaType string `json:"media_type"`
			Data      string `json:"data"`
		} `json:"source,omitempty"`
	}
	require.NoError(t, json.Unmarshal(raw.Messages[0].Content, &blocks))
	require.Equal(t, 2, len(blocks))
	require.Equal(t, "text", blocks[0].Type)
	require.Equal(t, "look", blocks[0].Text)
	require.Equal(t, "image", blocks[1].Type)
	require.Equal(t, "base64", blocks[1].Source.Type)
	require.Equal(t, "image/png", blocks[1].Source.MediaType)
	require.Equal(t, "AAECAw==", blocks[1].Source.Data)
}
