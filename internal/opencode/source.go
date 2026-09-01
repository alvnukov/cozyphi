// Package opencode reads provider credentials and MCP configuration owned by opencode.
// It never modifies or copies opencode state.
package opencode

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/mcp"
	"github.com/alvnukov/cozyphi/internal/provider"
)

const maxFileBytes = 16 << 20

var envToken = regexp.MustCompile(`\{env:([^}]+)\}`)

// Options identifies opencode state and the trusted cozyphi provider catalog.
type Options struct {
	ConfigPath string
	AuthPath   string
	Catalog    []provider.Info
	LookupEnv  func(string) string
}

// Source is a detached, read-only snapshot of supported opencode settings.
type Source struct {
	models  []llm.ModelConfig
	servers map[string]mcp.ServerConfig
}

// Load reads opencode's global config and API-key credentials. Missing files
// represent an empty source.
func Load(opts Options) (*Source, error) {
	lookup := opts.LookupEnv
	if lookup == nil {
		lookup = os.Getenv
	}
	configPath, authPath, err := paths(opts)
	if err != nil {
		return nil, err
	}
	auth, err := loadAuth(authPath)
	if err != nil {
		return nil, err
	}
	config, err := loadConfig(configPath, lookup)
	if err != nil {
		return nil, err
	}
	return &Source{models: resolveModels(auth, config.Provider, opts.Catalog), servers: resolveServers(config.MCP)}, nil
}

// Models returns a detached, stable list of connection-ready models.
func (s *Source) Models() []llm.ModelConfig {
	if s == nil {
		return nil
	}
	return append([]llm.ModelConfig(nil), s.models...)
}

// MCPServers returns detached MCP server settings keyed exactly as in opencode.
func (s *Source) MCPServers() map[string]mcp.ServerConfig {
	if s == nil {
		return nil
	}
	result := make(map[string]mcp.ServerConfig, len(s.servers))
	for name, cfg := range s.servers {
		cfg.Command = append([]string(nil), cfg.Command...)
		cfg.Args = append([]string(nil), cfg.Args...)
		cfg.Env = cloneMap(cfg.Env)
		cfg.Headers = cloneMap(cfg.Headers)
		result[name] = cfg
	}
	return result
}

func paths(opts Options) (string, string, error) {
	configPath := strings.TrimSpace(opts.ConfigPath)
	authPath := strings.TrimSpace(opts.AuthPath)
	if configPath != "" && authPath != "" {
		return configPath, authPath, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", fmt.Errorf("opencode: resolve home directory: %w", err)
	}
	if configPath == "" {
		switch {
		case strings.TrimSpace(os.Getenv("OPENCODE_CONFIG")) != "":
			configPath = strings.TrimSpace(os.Getenv("OPENCODE_CONFIG"))
		case strings.TrimSpace(os.Getenv("OPENCODE_CONFIG_DIR")) != "":
			configPath = filepath.Join(strings.TrimSpace(os.Getenv("OPENCODE_CONFIG_DIR")), "opencode.json")
		case strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")) != "":
			configPath = filepath.Join(strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")), "opencode", "opencode.json")
		default:
			configPath = filepath.Join(home, ".config", "opencode", "opencode.json")
		}
	}
	if authPath == "" {
		dataHome := strings.TrimSpace(os.Getenv("XDG_DATA_HOME"))
		if dataHome == "" {
			dataHome = filepath.Join(home, ".local", "share")
		}
		authPath = filepath.Join(dataHome, "opencode", "auth.json")
	}
	return configPath, authPath, nil
}

type authEntry struct {
	Type string `json:"type"`
	Key  string `json:"key"`
}

type configFile struct {
	Provider map[string]providerConfig  `json:"provider"`
	MCP      map[string]json.RawMessage `json:"mcp"`
}

type providerConfig struct {
	NPM     string                   `json:"npm"`
	Options providerOptions          `json:"options"`
	Models  map[string]providerModel `json:"models"`
}

type providerOptions struct {
	BaseURL string `json:"baseURL"`
}

type providerModel struct {
	ID    string `json:"id"`
	Limit struct {
		Context int `json:"context"`
		Output  int `json:"output"`
	} `json:"limit"`
}

type mcpConfig struct {
	Type        string            `json:"type"`
	Command     []string          `json:"command"`
	Environment map[string]string `json:"environment"`
	Enabled     *bool             `json:"enabled"`
	Disabled    bool              `json:"disabled"`
	Timeout     json.RawMessage   `json:"timeout"`
	URL         string            `json:"url"`
	Headers     map[string]string `json:"headers"`
	OAuth       json.RawMessage   `json:"oauth"`
}

func loadAuth(path string) (map[string]authEntry, error) {
	data, err := readOptional(path)
	if err != nil {
		return nil, fmt.Errorf("opencode: read auth: %w", err)
	}
	if data == nil {
		return nil, nil
	}
	var entries map[string]authEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("opencode: parse auth %s: %w", path, err)
	}
	for id, entry := range entries {
		if entry.Type != "api" || strings.TrimSpace(entry.Key) == "" {
			delete(entries, id)
		}
	}
	return entries, nil
}

func loadConfig(path string, lookup func(string) string) (configFile, error) {
	data, err := readOptional(path)
	if err != nil {
		return configFile{}, fmt.Errorf("opencode: read config: %w", err)
	}
	if data == nil {
		return configFile{}, nil
	}
	expanded := envToken.ReplaceAllStringFunc(string(data), func(token string) string {
		parts := envToken.FindStringSubmatch(token)
		return lookup(parts[1])
	})
	var config configFile
	if err := json.Unmarshal(stripJSONC([]byte(expanded)), &config); err != nil {
		return configFile{}, fmt.Errorf("opencode: parse config %s: %w", path, err)
	}
	return config, nil
}

func readOptional(path string) ([]byte, error) {
	file, err := os.Open(path) //nolint:gosec // user-selected opencode path is an explicit read-only source
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxFileBytes {
		return nil, fmt.Errorf("%s exceeds %d bytes", path, maxFileBytes)
	}
	return data, nil
}

func resolveModels(
	auth map[string]authEntry,
	configured map[string]providerConfig,
	catalog []provider.Info,
) []llm.ModelConfig {
	catalogByID := make(map[string]provider.Info, len(catalog))
	for _, item := range catalog {
		catalogByID[item.ID] = item
	}
	var result []llm.ModelConfig
	for id, credential := range auth {
		item, known := catalogByID[id]
		custom := configured[id]
		baseURL, protocol, models := item.BaseURL, item.Protocol, item.Models
		if custom.Options.BaseURL != "" {
			baseURL = strings.TrimRight(custom.Options.BaseURL, "/")
		}
		if custom.NPM != "" {
			protocol, known = protocolForNPM(custom.NPM)
		}
		if len(custom.Models) > 0 {
			models = make([]provider.Model, 0, len(custom.Models))
			for key, model := range custom.Models {
				modelID := strings.TrimSpace(model.ID)
				if modelID == "" {
					modelID = key
				}
				models = append(
					models,
					provider.Model{
						ID:              modelID,
						ContextWindow:   model.Limit.Context,
						MaxOutputTokens: model.Limit.Output,
					},
				)
			}
		}
		if !known || baseURL == "" || len(models) == 0 {
			continue
		}
		for _, model := range models {
			result = append(result, llm.ModelConfig{
				Name: "opencode/" + id + "/" + model.ID, APIName: model.ID, ProviderID: id,
				Protocol: protocol, APIKey: credential.Key, BaseURL: baseURL,
				ContextWindow: model.ContextWindow, MaxOutputTokens: model.MaxOutputTokens,
			})
		}
	}
	slices.SortFunc(result, func(a, b llm.ModelConfig) int { return strings.Compare(a.Name, b.Name) })
	return result
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

func resolveServers(raw map[string]json.RawMessage) map[string]mcp.ServerConfig {
	result := make(map[string]mcp.ServerConfig)
	for name, data := range raw {
		var item mcpConfig
		if json.Unmarshal(data, &item) != nil || item.Disabled || (item.Enabled != nil && !*item.Enabled) {
			continue
		}
		timeout := timeoutString(item.Timeout)
		switch item.Type {
		case "local":
			if len(item.Command) == 0 || strings.TrimSpace(item.Command[0]) == "" {
				continue
			}
			result[name] = mcp.ServerConfig{
				Command: item.Command[:1],
				Args:    item.Command[1:],
				Env:     item.Environment,
				Timeout: timeout,
			}
		case "remote":
			if strings.TrimSpace(item.URL) == "" || hasOAuth(item.OAuth) {
				continue
			}
			result[name] = mcp.ServerConfig{Transport: "http", URL: item.URL, Headers: item.Headers, Timeout: timeout}
		}
	}
	return result
}

func hasOAuth(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && trimmed != "null" && trimmed != "false"
}

func timeoutString(raw json.RawMessage) string {
	var milliseconds int64
	if json.Unmarshal(raw, &milliseconds) == nil && milliseconds > 0 {
		return (time.Duration(milliseconds) * time.Millisecond).String()
	}
	var value struct {
		Request int64 `json:"request"`
	}
	if json.Unmarshal(raw, &value) == nil && value.Request > 0 {
		return (time.Duration(value.Request) * time.Millisecond).String()
	}
	return ""
}

func cloneMap(input map[string]string) map[string]string {
	return maps.Clone(input)
}

// stripJSONC removes comments and trailing commas while preserving strings.
func stripJSONC(input []byte) []byte {
	output := make([]byte, 0, len(input))
	inString, escaped := false, false
	for i := 0; i < len(input); i++ {
		if inString {
			output = append(output, input[i])
			if escaped {
				escaped = false
			} else if input[i] == '\\' {
				escaped = true
			} else if input[i] == '"' {
				inString = false
			}
			continue
		}
		if input[i] == '"' {
			inString = true
			output = append(output, input[i])
			continue
		}
		if input[i] == '/' && i+1 < len(input) && input[i+1] == '/' {
			for i < len(input) && input[i] != '\n' {
				i++
			}
			output = append(output, '\n')
			continue
		}
		if input[i] == '/' && i+1 < len(input) && input[i+1] == '*' {
			i += 2
			for i+1 < len(input) && (input[i] != '*' || input[i+1] != '/') {
				i++
			}
			i++
			continue
		}
		if input[i] == ',' {
			j := i + 1
			for j < len(input) && strings.ContainsRune(" \t\r\n", rune(input[j])) {
				j++
			}
			if j < len(input) && (input[j] == '}' || input[j] == ']') {
				continue
			}
		}
		output = append(output, input[i])
	}
	return output
}
