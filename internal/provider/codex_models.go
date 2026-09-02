package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

const (
	// The backend gates models by Codex client compatibility. This value tracks
	// the official Codex CLI schema this decoder implements, not CozyPhi's app version.
	codexModelsClientVersion = "0.145.0"
	codexModelsCacheTTL      = 5 * time.Minute
	codexModelsTimeout       = 5 * time.Second
	maxCodexModelsBytes      = 16 << 20
)

// ModelCatalogWarning means authentication succeeded but the account-specific
// model catalog could not be refreshed. The stored last-known-good catalog (or
// the built-in offline fallback) remains usable.
type ModelCatalogWarning struct {
	err error
}

func (w *ModelCatalogWarning) Error() string {
	return fmt.Sprintf("signed in, but OpenAI model discovery failed; using cached/offline models: %v", w.err)
}

func (w *ModelCatalogWarning) Unwrap() error { return w.err }

type codexModelsResponse struct {
	Models []codexModelInfo `json:"models"`
}

type codexModelInfo struct {
	Slug          string `json:"slug"`
	DisplayName   string `json:"display_name"`
	Visibility    string `json:"visibility"`
	Priority      int    `json:"priority"`
	ContextWindow int    `json:"context_window"`
}

// RefreshSubscriptionModels refreshes account-specific model availability.
// A fresh, account-bound cache avoids unnecessary startup network traffic.
func (m *Manager) RefreshSubscriptionModels(ctx context.Context) error {
	return m.refreshCodexModels(ctx, false)
}

func (m *Manager) refreshCodexModels(ctx context.Context, force bool) error {
	if m == nil {
		return errors.New("provider: manager is nil")
	}
	m.mu.RLock()
	current, connected := m.credentials[openaiProviderID]
	m.mu.RUnlock()
	if !connected || current.Type != "oauth" {
		return nil
	}
	if !force && current.ModelsClientVersion == codexModelsClientVersion && len(current.Models) > 0 {
		age := time.Since(time.UnixMilli(current.ModelsFetchedAt))
		if age >= 0 && age < codexModelsCacheTTL {
			return nil
		}
	}

	credential, err := m.validOAuthCredential(ctx, openaiProviderID)
	if err != nil {
		return fmt.Errorf("provider: refresh OpenAI model catalog: %w", err)
	}
	if credential.AccountID == "" {
		return errors.New(
			"provider: refresh OpenAI model catalog: ChatGPT account id is missing; reconnect the provider",
		)
	}
	models, err := m.fetchCodexModels(ctx, credential)
	if err != nil {
		return err
	}
	return m.storeCodexModels(credential.AccountID, models)
}

func (m *Manager) fetchCodexModels(ctx context.Context, credential credential) ([]Model, error) {
	endpoint, err := url.Parse(strings.TrimRight(credential.BaseURL, "/") + "/models")
	if err != nil {
		return nil, fmt.Errorf("provider: build OpenAI models endpoint: %w", err)
	}
	query := endpoint.Query()
	query.Set("client_version", codexModelsClientVersion)
	endpoint.RawQuery = query.Encode()

	requestCtx, cancel := context.WithTimeout(ctx, codexModelsTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint.String(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("provider: build OpenAI model request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "cozyphi")
	if err := authorizeOAuthRequest(req, credential); err != nil {
		return nil, fmt.Errorf("provider: authorize OpenAI model request: %w", err)
	}

	client := *m.httpClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errors.New("OpenAI model endpoint redirects are not allowed")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("provider: fetch OpenAI model catalog: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("provider: fetch OpenAI model catalog: unexpected HTTP status %d", resp.StatusCode)
	}
	models, err := decodeCodexModels(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("provider: decode OpenAI model catalog: %w", err)
	}
	return models, nil
}

func decodeCodexModels(r io.Reader) ([]Model, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxCodexModelsBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxCodexModelsBytes {
		return nil, fmt.Errorf("response exceeds %d bytes", maxCodexModelsBytes)
	}
	var response codexModelsResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if len(response.Models) == 0 || len(response.Models) > maxModels {
		return nil, fmt.Errorf("model count %d is outside 1..%d", len(response.Models), maxModels)
	}

	listed := make([]codexModelInfo, 0, len(response.Models))
	seen := make(map[string]struct{}, len(response.Models))
	for _, raw := range response.Models {
		if raw.Visibility != "list" {
			continue
		}
		raw.Slug = strings.TrimSpace(raw.Slug)
		raw.DisplayName = strings.TrimSpace(raw.DisplayName)
		if !validCodexModelID(raw.Slug) {
			return nil, fmt.Errorf("invalid listed model id %q", raw.Slug)
		}
		if raw.DisplayName == "" {
			raw.DisplayName = raw.Slug
		}
		if len(raw.DisplayName) > maxStringBytes || raw.ContextWindow < 0 {
			return nil, fmt.Errorf("invalid metadata for listed model %q", raw.Slug)
		}
		if _, duplicate := seen[raw.Slug]; duplicate {
			return nil, fmt.Errorf("duplicate listed model %q", raw.Slug)
		}
		seen[raw.Slug] = struct{}{}
		listed = append(listed, raw)
	}
	if len(listed) == 0 {
		return nil, errors.New("response contains no models visible to this account")
	}
	slices.SortFunc(listed, func(a, b codexModelInfo) int {
		if a.Priority < b.Priority {
			return -1
		}
		if a.Priority > b.Priority {
			return 1
		}
		return strings.Compare(a.Slug, b.Slug)
	})
	models := make([]Model, 0, len(listed))
	for _, raw := range listed {
		models = append(models, Model{ID: raw.Slug, Name: raw.DisplayName, ContextWindow: raw.ContextWindow})
	}
	return models, nil
}

func validCodexModelID(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func (m *Manager) storeCodexModels(expectedAccountID string, models []Model) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.credentials[openaiProviderID]
	if !ok || current.Type != "oauth" || current.AccountID != expectedAccountID {
		return errors.New("provider: ChatGPT account changed while refreshing its model catalog")
	}
	next := cloneCredentials(m.credentials)
	current.Models = append([]Model(nil), models...)
	current.ModelsFetchedAt = time.Now().UnixMilli()
	current.ModelsClientVersion = codexModelsClientVersion
	next[openaiProviderID] = current
	if err := writeCredentials(m.credsPath, next); err != nil {
		return fmt.Errorf("provider: save OpenAI model catalog: %w", err)
	}
	m.credentials = next
	// The catalog entry stays as the public one. What this account may reach
	// belongs to its credential, which is what Models reads.
	return nil
}
