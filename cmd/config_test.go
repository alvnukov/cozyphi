package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pulseaiclub/phi/internal/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const configUIFixture = `models:
  - name: model-a
    api_key: key-a
    base_url: https://a.example/v1
    context_window: 1000
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
`

func TestConfigHandlerGETAndRoundTrip(t *testing.T) {
	home := t.TempDir()
	phiDir := filepath.Join(home, ".phi")
	require.NoError(t, os.MkdirAll(phiDir, 0o755))
	path := filepath.Join(phiDir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(configUIFixture), 0o644))

	h := &configHandler{configPath: path}

	// GET serves the current document.
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	require.Equal(t, http.StatusOK, rr.Code)
	var got configDoc
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
	require.Len(t, got.Models, 2)
	assert.Equal(t, "model-a", got.Models[0].Name)
	assert.True(t, got.Models[0].Default)
	require.NotNil(t, got.Permissions)
	require.NotNil(t, got.Permissions.Bash)
	assert.Equal(t, []string{"^git "}, got.Permissions.Bash.Allow)
	assert.Equal(t, path, got.Path)

	// Edit: drop model-b and change the api_key, keep permissions untouched.
	got.Models = got.Models[:1]
	got.Models[0].APIKey = "new-key"
	body, err := json.Marshal(got)
	require.NoError(t, err)

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/config", strings.NewReader(string(body))))
	require.Equal(t, http.StatusOK, rr.Code)

	// The app must be able to load the written file with the same results.
	t.Setenv("HOME", home)
	p, err := project.Discover("")
	require.NoError(t, err)
	require.NoError(t, p.LoadConfig())
	cfg := p.Config()
	require.Len(t, cfg.Models, 1)
	assert.Equal(t, "model-a", cfg.Model().Name)
	assert.Equal(t, "new-key", cfg.Model().APIKey)
	assert.Equal(t, "readonly", string(cfg.Permissions.Mode))
	assert.True(t, cfg.Permissions.DangerouslyAllowAll)
	assert.Equal(t, []string{"^git "}, cfg.Permissions.BashAllow)

	// The previous file content is kept as a backup.
	bak, err := os.ReadFile(path + ".bak")
	require.NoError(t, err)
	assert.Contains(t, string(bak), "model-b")
}

func TestConfigHandlerMissingFile(t *testing.T) {
	h := &configHandler{configPath: filepath.Join(t.TempDir(), "nope.yaml")}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/config", nil))
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
		{"two defaults", configDoc{Models: []modelDoc{{Name: "a", APIKey: "k", Default: true}, {Name: "b", APIKey: "k", Default: true}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(tc.doc)
			require.NoError(t, err)
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/config", strings.NewReader(string(body))))
			require.Equal(t, http.StatusBadRequest, rr.Code)
		})
	}

	// A minimal valid document saves and marks the first model default.
	doc := configDoc{Models: []modelDoc{{Name: "m", APIKey: "k"}}}
	body, err := json.Marshal(doc)
	require.NoError(t, err)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/config", strings.NewReader(string(body))))
	require.Equal(t, http.StatusOK, rr.Code)

	data, err := os.ReadFile(h.configPath)
	require.NoError(t, err)
	require.Contains(t, string(data), "default: true")
	require.Contains(t, string(data), "name: m")
}

func TestConfigHandlerServesPage(t *testing.T) {
	h := &configHandler{configPath: filepath.Join(t.TempDir(), "config.yaml")}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "phi 配置")
	assert.Contains(t, rr.Body.String(), "/api/config")
}
