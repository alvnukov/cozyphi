package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
)

func TestBuildRequestMediaImage(t *testing.T) {
	cfg := llm.ModelConfig{Name: "gpt-4o", APIKey: "k", BaseURL: "https://api.openai.com/v1"}
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

	var parts []struct {
		Type     string `json:"type"`
		Text     string `json:"text,omitempty"`
		ImageURL *struct {
			URL string `json:"url"`
		} `json:"image_url,omitempty"`
	}
	require.NoError(t, json.Unmarshal(raw.Messages[0].Content, &parts))
	require.Equal(t, 2, len(parts))
	require.Equal(t, "text", parts[0].Type)
	require.Equal(t, "look", parts[0].Text)
	require.Equal(t, "image_url", parts[1].Type)
	require.Equal(t, "data:image/png;base64,AAECAw==", parts[1].ImageURL.URL)
}
