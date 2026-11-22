package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const mixedResult = `{
	"content":[{"type":"text","text":"parsing failed"},{"type":"image","mimeType":"image/png","data":"YWJjZA==","_meta":{"hidden":"secret"}}],
	"structuredContent":{"row":9007199254740993},"isError":true,"_meta":{"hidden":"secret"}
}`

func TestStructuredResultOverStdio(t *testing.T) {
	pool := NewPool(map[string]ServerConfig{"fake": {}})
	pool.clients["fake"] = newFakeStdioSession(t, "10s")
	t.Cleanup(func() { _ = pool.Close() })
	checkMixedResult(t, pool)
}

type inMemoryHTTP func(*http.Request) (*http.Response, error)

func (f inMemoryHTTP) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestStructuredResultOverHTTP(t *testing.T) {
	tr, err := newHTTPTransport("fake", ServerConfig{Transport: "http", URL: "https://example.invalid/mcp"})
	require.NoError(t, err)
	tr.client.Transport = inMemoryHTTP(func(r *http.Request) (*http.Response, error) {
		var req struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.NoError(t, r.Body.Close())
		var body string
		switch req.Method {
		case "initialize":
			body = fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":{"protocolVersion":"2025-06-18"}}`, req.ID)
		case "notifications/initialized":
		case "tools/call":
			assert.Equal(t, "2025-06-18", r.Header.Get("MCP-Protocol-Version"))
			body = fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"result":%s}`, req.ID, mixedResult)
		default:
			t.Fatalf("unexpected request %q", req.Method)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})
	pool := NewPool(map[string]ServerConfig{"fake": {}})
	pool.clients["fake"] = newSession("fake", tr)
	t.Cleanup(func() { _ = pool.Close() })
	checkMixedResult(t, pool)
}

func checkMixedResult(t *testing.T, pool *Pool) {
	t.Helper()
	result, err := pool.CallResult(t.Context(), "fake", "mixed", nil)
	require.NoError(t, err)
	assert.Contains(t, result.Text(), "error: parsing failed")
	require.JSONEq(t, `{"row":9007199254740993}`, string(result.StructuredContent))
	require.True(t, result.IsError)
	require.Len(t, result.Content, 2)
	ref := result.Content[1].Artifact
	require.NotNil(t, ref)
	require.NotEmpty(t, ref.ID)
	assert.Equal(t, 4, ref.Size)
	wire, err := json.Marshal(result)
	require.NoError(t, err)
	assert.NotContains(t, string(wire), "secret")
	assert.NotContains(t, string(wire), "YWJjZA==")
	page, err := pool.ReadArtifact("fake", "mixed", ref.ID, 0, 2)
	require.NoError(t, err)
	assert.Equal(t, "YWI=", page.Data)
	require.NotNil(t, page.Next)
	page, err = pool.ReadArtifact("fake", "mixed", ref.ID, *page.Next, 2)
	require.NoError(t, err)
	assert.Equal(t, "Y2Q=", page.Data)
	assert.Nil(t, page.Next)
	_, err = pool.ReadArtifact("fake", "other", ref.ID, 0, 0)
	require.Error(t, err)
	require.NoError(t, pool.SetEnabled("fake", false))
	_, err = pool.ReadArtifact("fake", "mixed", ref.ID, 0, 0)
	require.Error(t, err)
}

func TestMCPArtifactBoundsAndForgedReferences(t *testing.T) {
	pool := NewPool(map[string]ServerConfig{"fake": {}})
	first := pool.storeArtifact("fake", "mixed", "image/png", "YQ==")
	for range artifactStoreCount {
		pool.storeArtifact("fake", "mixed", "image/png", "Yg==")
	}
	_, err := pool.ReadArtifact("fake", "mixed", first.ID, 0, 0)
	require.ErrorContains(t, err, "expired")
	result, err := decodeCallResult(
		json.RawMessage(`{"content":[{"type":"image","data":"broken base64","artifact":{"id":"invented"}}]}`),
	)
	require.NoError(t, err)
	pool.retainArtifacts("fake", "mixed", &result)
	require.NotNil(t, result.Content[0].Artifact)
	assert.Empty(t, result.Content[0].Artifact.ID)
	assert.NotEmpty(t, result.Content[0].Artifact.Error)
}

func TestOversizedProgramResult(t *testing.T) {
	pool := NewPool(map[string]ServerConfig{"fixture": {}})
	result := CallResult{Content: []ContentBlock{{Type: "text", Text: strings.Repeat("隣", inlineResultBytes/3+1)}}}
	value, err := pool.ProgramResult("fixture", "rows", result)
	require.NoError(t, err)
	envelope, err := json.Marshal(value)
	require.NoError(t, err)
	var handle struct{ ResultArtifact Artifact }
	require.NoError(t, json.Unmarshal(envelope, &handle))
	require.NotEmpty(t, handle.ResultArtifact.ID)
	var body strings.Builder
	offset := 0
	for {
		page, err := pool.ReadArtifact("fixture", "rows", handle.ResultArtifact.ID, offset, artifactPageBytes)
		require.NoError(t, err)
		body.WriteString(page.Text)
		if page.Next == nil {
			break
		}
		offset = *page.Next
	}
	expected, err := json.Marshal(result)
	require.NoError(t, err)
	require.Equal(t, string(expected), body.String())
}
