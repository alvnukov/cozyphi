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

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/util"
)

const (
	defaultCatalogURL = "https://models.dev/api.json"
	maxCatalogBytes   = 16 << 20
	maxProviders      = 512
	maxModels         = 4096
	maxStringBytes    = 4096
	maxAPIKeyBytes    = 64 << 10
)

const (
	// openaiProviderID fronts every OpenAI sign-in. A ChatGPT Pro/Plus
	// subscription and an OpenAI API key are two methods of one provider, not
	// two providers: they differ in endpoint and protocol, not in vendor.
	openaiProviderID = "openai"
	// legacyCodexProviderID is the retired Codex-only entry. It survives here
	// only as something to migrate away from.
	legacyCodexProviderID = "codex"
	openaiAPIBaseURL      = "https://api.openai.com/v1"
	chatgptCodexBaseURL   = "https://chatgpt.com/backend-api/codex"
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

// AuthMethod is one way to sign in to a provider. It carries the endpoint and
// protocol it pins, because one provider name can front two different services:
// a ChatGPT subscription talks to the Codex backend over the Responses API,
// while an OpenAI API key talks to the public API.
type AuthMethod struct {
	Kind     AuthKind
	Label    string
	BaseURL  string
	Protocol llm.Protocol
	// Models pins the catalog this method reaches. It is set only where the
	// method sees a different set than the provider's public catalog does, as
	// a ChatGPT subscription does.
	Models []Model
}

// Info is safe catalog metadata suitable for display.
type Info struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	BaseURL  string       `json:"base_url"`
	Protocol llm.Protocol `json:"protocol"`
	// Auth is the provider's own sign-in flow, and the only one for a provider
	// that declares no Methods. Where Methods exist, they decide: the first is
	// the primary flow, and each pins its own endpoint.
	Auth   AuthKind `json:"auth,omitempty"`
	Models []Model  `json:"models"`
	// Methods are the sign-in choices /connect offers, most preferred first.
	// They are a code-level contract rather than catalog data, so they stay out
	// of the cache and are restored from the built-in table on every load.
	Methods []AuthMethod `json:"-"`
}

// AuthMethods reports the sign-in choices in presentation order. A provider
// that declares none has exactly one: its own endpoint under its own Auth.
func (i Info) AuthMethods() []AuthMethod {
	if len(i.Methods) > 0 {
		return i.Methods
	}
	kind := i.Auth
	if kind == "" {
		kind = AuthAPIKey
	}
	return []AuthMethod{{Kind: kind, Label: authKindLabel(kind), BaseURL: i.BaseURL, Protocol: i.Protocol}}
}

func authKindLabel(kind AuthKind) string {
	switch kind {
	case AuthOAuthBrowser:
		return "Subscription sign-in (browser)"
	case AuthOAuthDevice:
		return "Subscription sign-in (device code)"
	default:
		return "API key"
	}
}

// IsOAuth reports whether a method signs in through OAuth rather than a stored key.
func (m AuthMethod) IsOAuth() bool {
	return m.Kind == AuthOAuthBrowser || m.Kind == AuthOAuthDevice
}

// methodOfKind returns the provider's method for one sign-in flow.
func methodOfKind(item Info, kind AuthKind) (AuthMethod, bool) {
	for _, method := range item.AuthMethods() {
		if method.Kind == kind {
			return method, true
		}
	}
	return AuthMethod{}, false
}

// credentialMethod returns the sign-in method a stored credential belongs to:
// the one whose flow matches how the credential was obtained and whose pinned
// endpoint and protocol match the contract stored with it. A credential that
// matches no method is one the catalog no longer backs, and reconnecting is the
// only safe answer.
func credentialMethod(item Info, cred credential) (AuthMethod, bool) {
	for _, method := range item.AuthMethods() {
		if method.BaseURL != cred.BaseURL || method.Protocol != cred.Protocol {
			continue
		}
		if method.IsOAuth() != (cred.Type == "oauth") {
			continue
		}
		return method, true
	}
	return AuthMethod{}, false
}

// Model is safe model metadata from the catalog.
type Model struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	ContextWindow   int    `json:"context_window"`
	MaxOutputTokens int    `json:"max_output_tokens"`
}

var reasoningEfforts = []llm.ReasoningEffort{
	llm.ReasoningEffortMinimal,
	llm.ReasoningEffortLow,
	llm.ReasoningEffortMedium,
	llm.ReasoningEffortHigh,
}

func appendReasoningEffortVariants(result *[]llm.ModelConfig, base llm.ModelConfig) {
	for _, effort := range reasoningEfforts {
		cfg := base
		cfg.Name = base.Name + ":" + string(effort)
		cfg.ReasoningEffort = effort
		*result = append(*result, cfg)
	}
}

// supportsReasoningEffort reports whether a Z.AI model accepts
// reasoning_effort. Z.AI documents it for GLM-5.2 and above only; GLM-4.x
// models ignore or reject the field.
func supportsReasoningEffort(modelID string) bool {
	return strings.HasPrefix(modelID, "glm-5")
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
	// callbackAddr is where the browser sign-in listener binds. It is a field
	// only so a test can take an ephemeral port instead of the one OpenAI pins.
	callbackAddr string
	providers    map[string]Info
	credentials  map[string]credential
}

// chatgptModels is the offline fallback for a ChatGPT subscription. The
// account's real availability comes from OpenAI's authenticated /models
// endpoint; the public catalog never describes it, because a subscription sees
// a different set of models than an API key does.
func chatgptModels() []Model {
	return []Model{
		{ID: "gpt-5.5", Name: "GPT-5.5"},
		{ID: "gpt-5.4", Name: "GPT-5.4"},
		{ID: "gpt-5.4-mini", Name: "GPT-5.4 mini"},
		{ID: "gpt-5.3-codex-spark", Name: "GPT-5.3 Codex Spark"},
	}
}

func builtinProviders() map[string]Info {
	return map[string]Info{
		openaiProviderID: {
			ID: openaiProviderID, Name: "OpenAI",
			// The triple below describes the public API, which is what an
			// unconnected provider row shows. Which way the user actually signs
			// in — subscription first — is Methods.
			BaseURL: openaiAPIBaseURL, Protocol: llm.ProtocolOpenAI, Auth: AuthAPIKey,
			// Until models.dev is reached, the subscription list stands in for
			// the public catalog: the model ids are the same on both endpoints,
			// and a catalog refresh replaces this list within seconds.
			Models: chatgptModels(),
			Methods: []AuthMethod{
				{
					Kind: AuthOAuthBrowser, Label: "ChatGPT Pro/Plus (browser)",
					BaseURL: chatgptCodexBaseURL, Protocol: llm.ProtocolOpenAIResponses,
					Models: chatgptModels(),
				},
				{
					Kind: AuthOAuthDevice, Label: "ChatGPT Pro/Plus (headless device code)",
					BaseURL: chatgptCodexBaseURL, Protocol: llm.ProtocolOpenAIResponses,
					Models: chatgptModels(),
				},
				{
					Kind: AuthAPIKey, Label: "OpenAI API key",
					BaseURL: openaiAPIBaseURL, Protocol: llm.ProtocolOpenAI,
				},
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
	// The Codex-only entry was retired: OpenAI is one provider with several
	// sign-in methods now, so neither a stale cache nor a catalog refresh may
	// bring the duplicate back.
	delete(providers, legacyCodexProviderID)
	for id, builtin := range builtinProviders() {
		// Authentication, endpoint, and protocol are trusted connection
		// contracts. A catalog refresh may update model metadata, but must not
		// redirect credentials or change the wire protocol behind the UI.
		// Subscription availability is account-specific and stays pinned on the
		// sign-in method, out of reach of the public catalog.
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
	if migrateLegacyCodexCredential(creds) {
		if err := writeCredentials(opts.CredentialsPath, creds); err != nil {
			return nil, fmt.Errorf("provider: migrate the retired Codex credential: %w", err)
		}
	}
	if err := validateCredentialContracts(providers, creds); err != nil {
		return nil, err
	}
	return &Manager{
		catalogURL:   catalogURL,
		cachePath:    opts.CachePath,
		credsPath:    opts.CredentialsPath,
		httpClient:   client,
		oauthIssuer:  defaultOAuthIssuer,
		callbackAddr: oauthCallbackAddr,
		providers:    providers,
		credentials:  creds,
	}, nil
}

// migrateLegacyCodexCredential moves a credential stored under the retired
// "codex" provider onto "openai", which now owns every OpenAI sign-in method.
// A credential already stored under "openai" wins, because that is the
// connection the user made most recently and on purpose. It reports whether the
// credential file needs rewriting.
func migrateLegacyCodexCredential(credentials map[string]credential) bool {
	legacy, ok := credentials[legacyCodexProviderID]
	if !ok {
		return false
	}
	delete(credentials, legacyCodexProviderID)
	_, connected := credentials[openaiProviderID]
	if !connected && legacy.Type == "oauth" && legacy.BaseURL == chatgptCodexBaseURL {
		credentials[openaiProviderID] = legacy
	}
	return true
}

func validateCredentialContracts(providers map[string]Info, credentials map[string]credential) error {
	for id, current := range credentials {
		item, exists := providers[id]
		if !exists {
			continue
		}
		if _, ok := credentialMethod(item, current); !ok {
			return fmt.Errorf("provider: stored connection contract for %q is invalid; reconnect the provider", id)
		}
	}
	return nil
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
	m.mu.Lock()
	defer m.mu.Unlock()
	next = mergeBuiltins(next)
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
	method, ok := methodOfKind(item, AuthAPIKey)
	if !ok {
		return fmt.Errorf("provider: %q requires subscription sign-in, not an API key", id)
	}
	if expected == "" || expected != method.BaseURL {
		return fmt.Errorf("provider: endpoint changed for %q; reopen /connect and review it", id)
	}
	next := cloneCredentials(m.credentials)
	next[id] = credential{
		Type:     "api",
		Key:      key,
		BaseURL:  method.BaseURL,
		Protocol: method.Protocol,
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
		for _, model := range connectedModels(item, cred) {
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
			if id == openaiProviderID && cred.Protocol == llm.ProtocolOpenAIResponses {
				appendReasoningEffortVariants(&result, base)
			}
			if id == "zai-coding-plan" && supportsReasoningEffort(model.ID) {
				appendReasoningEffortVariants(&result, base)
			}
		}
	}
	slices.SortFunc(result, func(a, b llm.ModelConfig) int { return strings.Compare(a.Name, b.Name) })
	return result
}

// connectedModels reports the models a stored credential actually reaches. A
// ChatGPT subscription and an OpenAI API key sit behind one provider name but
// serve different catalogs, so the credential's own contract decides which one
// applies; the account-bound list refreshed from OpenAI wins over both.
func connectedModels(item Info, cred credential) []Model {
	if len(cred.Models) > 0 {
		return cred.Models
	}
	if method, ok := credentialMethod(item, cred); ok && len(method.Models) > 0 {
		return method.Models
	}
	return item.Models
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
	item.Methods = cloneMethods(item.Methods)
	return item
}

func cloneMethods(methods []AuthMethod) []AuthMethod {
	if len(methods) == 0 {
		return nil
	}
	result := make([]AuthMethod, len(methods))
	for i, method := range methods {
		method.Models = append([]Model(nil), method.Models...)
		result[i] = method
	}
	return result
}
