package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/project"
)

const configUIFixture = `models:
  - name: model-a
    api_key: key-a
    base_url: https://a.example/v1
    context_window: 1000
    max_output_tokens: 12000
    default: true
  - name: model-b
    api_key: key-b
    base_url: https://b.example/v1
permissions:
  mode: readonly
  dangerously_allow_all: true
  bash:
    default: ask
    allow:
      - "^git "
agents:
  enabled: false
`

func TestConfigHandlerGETAndRoundTrip(t *testing.T) {
	home := t.TempDir()
	cozyPhiDir := filepath.Join(home, ".cozyphi")
	require.NoError(t, os.MkdirAll(cozyPhiDir, 0o755))
	path := filepath.Join(cozyPhiDir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(configUIFixture), 0o644))

	h := &configHandler{configPath: path}

	// GET serves the current document.
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, newLocalAPIRequest(http.MethodGet, "/api/config", nil))
	require.Equal(t, http.StatusOK, rr.Code)
	var got configDoc
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	require.Len(t, got.Models, 2)
	assert.Equal(t, "model-a", got.Models[0].Name)
	assert.True(t, got.Models[0].Default)
	require.NotNil(t, got.Models[0].MaxOutputTokens)
	assert.Equal(t, 12000, *got.Models[0].MaxOutputTokens)
	require.NotNil(t, got.Permissions)
	require.NotNil(t, got.Permissions.Bash)
	assert.Equal(t, []string{"^git "}, got.Permissions.Bash.Allow)
	require.NotNil(t, got.Agents)
	assert.False(t, got.Agents.Enabled)
	assert.Equal(t, path, got.Path)

	// Edit: drop model-b and change the api_key, keep permissions untouched.
	got.Models = got.Models[:1]
	got.Models[0].APIKey = "new-key"
	body, err := json.Marshal(got)
	require.NoError(t, err)

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, newJSONAPIRequest("/api/config", strings.NewReader(string(body))))
	require.Equal(t, http.StatusOK, rr.Code)

	// The app must be able to load the written file with the same results.
	// os.UserHomeDir uses HOME on Unix and USERPROFILE on Windows.
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	p, err := project.Discover("")
	require.NoError(t, err)
	require.NoError(t, p.LoadConfig())
	cfg := p.Config()
	require.Len(t, cfg.Models, 1)
	assert.Equal(t, "model-a", cfg.Model().Name)
	assert.Equal(t, "new-key", cfg.Model().APIKey)
	assert.Equal(t, 12000, cfg.Model().MaxOutputTokens)
	assert.Equal(t, "readonly", string(cfg.Permissions.Mode))
	assert.True(t, cfg.Permissions.DangerouslyAllowAll)
	assert.Equal(t, []string{"^git "}, cfg.Permissions.BashAllow)
	assert.False(t, cfg.Agents.Enabled)

	// The previous file content is kept as a backup.
	bak, err := os.ReadFile(path + ".bak")
	require.NoError(t, err)
	assert.Contains(t, string(bak), "model-b")
}

func TestConfigHandlerMasksAPIKeysAndWritesOwnerOnly(t *testing.T) {
	home := t.TempDir()
	cozyPhiDir := filepath.Join(home, ".cozyphi")
	require.NoError(t, os.MkdirAll(cozyPhiDir, 0o755))
	path := filepath.Join(cozyPhiDir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(configUIFixture), 0o644))

	h := &configHandler{configPath: path}

	// GET masks stored keys and never leaks plaintext.
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, newLocalAPIRequest(http.MethodGet, "/api/config", nil))
	require.Equal(t, http.StatusOK, rr.Code)
	require.NotContains(t, rr.Body.String(), "key-a")
	require.NotContains(t, rr.Body.String(), "key-b")
	var got configDoc
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	require.Len(t, got.Models, 2)
	assert.Equal(t, maskedAPIKey, got.Models[0].APIKey)
	assert.Equal(t, maskedAPIKey, got.Models[1].APIKey)

	// Saving the masked document back preserves the stored keys.
	body, err := json.Marshal(got)
	require.NoError(t, err)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, newJSONAPIRequest("/api/config", strings.NewReader(string(body))))
	require.Equal(t, http.StatusOK, rr.Code)

	written, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(written), "api_key: key-a")
	assert.Contains(t, string(written), "api_key: key-b")

	// config.yaml and its backup are owner-only, even after a 0644 predecessor.
	// Windows cannot express 0600: Stat maps a writable file to 0666, so the
	// permission check is a POSIX-only assertion.
	for _, p := range []string{path, path + ".bak"} {
		info, err := os.Stat(p)
		require.NoError(t, err)
		if runtime.GOOS == "windows" {
			continue
		}
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), p)
	}
}

// The editor page does not know about plan.defaults (that is the TUI settings
// pane's domain); a save must preserve the section — and any other unknown
// key — instead of dropping it from the rewritten document.
func TestConfigHandlerSavePreservesUnmanagedSections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`models:
  - name: m
    api_key: k
custom: keep
plan:
  defaults:
    types:
      - name: inspect
        tools: [read]
`), 0o600))

	h := &configHandler{configPath: path}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, newJSONAPIRequest("/api/config", strings.NewReader(
		`{"models":[{"name":"m2","apiKey":"k2","default":true}]}`,
	)))
	require.Equal(t, http.StatusOK, rr.Code)

	written, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(written), "name: m2")
	assert.Contains(t, string(written), "custom: keep")
	assert.Contains(t, string(written), "plan:")
	assert.Contains(t, string(written), "inspect")

	// The preserved document is still loadable as a whole.
	doc, err := readConfigDoc(path)
	require.NoError(t, err)
	require.Len(t, doc.Models, 1)
	assert.Equal(t, "m2", doc.Models[0].Name)
}

// Renaming a model while its key is masked leaves a sentinel the stored doc
// cannot satisfy. The save must fail naming the model — a non-default model
// passes validation without an api_key, so letting the sentinel resolve to ""
// would silently drop the stored key.
func TestConfigHandlerRenamedModelWithMaskedKeyFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(configUIFixture), 0o600))

	h := &configHandler{configPath: path}

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, newLocalAPIRequest(http.MethodGet, "/api/config", nil))
	require.Equal(t, http.StatusOK, rr.Code)
	var got configDoc
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))

	got.Models[1].Name = "model-b-renamed" // non-default, api_key still masked
	body, err := json.Marshal(got)
	require.NoError(t, err)

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, newJSONAPIRequest("/api/config", strings.NewReader(string(body))))
	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "model-b-renamed")

	// The rejected save leaves the stored keys untouched.
	written, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(written), "api_key: key-a")
	assert.Contains(t, string(written), "api_key: key-b")
}

func TestRestoreMaskedAPIKeysRenamedModelFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(configUIFixture), 0o600))

	doc := &configDoc{Models: []modelDoc{
		{Name: "model-a", APIKey: maskedAPIKey}, // still stored: restored
		{Name: "model-b-renamed", APIKey: maskedAPIKey},
	}}
	err := restoreMaskedAPIKeys(path, doc)

	require.ErrorIs(t, err, errMaskedKeyUnrestored)
	assert.Contains(t, err.Error(), "model-b-renamed")
	assert.Equal(t, "key-a", doc.Models[0].APIKey, "restorable keys are still restored")
}

func TestConfigHandlerMissingFile(t *testing.T) {
	h := &configHandler{configPath: filepath.Join(t.TempDir(), "nope.yaml")}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, newLocalAPIRequest(http.MethodGet, "/api/config", nil))
	require.Equal(t, http.StatusOK, rr.Code)
	var doc configDoc
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &doc))
	assert.Empty(t, doc.Models)
}

func TestConfigHandlerValidation(t *testing.T) {
	h := &configHandler{configPath: filepath.Join(t.TempDir(), "config.yaml")}

	cases := []struct {
		name string
		doc  configDoc
	}{
		{"no models", configDoc{}},
		{"default missing api_key", configDoc{Models: []modelDoc{{Name: "m"}}}},
		{
			"two defaults",
			configDoc{
				Models: []modelDoc{{Name: "a", APIKey: "k", Default: true}, {Name: "b", APIKey: "k", Default: true}},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(tc.doc)
			require.NoError(t, err)
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, newJSONAPIRequest("/api/config", strings.NewReader(string(body))))
			require.Equal(t, http.StatusBadRequest, rr.Code)
		})
	}

	// A minimal valid document saves and marks the first model default.
	doc := configDoc{Models: []modelDoc{{Name: "m", APIKey: "k"}}}
	body, err := json.Marshal(doc)
	require.NoError(t, err)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, newJSONAPIRequest("/api/config", strings.NewReader(string(body))))
	require.Equal(t, http.StatusOK, rr.Code)

	data, err := os.ReadFile(h.configPath)
	require.NoError(t, err)
	require.Contains(t, string(data), "default: true")
	require.Contains(t, string(data), "name: m")
}

func TestConfigHandlerServesPage(t *testing.T) {
	h := &configHandler{configPath: filepath.Join(t.TempDir(), "config.yaml")}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody))
	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()
	assert.Contains(t, body, `id="langToggle"`)
	assert.Contains(t, body, "cozyphi-config-lang")
	assert.Contains(t, body, "配置中心")
	assert.Contains(t, body, "Config")
	require.Contains(t, body, `type: "password"`)
	assert.Contains(t, body, "tokens")
	assert.Contains(t, body, "seconds")
	assert.Contains(t, body, "/api/config")
	assert.Contains(t, body, "/api/models")
}

func TestConfigHandlerListsModels(t *testing.T) {
	cases := []struct {
		name       string
		model      string
		response   string
		wantPath   string
		wantModels []string
		checkAuth  func(*testing.T, *http.Request)
	}{
		{
			name:       "openai compatible",
			model:      "gpt-4o",
			response:   `{"data":[{"id":"z-model"},{"id":"a-model"},{"id":"z-model"}]}`,
			wantPath:   "/v1/models",
			wantModels: []string{"a-model", "z-model"},
			checkAuth: func(t *testing.T, r *http.Request) {
				assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
				assert.Empty(t, r.Header.Get("Anthropic-Version"))
			},
		},
		{
			name:       "anthropic",
			model:      "claude-sonnet-4-20250514",
			response:   `{"data":[{"id":"claude-sonnet-4-20250514","display_name":"Claude Sonnet"}]}`,
			wantPath:   "/v1/models",
			wantModels: []string{"claude-sonnet-4-20250514"},
			checkAuth: func(t *testing.T, r *http.Request) {
				assert.Equal(t, "test-key", r.Header.Get("X-Api-Key"))
				assert.Equal(t, "2023-06-01", r.Header.Get("Anthropic-Version"))
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, tc.wantPath, r.URL.Path)
				tc.checkAuth(t, r)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.response))
			}))
			defer server.Close()

			body, err := json.Marshal(modelListRequest{
				BaseURL: server.URL + "/v1",
				APIKey:  "test-key",
				Model:   tc.model,
			})
			require.NoError(t, err)

			h := &configHandler{configPath: filepath.Join(t.TempDir(), "config.yaml")}
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, newJSONAPIRequest("/api/models", strings.NewReader(string(body))))
			require.Equal(t, http.StatusOK, rr.Code)

			var got struct {
				Models []string `json:"models"`
			}
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
			assert.Equal(t, tc.wantModels, got.Models)
		})
	}
}

func TestConfigHandlerModelListRedirects(t *testing.T) {
	t.Run("same origin is followed", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/v1/models":
				http.Redirect(w, r, "/models-final", http.StatusTemporaryRedirect)
			case "/models-final":
				assert.Equal(t, "test-key", r.Header.Get("X-Api-Key"))
				_, _ = w.Write([]byte(`{"data":[{"id":"claude-model"}]}`))
			default:
				http.NotFound(w, r)
			}
		}))
		defer server.Close()

		rr := requestModelList(t, server.URL+"/v1", "claude-model")
		require.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), "claude-model")
	})

	t.Run("cross origin is rejected without forwarding key", func(t *testing.T) {
		var targetRequests atomic.Int32
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			targetRequests.Add(1)
			assert.Empty(t, r.Header.Get("X-Api-Key"))
			_, _ = w.Write([]byte(`{"data":[]}`))
		}))
		defer target.Close()

		source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target.URL+"/models", http.StatusTemporaryRedirect)
		}))
		defer source.Close()

		rr := requestModelList(t, source.URL+"/v1", "claude-model")
		require.Equal(t, http.StatusBadGateway, rr.Code)
		assert.Zero(t, targetRequests.Load())
	})
}

func TestConfigHandlerRejectsUnsafePOSTs(t *testing.T) {
	const localHost = "127.0.0.1:43210"
	const localOrigin = "http://127.0.0.1:43210"

	var targetRequests atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetRequests.Add(1)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer target.Close()

	configBody := `{"models":[{"name":"model","apiKey":"key","default":true}]}`
	modelBody := fmt.Sprintf(`{"baseUrl":%q,"apiKey":"key","model":"model"}`, target.URL)
	cases := []struct {
		name        string
		path        string
		body        string
		contentType string
		host        string
		origin      string
		wantStatus  int
	}{
		{
			"config rejects non-JSON",
			"/api/config",
			configBody,
			"text/plain",
			localHost,
			localOrigin,
			http.StatusUnsupportedMediaType,
		},
		{
			"config rejects missing origin",
			"/api/config",
			configBody,
			"application/json",
			localHost,
			"",
			http.StatusForbidden,
		},
		{
			"config rejects cross-origin",
			"/api/config",
			configBody,
			"application/json",
			localHost,
			"https://attacker.example",
			http.StatusForbidden,
		},
		{
			"config rejects non-loopback host",
			"/api/config",
			configBody,
			"application/json",
			"attacker.example",
			"http://attacker.example",
			http.StatusForbidden,
		},
		{
			"models rejects non-JSON",
			"/api/models",
			modelBody,
			"text/plain",
			localHost,
			localOrigin,
			http.StatusUnsupportedMediaType,
		},
		{
			"models rejects missing origin",
			"/api/models",
			modelBody,
			"application/json",
			localHost,
			"",
			http.StatusForbidden,
		},
		{
			"models rejects cross-origin",
			"/api/models",
			modelBody,
			"application/json",
			localHost,
			"https://attacker.example",
			http.StatusForbidden,
		},
		{
			"models rejects non-loopback host",
			"/api/models",
			modelBody,
			"application/json",
			"attacker.example",
			"http://attacker.example",
			http.StatusForbidden,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := targetRequests.Load()
			h := &configHandler{configPath: filepath.Join(t.TempDir(), "config.yaml")}
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, tc.path, strings.NewReader(tc.body))
			req.Host = tc.host
			req.Header.Set("Content-Type", tc.contentType)
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			rr := httptest.NewRecorder()

			h.ServeHTTP(rr, req)

			assert.Equal(t, tc.wantStatus, rr.Code)
			assert.Equal(t, before, targetRequests.Load())
			_, err := os.Stat(h.configPath)
			assert.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}

func TestConfigHandlerRejectsNonLoopbackGET(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`models:
  - name: secret-model
    api_key: secret-key
    default: true
`), 0o600))

	h := &configHandler{configPath: path}
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/config", http.NoBody)
	req.Host = "attacker.example"
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	require.Equal(t, http.StatusForbidden, rr.Code)
	assert.NotContains(t, rr.Body.String(), "secret-key")
}

func requestModelList(t *testing.T, baseURL, model string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(modelListRequest{
		BaseURL: baseURL,
		APIKey:  "test-key",
		Model:   model,
	})
	require.NoError(t, err)

	h := &configHandler{configPath: filepath.Join(t.TempDir(), "config.yaml")}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, newJSONAPIRequest("/api/models", strings.NewReader(string(body))))
	return rr
}

func newJSONAPIRequest(target string, body io.Reader) *http.Request {
	req := newLocalAPIRequest(http.MethodPost, target, body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://"+req.Host)
	return req
}

func newLocalAPIRequest(method, target string, body io.Reader) *http.Request {
	req := httptest.NewRequestWithContext(context.Background(), method, target, body)
	req.Host = "127.0.0.1:43210"
	return req
}
