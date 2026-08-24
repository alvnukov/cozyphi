package provider

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pulseaiclub/phi/internal/llm"
)

const (
	catalogCacheVersion = 1
	maxCacheBytes       = 16 << 20
	maxCredentialsBytes = 1 << 20
)

type catalogCache struct {
	Version   int    `json:"version"`
	Providers []Info `json:"providers"`
}

type credential struct {
	Type      string       `json:"type"`
	Key       string       `json:"key,omitempty"`
	Access    string       `json:"access,omitempty"`
	Refresh   string       `json:"refresh,omitempty"`
	Expires   int64        `json:"expires,omitempty"`
	AccountID string       `json:"account_id,omitempty"`
	BaseURL   string       `json:"base_url"`
	Protocol  llm.Protocol `json:"protocol"`
}

type credentialFile struct {
	Version   int                   `json:"version"`
	Providers map[string]credential `json:"providers"`
}

func readCatalogCache(path string) (map[string]Info, error) {
	data, err := readBounded(path, maxCacheBytes)
	if errors.Is(err, os.ErrNotExist) {
		return make(map[string]Info), nil
	}
	if err != nil {
		return nil, err
	}
	var cache catalogCache
	if err := decodeStrict(data, &cache); err != nil {
		return nil, err
	}
	if cache.Version != catalogCacheVersion {
		return nil, fmt.Errorf("unsupported cache version %d", cache.Version)
	}
	result := make(map[string]Info, len(cache.Providers))
	for _, item := range cache.Providers {
		if !validID(item.ID) || item.Name == "" || len(item.Models) == 0 {
			return nil, fmt.Errorf("invalid cached provider %q", item.ID)
		}
		if item.Protocol != llm.ProtocolOpenAI &&
			item.Protocol != llm.ProtocolOpenAIResponses &&
			item.Protocol != llm.ProtocolAnthropic {
			return nil, fmt.Errorf("invalid cached protocol for %q", item.ID)
		}
		if item.Auth == "" {
			item.Auth = AuthAPIKey
		}
		if item.Auth != AuthAPIKey && item.Auth != AuthOAuthDevice {
			return nil, fmt.Errorf("invalid cached auth kind for %q", item.ID)
		}
		if _, exists := result[item.ID]; exists {
			return nil, fmt.Errorf("duplicate cached provider %q", item.ID)
		}
		result[item.ID] = cloneInfo(item)
	}
	return result, nil
}

func writeCatalogCache(path string, providers map[string]Info) error {
	items := make([]Info, 0, len(providers))
	for _, item := range providers {
		items = append(items, cloneInfo(item))
	}
	slices.SortFunc(items, func(a, b Info) int { return strings.Compare(a.ID, b.ID) })
	return writeJSONAtomic(path, catalogCache{Version: catalogCacheVersion, Providers: items}, 0o600)
}

func readCredentials(path string) (map[string]credential, error) {
	data, err := readBounded(path, maxCredentialsBytes)
	if errors.Is(err, os.ErrNotExist) {
		return make(map[string]credential), nil
	}
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return nil, fmt.Errorf("tighten permissions: %w", err)
	}
	var file credentialFile
	if err := decodeStrict(data, &file); err != nil {
		return nil, err
	}
	if file.Version != 1 {
		return nil, fmt.Errorf("unsupported credential version %d", file.Version)
	}
	if file.Providers == nil {
		file.Providers = make(map[string]credential)
	}
	for id, item := range file.Providers {
		if !validID(id) || normalizeURL(item.BaseURL) != item.BaseURL {
			return nil, fmt.Errorf("invalid credential record for %q", id)
		}
		switch item.Type {
		case "api":
			if item.Key == "" || item.Access != "" || item.Refresh != "" {
				return nil, fmt.Errorf("invalid API credential record for %q", id)
			}
		case "oauth":
			if item.Key != "" || item.Access == "" || item.Refresh == "" || item.Expires <= 0 {
				return nil, fmt.Errorf("invalid OAuth credential record for %q", id)
			}
		default:
			return nil, fmt.Errorf("invalid credential type for %q", id)
		}
		if item.Protocol != llm.ProtocolOpenAI &&
			item.Protocol != llm.ProtocolOpenAIResponses &&
			item.Protocol != llm.ProtocolAnthropic {
			return nil, fmt.Errorf("invalid credential protocol for %q", id)
		}
	}
	return file.Providers, nil
}

func writeCredentials(path string, providers map[string]credential) error {
	return writeJSONAtomic(path, credentialFile{Version: 1, Providers: providers}, 0o600)
}

func cloneCredentials(source map[string]credential) map[string]credential {
	result := make(map[string]credential, len(source)+1)
	maps.Copy(result, source)
	return result
}

func readBounded(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s exceeds %d bytes", filepath.Base(path), limit)
	}
	return data, nil
}

func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if decoder.More() {
		return errors.New("invalid JSON: multiple values")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("invalid JSON: multiple values")
		}
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func writeJSONAtomic(path string, value any, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}
	if err := tmp.Chmod(mode); err != nil {
		cleanup()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Chmod(path, mode)
}
