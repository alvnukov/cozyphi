// Package provider owns model catalog metadata and provider credentials.
//
// Remote catalog data is untrusted. A connection pins the endpoint and protocol
// that the user approved, so later catalog refreshes can update model metadata
// without redirecting a stored credential.
package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"

	"github.com/pulseaiclub/phi/internal/llm"
	"github.com/pulseaiclub/phi/internal/util"
)

const (
	defaultCatalogURL = "https://models.dev/api.json"
	maxCatalogBytes   = 16 << 20
	maxProviders      = 512
	maxModels         = 4096
	maxStringBytes    = 4096
	maxAPIKeyBytes    = 64 << 10
)

// Options configures catalog and credential persistence.
type Options struct {
	CatalogURL      string
	CachePath       string
	CredentialsPath string
	HTTPClient      *http.Client
}

// AuthKind identifies the user-visible authentication flow.
type AuthKind string

const (
	AuthAPIKey       AuthKind = "api-key"
	AuthOAuthBrowser AuthKind = "oauth-browser"
	AuthOAuthDevice  AuthKind = "oauth-device"
)

// Info is safe catalog metadata suitable for display.
type Info struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	BaseURL  string       `json:"base_url"`
	Protocol llm.Protocol `json:"protocol"`
	Auth     AuthKind     `json:"auth,omitempty"`
	Models   []Model      `json:"models"`
}

// Model is safe model metadata from the catalog.
type Model struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	ContextWindow   int    `json:"context_window"`
	MaxOutputTokens int    `json:"max_output_tokens"`
}

var codexReasoningEfforts = []llm.ReasoningEffort{
	llm.ReasoningEffortMinimal,
	llm.ReasoningEffortLow,
	llm.ReasoningEffortMedium,
	llm.ReasoningEffortHigh,
}

// ConnectRequest pins the exact endpoint shown to the user.
type ConnectRequest struct {
	ProviderID      string
	ExpectedBaseURL string
	APIKey          string
}

// Manager is the deep provider module used by the TUI and controller.
type Manager struct {
	mu          sync.RWMutex
	authMu      sync.Mutex
	catalogURL  string
	cachePath   string
	credsPath   string
	httpClient  *http.Client
	oauthIssuer string
	providers   map[string]Info
	credentials map[string]credential
}

func builtinProviders() map[string]Info {
	return map[string]Info{
		"codex": {
			ID: "codex", Name: "OpenAI Codex (ChatGPT subscription)",
			BaseURL:  "https://chatgpt.com/backend-api/codex",
			Protocol: llm.ProtocolOpenAIResponses, Auth: AuthOAuthBrowser,
			Models: []Model{
				{ID: "gpt-5.5", Name: "GPT-5.5"},
				{ID: "gpt-5.4", Name: "GPT-5.4"},
				{ID: "gpt-5.4-mini", Name: "GPT-5.4 mini"},
				{ID: "gpt-5.3-codex-spark", Name: "GPT-5.3 Codex Spark"},
			},
		},
		"zai-coding-plan": {
			ID: "zai-coding-plan", Name: "Z.AI Coding Plan",
			BaseURL:  "https://api.z.ai/api/coding/paas/v4",
			Protocol: llm.ProtocolOpenAI, Auth: AuthAPIKey,
			Models: []Model{
				{ID: "glm-4.5-air", Name: "GLM-4.5-Air", ContextWindow: 131072, MaxOutputTokens: 98304},
				{ID: "glm-4.7", Name: "GLM-4.7", ContextWindow: 204800, MaxOutputTokens: 131072},
				{ID: "glm-5-turbo", Name: "GLM-5-Turbo", ContextWindow: 200000, MaxOutputTokens: 131072},
				{ID: "glm-5.1", Name: "GLM-5.1", ContextWindow: 200000, MaxOutputTokens: 131072},
				{ID: "glm-5.2", Name: "GLM-5.2", ContextWindow: 1000000, MaxOutputTokens: 131072},
				{ID: "glm-5v-turbo", Name: "GLM-5V-Turbo", ContextWindow: 200000, MaxOutputTokens: 131072},
			},
		},
	}
}

func mergeBuiltins(providers map[string]Info) map[string]Info {
	if providers == nil {
		providers = make(map[string]Info)
	}
	for id, builtin := range builtinProviders() {
		// Authentication, endpoint, and protocol are trusted connection
		// contracts. A catalog refresh may update model metadata, but must not
		// redirect credentials or change the wire protocol behind the UI.
		if catalog, ok := providers[id]; ok && len(catalog.Models) > 0 {
			builtin.Models = append([]Model(nil), catalog.Models...)
		}
		providers[id] = builtin
	}
	return providers
}

// Open loads the validated last-known-good catalog and owner-only credentials.
func Open(opts Options) (*Manager, error) {
	if strings.TrimSpace(opts.CachePath) == "" {
		return nil, errors.New("provider: catalog cache path is required")
	}
	if strings.TrimSpace(opts.CredentialsPath) == "" {
		return nil, errors.New("provider: credentials path is required")
	}
	catalogURL := strings.TrimSpace(opts.CatalogURL)
	if catalogURL == "" {
		catalogURL = defaultCatalogURL
	}
	client := opts.HTTPClient
	if client == nil {
		client = util.DefaultHTTPClient()
	}

	providers, err := readCatalogCache(opts.CachePath)
	if err != nil {
		return nil, fmt.Errorf("provider: load catalog cache: %w", err)
	}
	providers = mergeBuiltins(providers)
	creds, err := readCredentials(opts.CredentialsPath)
	if err != nil {
		return nil, fmt.Errorf("provider: load credentials: %w", err)
	}
	return &Manager{
		catalogURL:  catalogURL,
		cachePath:   opts.CachePath,
		credsPath:   opts.CredentialsPath,
		httpClient:  client,
		oauthIssuer: defaultOAuthIssuer,
		providers:   providers,
		credentials: creds,
	}, nil
}

// Providers returns a detached, stable catalog snapshot.
func (m *Manager) Providers() []Info {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	items := make([]Info, 0, len(m.providers))
	for _, item := range m.providers {
		items = append(items, cloneInfo(item))
	}
	slices.SortFunc(items, func(a, b Info) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return items
}

// Refresh fetches, validates, and atomically installs a new catalog snapshot.
// A failure leaves both memory and the last-known-good cache unchanged.
func (m *Manager) Refresh(ctx context.Context) error {
	if m == nil {
		return errors.New("provider: manager is nil")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.catalogURL, http.NoBody)
	if err != nil {
		return fmt.Errorf("provider: catalog request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	client := *m.httpClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errors.New("catalog redirects are not allowed")
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("provider: refresh catalog: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("provider: refresh catalog: unexpected HTTP status %d", resp.StatusCode)
	}
	return m.ReplaceCatalog(resp.Body)
}

// ReplaceCatalog validates a models.dev document and installs it atomically.
// It is also useful for explicit offline catalog imports.
func (m *Manager) ReplaceCatalog(r io.Reader) error {
	if m == nil {
		return errors.New("provider: manager is nil")
	}
	next, err := decodeRemoteCatalog(r)
	if err != nil {
		return fmt.Errorf("provider: catalog rejected: %w", err)
	}
	next = mergeBuiltins(next)

	m.mu.Lock()
	defer m.mu.Unlock()
	for id := range m.credentials {
		if _, exists := next[id]; exists {
			continue
		}
		if previous, ok := m.providers[id]; ok {
			next[id] = previous
		}
	}
	if err := writeCatalogCache(m.cachePath, next); err != nil {
		return fmt.Errorf("provider: save catalog cache: %w", err)
	}
	m.providers = next
	return nil
}

// Connect stores an API key only after matching the endpoint currently shown
// to the user. The key is never included in returned errors.
func (m *Manager) Connect(req ConnectRequest) error {
	if m == nil {
		return errors.New("provider: manager is nil")
	}
	id := strings.TrimSpace(req.ProviderID)
	expected := normalizeURL(req.ExpectedBaseURL)
	key := strings.TrimSpace(req.APIKey)
	if id == "" {
		return errors.New("provider: provider id is required")
	}
	if key == "" {
		return errors.New("provider: API key is required")
	}
	if len(key) > maxAPIKeyBytes {
		return errors.New("provider: API key is too large")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.providers[id]
	if !ok {
		return fmt.Errorf("provider: provider %q is not in the validated catalog", id)
	}
	if item.Auth == AuthOAuthBrowser || item.Auth == AuthOAuthDevice {
		return fmt.Errorf("provider: %q requires subscription sign-in, not an API key", id)
	}
	if expected == "" || expected != item.BaseURL {
		return fmt.Errorf("provider: endpoint changed for %q; reopen /connect and review it", id)
	}
	next := cloneCredentials(m.credentials)
	next[id] = credential{
		Type:     "api",
		Key:      key,
		BaseURL:  item.BaseURL,
		Protocol: item.Protocol,
	}
	if err := writeCredentials(m.credsPath, next); err != nil {
		return fmt.Errorf("provider: save credential for %q: %w", id, err)
	}
	m.credentials = next
	return nil
}

// Models returns connection-ready model configurations for connected providers.
func (m *Manager) Models() []llm.ModelConfig {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []llm.ModelConfig
	for id, cred := range m.credentials {
		item, ok := m.providers[id]
		if !ok {
			continue
		}
		var authenticator llm.RequestAuthenticator
		switch cred.Type {
		case "api":
		case "oauth":
			authenticator = &oauthAuthenticator{manager: m, providerID: id}
		default:
			continue
		}
		for _, model := range item.Models {
			base := llm.ModelConfig{
				Name:            id + "/" + model.ID,
				APIName:         model.ID,
				ProviderID:      id,
				Protocol:        cred.Protocol,
				APIKey:          cred.Key,
				BaseURL:         cred.BaseURL,
				Authenticator:   authenticator,
				ContextWindow:   model.ContextWindow,
				MaxOutputTokens: model.MaxOutputTokens,
			}
			result = append(result, base)
			if id != "codex" || cred.Protocol != llm.ProtocolOpenAIResponses {
				continue
			}
			for _, effort := range codexReasoningEfforts {
				cfg := base
				cfg.Name = base.Name + ":" + string(effort)
				cfg.ReasoningEffort = effort
				result = append(result, cfg)
			}
		}
	}
	slices.SortFunc(result, func(a, b llm.ModelConfig) int { return strings.Compare(a.Name, b.Name) })
	return result
}

type remoteProvider struct {
	ID     string                 `json:"id"`
	Name   string                 `json:"name"`
	API    string                 `json:"api"`
	NPM    string                 `json:"npm"`
	Models map[string]remoteModel `json:"models"`
}

type remoteModel struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Limit struct {
		Context int `json:"context"`
		Output  int `json:"output"`
	} `json:"limit"`
}

func decodeRemoteCatalog(r io.Reader) (map[string]Info, error) {
	if r == nil {
		return nil, errors.New("empty input")
	}
	limited := io.LimitReader(r, maxCatalogBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(data) > maxCatalogBytes {
		return nil, fmt.Errorf("document exceeds %d bytes", maxCatalogBytes)
	}
	var raw map[string]remoteProvider
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if len(raw) == 0 || len(raw) > maxProviders {
		return nil, fmt.Errorf("provider count %d is outside 1..%d", len(raw), maxProviders)
	}

	result := make(map[string]Info)
	for key, item := range raw {
		protocol, supported := protocolForNPM(item.NPM)
		if !supported {
			continue
		}
		info, err := decodeRemoteProvider(key, item, protocol)
		if err != nil {
			// Catalog entries are independent trust boundaries. Reject the
			// unsafe entry without letting it suppress every valid provider.
			continue
		}
		result[info.ID] = info
	}
	if len(result) == 0 {
		return nil, errors.New("catalog contains no supported providers")
	}
	return result, nil
}

func decodeRemoteProvider(key string, item remoteProvider, protocol llm.Protocol) (Info, error) {
	id := strings.TrimSpace(item.ID)
	if id == "" {
		id = strings.TrimSpace(key)
	}
	if !validID(id) || id != key {
		return Info{}, fmt.Errorf("invalid provider id %q", id)
	}
	name := strings.TrimSpace(item.Name)
	if name == "" || len(name) > maxStringBytes {
		return Info{}, fmt.Errorf("invalid provider name for %q", id)
	}
	baseURL, err := trustedCatalogURL(id, item.API)
	if err != nil {
		return Info{}, fmt.Errorf("provider %q: %w", id, err)
	}
	if len(item.Models) == 0 || len(item.Models) > maxModels {
		return Info{}, fmt.Errorf("provider %q has invalid model count", id)
	}
	models := make([]Model, 0, len(item.Models))
	for modelKey, rawModel := range item.Models {
		modelID := strings.TrimSpace(rawModel.ID)
		if modelID == "" {
			modelID = strings.TrimSpace(modelKey)
		}
		if modelID == "" || modelID != modelKey || len(modelID) > maxStringBytes {
			return Info{}, fmt.Errorf("provider %q has invalid model id", id)
		}
		modelName := strings.TrimSpace(rawModel.Name)
		if modelName == "" {
			modelName = modelID
		}
		if len(modelName) > maxStringBytes || rawModel.Limit.Context < 0 || rawModel.Limit.Output < 0 {
			return Info{}, fmt.Errorf("provider %q model %q has invalid metadata", id, modelID)
		}
		models = append(models, Model{
			ID:              modelID,
			Name:            modelName,
			ContextWindow:   rawModel.Limit.Context,
			MaxOutputTokens: rawModel.Limit.Output,
		})
	}
	slices.SortFunc(models, func(a, b Model) int { return strings.Compare(a.ID, b.ID) })
	return Info{
		ID: id, Name: name, BaseURL: baseURL, Protocol: protocol,
		Auth: AuthAPIKey, Models: models,
	}, nil
}

func protocolForNPM(npm string) (llm.Protocol, bool) {
	switch strings.TrimSpace(npm) {
	case "@ai-sdk/openai", "@ai-sdk/openai-compatible":
		return llm.ProtocolOpenAI, true
	case "@ai-sdk/anthropic":
		return llm.ProtocolAnthropic, true
	default:
		return "", false
	}
}

func trustedCatalogURL(id, value string) (string, error) {
	switch id {
	case "openai":
		return "https://api.openai.com/v1", nil
	case "anthropic":
		return "https://api.anthropic.com", nil
	}
	normalized := normalizeURL(value)
	parsed, err := url.Parse(normalized)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("catalog endpoint must be an HTTPS origin without credentials, query, or fragment")
	}
	host := parsed.Hostname()
	if host == "" || strings.EqualFold(host, "localhost") {
		return "", errors.New("catalog endpoint host is not public")
	}
	if ip := net.ParseIP(
		host,
	); ip != nil &&
		(ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()) {
		return "", errors.New("catalog endpoint host is not public")
	}
	return normalized, nil
}

func validID(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func normalizeURL(value string) string {
	return strings.TrimRight(strings.TrimSpace(value), "/")
}

func cloneInfo(item Info) Info {
	item.Models = append([]Model(nil), item.Models...)
	return item
}
