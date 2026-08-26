package responses

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
)

func TestBuildRequestMediaImage(t *testing.T) {
	cfg := llm.ModelConfig{Name: "gpt-4o", APIKey: "k", BaseURL: "https://api.openai.com/v1"}
	req, err := buildRequest(cfg, []llm.Message{
		{Role: llm.RoleUser, Content: "look", Media: []llm.Media{{MediaType: "image/png", Data: "AAECAw=="}}},
	}, nil, "")
	require.NoError(t, err)

	body, err := json.Marshal(req)
	require.NoError(t, err)

	var raw struct {
		Input []json.RawMessage `json:"input"`
	}
	require.NoError(t, json.Unmarshal(body, &raw))
	require.Equal(t, 1, len(raw.Input))

	var part struct {
		Type    string          `json:"type"`
		Content json.RawMessage `json:"content"`
	}
	require.NoError(t, json.Unmarshal(raw.Input[0], &part))
	require.Equal(t, "message", part.Type)

	var content []struct {
		Type     string `json:"type"`
		Text     string `json:"text,omitempty"`
		ImageURL string `json:"image_url,omitempty"`
	}
	require.NoError(t, json.Unmarshal(part.Content, &content))
	require.Equal(t, 2, len(content))
	require.Equal(t, "input_text", content[0].Type)
	require.Equal(t, "look", content[0].Text)
	require.Equal(t, "input_image", content[1].Type)
	require.Equal(t, "data:image/png;base64,AAECAw==", content[1].ImageURL)
}
