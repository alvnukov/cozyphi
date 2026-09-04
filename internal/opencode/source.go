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

// envToken and fileToken are the credential reference forms opencode accepts:
// {env:NAME} reads the environment, {file:PATH} reads a credential file.
// envToken is also expanded across the raw config text in loadConfig; a file
// reference never gets that text pass — file content could corrupt the JSON
// parse — so fileToken is resolved after parsing: as a whole options.apiKey
// value, and embedded in MCP header and environment values.
var (
	envToken  = regexp.MustCompile(`\{env:([^}]+)\}`)
	fileToken = regexp.MustCompile(`\{file:([^}]+)\}`)
)

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
	keys := keySource{lookupEnv: lookup, readFile: os.ReadFile}
	models := resolveModels(auth, config.Provider, opts.Catalog, disabledSet(config.DisabledProviders), keys)
	return &Source{models: models, servers: resolveServers(config.MCP, keys.expandFileTokens)}, nil
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
	Provider          map[string]providerConfig  `json:"provider"`
	MCP               map[string]json.RawMessage `json:"mcp"`
	DisabledProviders []string                   `json:"disabled_providers"`
}

type providerConfig struct {
	NPM     string                   `json:"npm"`
	Options providerOptions          `json:"options"`
	Models  map[string]providerModel `json:"models"`
}

type providerOptions struct {
	BaseURL string `json:"baseURL"`
	// APIKey stays raw because opencode accepts a plain string and an object
	// form ({"env":..}/{"file":..}); keySource resolves both.
	APIKey json.RawMessage `json:"apiKey"`
}

// apiKeyRef is the object form of options.apiKey: {"env":"NAME"} resolves
// through the environment, {"file":"PATH"} reads the credential from disk.
type apiKeyRef struct {
	Env  string `json:"env"`
	File string `json:"file"`
}

// keySource turns an options.apiKey value into a credential string. Both
// lookups are injected so resolveModels stays a pure function over its
// arguments. A missing or unreadable file yields an empty key — the provider
// is still imported and the provider's own auth error names the real problem,
// where silently dropping it would hide a working endpoint. Key material never
// reaches an error or a log line.
type keySource struct {
	lookupEnv func(string) string
	readFile  func(string) ([]byte, error)
}

func (s keySource) resolveKey(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}
	var literal string
	if err := json.Unmarshal(raw, &literal); err == nil {
		return s.resolveStringKey(literal)
	}
	var ref apiKeyRef
	if json.Unmarshal(raw, &ref) != nil || (ref.Env == "" && ref.File == "") {
		return ""
	}
	if ref.Env != "" {
		return s.lookupEnv(ref.Env)
	}
	return s.readKeyFile(ref.File)
}

func (s keySource) resolveStringKey(value string) string {
	// loadConfig already expands {env:..} tokens in the raw file text, so a
	// token only reaches a parsed value when an expanded value itself embeds
	// one — resolving again follows that nesting instead of leaking the raw
	// token into a credential. For an apiKey, {file:..} counts only as a whole
	// value — an embedded token stays part of the key. MCP header and
	// environment values are the one place embedded tokens expand
	// (expandFileTokens).
	if name, ok := wholeToken(envToken, value); ok {
		return s.lookupEnv(name)
	}
	if path, ok := wholeToken(fileToken, value); ok {
		return s.readKeyFile(path)
	}
	return value
}

// wholeToken reports value as exactly one re token and returns its capture;
// a token embedded in a longer string is not a reference.
func wholeToken(re *regexp.Regexp, value string) (string, bool) {
	m := re.FindStringSubmatch(value)
	if len(m) != 2 || m[0] != value {
		return "", false
	}
	return m[1], true
}

func (s keySource) readKeyFile(path string) string {
	data, err := s.readFile(path) //nolint:gosec // path comes from the user's own opencode.json
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// expandFileTokens replaces every {file:PATH} token in value — including one
// embedded in longer text, such as `Bearer {file:…}` — with the referenced
// file's trimmed content. It runs on parsed MCP header and environment values
// only: expanding into the raw config text would let file content corrupt the
// JSON parse. A missing or unreadable file expands to an empty string so a
// broken reference never fails the load — the endpoint's own auth error names
// the real problem. Key material never reaches an error or a log line.
func (s keySource) expandFileTokens(value string) string {
	return fileToken.ReplaceAllStringFunc(value, func(token string) string {
		return s.readKeyFile(fileToken.FindStringSubmatch(token)[1])
	})
}

type providerModel struct {
	ID    string     `json:"id"`
	Limit modelLimit `json:"limit"`
}

// modelLimit mirrors opencode's per-model limit object; zero means unset.
type modelLimit struct {
	Context int `json:"context"`
	Output  int `json:"output"`
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

// resolveModels walks the union of auth.json api providers and opencode.json
// `provider` entries. auth.json owns the credential when it has one; a provider
// declared only in opencode.json may instead carry its key in options.apiKey,
// and imports even keyless when endpoint, models and protocol line up. A
// provider absent from opencode.json still needs an auth.json key, and an
// auth.json entry CozyPhi knows nothing about (catalog or config) is skipped.
func resolveModels(
	auth map[string]authEntry,
	configured map[string]providerConfig,
	catalog []provider.Info,
	disabled map[string]bool,
	keys keySource,
) []llm.ModelConfig {
	catalogByID := make(map[string]provider.Info, len(catalog))
	for _, item := range catalog {
		catalogByID[item.ID] = item
	}
	var result []llm.ModelConfig
	for _, id := range providerIDs(auth, configured) {
		if disabled[id] {
			continue
		}
		item, known := catalogByID[id]
		custom, declared := configured[id]
		baseURL, protocol := item.BaseURL, item.Protocol
		if custom.Options.BaseURL != "" {
			baseURL = strings.TrimRight(custom.Options.BaseURL, "/")
		}
		if custom.NPM != "" {
			protocol, known = protocolForNPM(custom.NPM)
		}
		credential := ""
		if entry, ok := auth[id]; ok {
			credential = entry.Key
		} else if declared {
			credential = keys.resolveKey(custom.Options.APIKey)
		}
		models := overlayModels(item.Models, custom.Models)
		if !known || baseURL == "" || len(models) == 0 {
			continue
		}
		for _, model := range models {
			result = append(result, llm.ModelConfig{
				Name: "opencode/" + id + "/" + model.ID, APIName: model.ID, ProviderID: id,
				Protocol: protocol, APIKey: credential, BaseURL: baseURL,
				ContextWindow: model.ContextWindow, MaxOutputTokens: model.MaxOutputTokens,
			})
		}
	}
	slices.SortFunc(result, func(a, b llm.ModelConfig) int { return strings.Compare(a.Name, b.Name) })
	return result
}

// providerIDs lists the union of auth.json and opencode.json provider IDs,
// sorted so the walk is deterministic. The final model list is sorted anyway;
// this keeps intermediate behavior reproducible too.
func providerIDs(auth map[string]authEntry, configured map[string]providerConfig) []string {
	ids := make([]string, 0, len(auth)+len(configured))
	for id := range auth {
		ids = append(ids, id)
	}
	for id := range configured {
		if _, ok := auth[id]; !ok {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids
}

// overlayModels lays opencode.json `models` over the catalog list: catalog
// models stay, an entry whose id matches one overrides it (a limit only wins
// when set above zero), and an entry with a new id is added — the id field
// names the model, falling back to the map key. The result is sorted by model
// id so a provider's list is stable regardless of map order.
func overlayModels(catalog []provider.Model, custom map[string]providerModel) []provider.Model {
	if len(custom) == 0 {
		return catalog
	}
	result := make([]provider.Model, len(catalog))
	copy(result, catalog)
	index := make(map[string]int, len(result)+len(custom))
	for i, model := range result {
		index[model.ID] = i
	}
	for key, model := range custom {
		id := strings.TrimSpace(model.ID)
		if id == "" {
			id = key
		}
		if i, ok := index[id]; ok {
			if model.Limit.Context > 0 {
				result[i].ContextWindow = model.Limit.Context
			}
			if model.Limit.Output > 0 {
				result[i].MaxOutputTokens = model.Limit.Output
			}
			continue
		}
		index[id] = len(result)
		result = append(result, provider.Model{
			ID: id, ContextWindow: model.Limit.Context, MaxOutputTokens: model.Limit.Output,
		})
	}
	slices.SortFunc(result, func(a, b provider.Model) int { return strings.Compare(a.ID, b.ID) })
	return result
}

// disabledSet indexes the top-level disabled_providers ids for the membership
// check inside resolveModels; it is passed in so resolveModels reaches for no
// state of its own.
func disabledSet(ids []string) map[string]bool {
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id = strings.TrimSpace(id); id != "" {
			set[id] = true
		}
	}
	return set
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

// resolveServers turns the raw `mcp` entries into server settings. expand is
// applied to local `environment` and remote `headers` values after parsing, so
// an embedded {file:PATH} token resolves from its credential file without a
// text pass over the raw config that file content could corrupt.
func resolveServers(raw map[string]json.RawMessage, expand func(string) string) map[string]mcp.ServerConfig {
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
				Env:     expandMapValues(item.Environment, expand),
				Timeout: timeout,
			}
		case "remote":
			if strings.TrimSpace(item.URL) == "" || hasOAuth(item.OAuth) {
				continue
			}
			result[name] = mcp.ServerConfig{
				Transport: "http", URL: item.URL,
				Headers: expandMapValues(item.Headers, expand), Timeout: timeout,
			}
		}
	}
	return result
}

// expandMapValues applies expand to every value, returning nil for a nil map
// so an absent `environment` or `headers` section stays absent.
func expandMapValues(input map[string]string, expand func(string) string) map[string]string {
	if input == nil {
		return nil
	}
	result := make(map[string]string, len(input))
	for key, value := range input {
		result[key] = expand(value)
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
