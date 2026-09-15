package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"
)

const (
	// kimiModelsCacheTTL matches the reference plugin's six-hour cache.
	kimiModelsCacheTTL = 6 * time.Hour
	kimiModelsTimeout  = 5 * time.Second
	maxKimiModelsBytes = 16 << 20
	// kimiMaxOutputTokens is the output limit the reference plugin pins for
	// every kimi coding model: the /models endpoint lists no output maximum.
	kimiMaxOutputTokens = 65536
	// kimiModelsCacheTag marks the credential's cached model list as decoded
	// by this decoder. Kimi has no client-version negotiation like Codex does,
	// but the account-bound cache validation requires a non-empty tag.
	kimiModelsCacheTag = "kimi-1"
)

// kimiModelsResponse is the OpenAI-shaped listing GET /models returns:
// {data:[{id, display_name, context_length, supports_*}]}. Only the fields
// cozyphi uses are decoded.
type kimiModelsResponse struct {
	Data []kimiModelInfo `json:"data"`
}

type kimiModelInfo struct {
	ID            string `json:"id"`
	DisplayName   string `json:"display_name"`
	ContextLength int    `json:"context_length"`
}

func (m *Manager) refreshKimiModels(ctx context.Context, force bool) error {
	if m == nil {
		return errors.New("provider: manager is nil")
	}
	m.mu.RLock()
	current, connected := m.credentials[kimiProviderID]
	m.mu.RUnlock()
	if !connected || current.Type != "oauth" {
		return nil
	}
	if !force && current.ModelsClientVersion == kimiModelsCacheTag && len(current.Models) > 0 {
		age := time.Since(time.UnixMilli(current.ModelsFetchedAt))
		if age >= 0 && age < kimiModelsCacheTTL {
			return nil
		}
	}

	credential, err := m.validOAuthCredential(ctx, kimiProviderID)
	if err != nil {
		return fmt.Errorf("provider: refresh Kimi model catalog: %w", err)
	}
	models, err := m.fetchKimiModels(ctx, credential)
	if err != nil {
		return err
	}
	return m.storeKimiModels(models)
}

func (m *Manager) fetchKimiModels(ctx context.Context, credential credential) ([]Model, error) {
	requestCtx, cancel := context.WithTimeout(ctx, kimiModelsTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet,
		strings.TrimRight(credential.BaseURL, "/")+"/models", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("provider: build Kimi model request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "cozyphi")
	if err := (kimiGrant{}).authorize(req, credential); err != nil {
		return nil, fmt.Errorf("provider: authorize Kimi model request: %w", err)
	}

	client := *m.httpClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errors.New("kimi model endpoint redirects are not allowed")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("provider: fetch Kimi model catalog: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("provider: fetch Kimi model catalog: unexpected HTTP status %d", resp.StatusCode)
	}
	models, err := decodeKimiModels(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("provider: decode Kimi model catalog: %w", err)
	}
	return models, nil
}

func decodeKimiModels(r io.Reader) ([]Model, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxKimiModelsBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxKimiModelsBytes {
		return nil, fmt.Errorf("response exceeds %d bytes", maxKimiModelsBytes)
	}
	var response kimiModelsResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if len(response.Data) == 0 || len(response.Data) > maxModels {
		return nil, fmt.Errorf("model count %d is outside 1..%d", len(response.Data), maxModels)
	}

	models := make([]Model, 0, len(response.Data))
	seen := make(map[string]struct{}, len(response.Data))
	for _, raw := range response.Data {
		raw.ID = strings.TrimSpace(raw.ID)
		raw.DisplayName = strings.TrimSpace(raw.DisplayName)
		if !validModelID(raw.ID) {
			return nil, fmt.Errorf("invalid listed model id %q", raw.ID)
		}
		if raw.DisplayName == "" {
			raw.DisplayName = raw.ID
		}
		if len(raw.DisplayName) > maxStringBytes || raw.ContextLength < 0 {
			return nil, fmt.Errorf("invalid metadata for listed model %q", raw.ID)
		}
		if _, duplicate := seen[raw.ID]; duplicate {
			return nil, fmt.Errorf("duplicate listed model %q", raw.ID)
		}
		seen[raw.ID] = struct{}{}
		models = append(models, Model{
			ID: raw.ID, Name: raw.DisplayName,
			ContextWindow: raw.ContextLength, MaxOutputTokens: kimiMaxOutputTokens,
		})
	}
	slices.SortFunc(models, func(a, b Model) int { return strings.Compare(a.ID, b.ID) })
	return models, nil
}

func (m *Manager) storeKimiModels(models []Model) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.credentials[kimiProviderID]
	if !ok || current.Type != "oauth" || current.AccountID != kimiAccountID {
		return errors.New("provider: Kimi connection changed while refreshing its model catalog")
	}
	next := cloneCredentials(m.credentials)
	current.Models = append([]Model(nil), models...)
	current.ModelsFetchedAt = time.Now().UnixMilli()
	current.ModelsClientVersion = kimiModelsCacheTag
	next[kimiProviderID] = current
	if err := writeCredentials(m.credsPath, next); err != nil {
		return fmt.Errorf("provider: save Kimi model catalog: %w", err)
	}
	m.credentials = next
	m.storeRevision++
	return nil
}
