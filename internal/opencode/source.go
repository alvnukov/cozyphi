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
	"unicode"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/mcp"
	"github.com/alvnukov/cozyphi/internal/provider"
)

const maxFileBytes = 16 << 20

// envToken and fileToken are the substitution forms opencode expands in the
// raw config text before parsing: {env:NAME} reads the environment,
// {file:PATH} splices in a file's trimmed content.
var (
	envToken  = regexp.MustCompile(`\{env:([^}]+)\}`)
	fileToken = regexp.MustCompile(`\{file:([^}]+)\}`)
)

// globalConfigFiles lists the global config files in opencode's load order;
// later files override earlier ones field by field.
var globalConfigFiles = []string{"config.json", "opencode.json", "opencode.jsonc"}

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
// represent an empty source; a file that exists but cannot be read or parsed
// fails Load with a wrapped error.
func Load(opts Options) (*Source, error) {
	lookup := opts.LookupEnv
	if lookup == nil {
		lookup = os.Getenv
	}
	configPaths, authPath, err := paths(opts)
	if err != nil {
		return nil, err
	}
	auth, err := loadAuth(authPath)
	if err != nil {
		return nil, err
	}
	config, err := loadConfig(configPaths, lookup)
	if err != nil {
		return nil, err
	}
	models := resolveModels(
		auth, config.Provider, opts.Catalog,
		disabledSet(config.DisabledProviders), allowedSet(config.EnabledProviders),
	)
	return &Source{models: models, servers: resolveServers(config.MCP)}, nil
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

// paths resolves the config files and auth file. The config side is a list:
// opencode loads config.json, opencode.json and opencode.jsonc from its config
// directory and merges them in that order, then a set OPENCODE_CONFIG names
// one more file that merges on top of the globals. Options.ConfigPath (a test
// seam) names one explicit file instead of the whole list.
// OPENCODE_CONFIG_DIR, XDG_CONFIG_HOME and the home default name the directory.
func paths(opts Options) ([]string, string, error) {
	configPath := strings.TrimSpace(opts.ConfigPath)
	authPath := strings.TrimSpace(opts.AuthPath)
	if configPath != "" && authPath != "" {
		return []string{configPath}, authPath, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, "", fmt.Errorf("opencode: resolve home directory: %w", err)
	}
	var configPaths []string
	if configPath != "" {
		configPaths = []string{configPath}
	} else {
		// OPENCODE_CONFIG_DIR, XDG_CONFIG_HOME and the home default all name
		// the config directory whose three global files load in order; a set
		// OPENCODE_CONFIG adds one file merged over them.
		configDir := filepath.Join(home, ".config", "opencode")
		if dir := strings.TrimSpace(os.Getenv("OPENCODE_CONFIG_DIR")); dir != "" {
			configDir = dir
		} else if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
			configDir = filepath.Join(xdg, "opencode")
		}
		configPaths = make([]string, 0, len(globalConfigFiles)+1)
		for _, name := range globalConfigFiles {
			configPaths = append(configPaths, filepath.Join(configDir, name))
		}
		if env := strings.TrimSpace(os.Getenv("OPENCODE_CONFIG")); env != "" {
			configPaths = append(configPaths, env)
		}
	}
	if authPath == "" {
		dataHome := strings.TrimSpace(os.Getenv("XDG_DATA_HOME"))
		if dataHome == "" {
			dataHome = filepath.Join(home, ".local", "share")
		}
		authPath = filepath.Join(dataHome, "opencode", "auth.json")
	}
	return configPaths, authPath, nil
}

type authEntry struct {
	Type string `json:"type"`
	Key  string `json:"key"`
}

type configFile struct {
	Provider          map[string]providerConfig  `json:"provider"`
	MCP               map[string]json.RawMessage `json:"mcp"`
	DisabledProviders []string                   `json:"disabled_providers"`
	EnabledProviders  []string                   `json:"enabled_providers"`
}

type providerConfig struct {
	NPM     string                   `json:"npm"`
	API     string                   `json:"api"`
	Options providerOptions          `json:"options"`
	Models  map[string]providerModel `json:"models"`
}

type providerOptions struct {
	BaseURL string `json:"baseURL"`
	// APIKey is the post-substitution literal from the config file; the
	// reference forms ({env:}/{file:}) were already expanded while the file
	// was still raw text. It is a pointer because opencode falls back to the
	// auth.json key only when apiKey is undefined — an explicitly empty
	// string is a value that stays and blocks that fallback, so absent and
	// empty must stay distinguishable.
	APIKey *string `json:"apiKey"`
}

// providerModel carries the config fields cozyphi understands per model.
// Options and Variants port opencode's model-level request tuning: the
// wire-key JSON names on llm.ModelOptions parse exactly the keys cozyphi
// forwards (temperature, top_p, reasoning_effort, chat_template_kwargs,
// enable_thinking, thinking); anything else opencode would pass through is
// dropped at import rather than carried opaquely.
type providerModel struct {
	ID       string                        `json:"id"`
	Limit    modelLimit                    `json:"limit"`
	Options  llm.ModelOptions              `json:"options"`
	Variants map[string]llm.VariantOptions `json:"variants"`
}

// modelLimit mirrors opencode's per-model limit object. Fields are pointers
// because opencode merges them with a nullish chain: a configured 0 wins over
// the catalog while an absent field falls back to it.
type modelLimit struct {
	Context *int `json:"context"`
	Output  *int `json:"output"`
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

// loadConfig reads every config file, substitutes tokens against each file's
// own path, and deep-merges the parsed documents in order. A missing or empty
// file contributes nothing; any other read, substitution or parse error fails
// the whole load — opencode silently falls back to an empty config there,
// while cozyphi reports the problem through Load, which callers already treat
// as "no opencode source".
func loadConfig(paths []string, lookup func(string) string) (configFile, error) {
	merged := make(map[string]any, len(paths))
	for _, path := range paths {
		data, err := readOptional(path)
		if err != nil {
			return configFile{}, fmt.Errorf("opencode: read config: %w", err)
		}
		if len(data) == 0 {
			continue
		}
		expanded, err := substitute(string(data), filepath.Dir(path), lookup)
		if err != nil {
			return configFile{}, fmt.Errorf("opencode: substitute config %s: %w", path, err)
		}
		var doc map[string]any
		if err := json.Unmarshal(stripJSONC([]byte(expanded)), &doc); err != nil {
			return configFile{}, fmt.Errorf("opencode: parse config %s: %w", path, err)
		}
		mergeMaps(merged, doc)
	}
	if len(merged) == 0 {
		return configFile{}, nil
	}
	// The deep merge above needs untyped maps (merging the typed view would
	// replace nested objects instead of merging them), so re-encode the merged
	// document and decode it into the typed view here.
	encoded, err := json.Marshal(merged)
	if err != nil {
		return configFile{}, fmt.Errorf("opencode: encode merged config: %w", err)
	}
	var config configFile
	if err := json.Unmarshal(encoded, &config); err != nil {
		return configFile{}, fmt.Errorf("opencode: decode merged config: %w", err)
	}
	return config, nil
}

// mergeMaps merges src into dst the way opencode merges its global config
// files: maps merge recursively, every other value (scalar or array) replaces
// the earlier one.
func mergeMaps(dst, src map[string]any) {
	for key, value := range src {
		if srcMap, ok := value.(map[string]any); ok {
			if dstMap, ok := dst[key].(map[string]any); ok {
				mergeMaps(dstMap, srcMap)
				continue
			}
		}
		dst[key] = value
	}
}

// substitute applies opencode's raw-text substitutions to config text, porting
// ConfigVariable.substitute. The {env:NAME} pass runs over the whole text
// first, a missing variable becoming the empty string; the {file:PATH} pass
// then runs once over the result, so an environment value may itself carry a
// file token, while file content is never re-expanded.
func substitute(text, configDir string, lookup func(string) string) (string, error) {
	text = envToken.ReplaceAllStringFunc(text, func(token string) string {
		return lookup(envToken.FindStringSubmatch(token)[1])
	})
	matches := fileToken.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return text, nil
	}
	var out strings.Builder
	cursor := 0
	for _, match := range matches {
		token := text[match[0]:match[1]]
		out.WriteString(text[cursor:match[0]])
		cursor = match[1]
		if commented(text, match[0]) {
			out.WriteString(token)
			continue
		}
		resolved, err := resolveFileRef(tokenRef(token), configDir)
		if err != nil {
			return "", err
		}
		data, err := readFileCapped(resolved)
		if err != nil {
			// The message names the token, never the file content: content is
			// key material more often than not. The os.PathError underneath
			// carries only the path and errno, both safe to wrap in.
			if errors.Is(err, os.ErrNotExist) {
				return "", fmt.Errorf("bad file reference: %q %s does not exist", token, resolved)
			}
			return "", fmt.Errorf("bad file reference: %q: %w", token, err)
		}
		// Trim, then splice the content in as a JSON string body so quotes,
		// backslashes and newlines cannot corrupt the surrounding JSON. The
		// Go encoder also escapes <, > and & as \uXXXX, which the parser
		// decodes back to the same characters.
		quoted, _ := json.Marshal(strings.TrimSpace(string(data))) // cannot fail for a string
		out.Write(quoted[1 : len(quoted)-1])
	}
	out.WriteString(text[cursor:])
	return out.String(), nil
}

// tokenRef strips the {file: and } delimiters from a matched token.
func tokenRef(token string) string {
	return strings.TrimSuffix(strings.TrimPrefix(token, "{file:"), "}")
}

// commented reports whether the token at index sits on a line whose text
// before the token starts with "//" once leading whitespace is skipped —
// opencode's rule for leaving commented-out file references alone.
func commented(text string, index int) bool {
	lineStart := strings.LastIndexByte(text[:index], '\n') + 1
	prefix := strings.TrimLeftFunc(text[lineStart:index], unicode.IsSpace)
	return strings.HasPrefix(prefix, "//")
}

// resolveFileRef expands "~/" against the home directory and resolves any
// other relative reference against the config file's directory.
func resolveFileRef(ref, configDir string) (string, error) {
	if strings.HasPrefix(ref, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		return filepath.Join(home, ref[2:]), nil
	}
	if filepath.IsAbs(ref) {
		return ref, nil
	}
	return filepath.Join(configDir, ref), nil
}

// readFileCapped reads a whole file, refusing anything larger than
// maxFileBytes so neither a config file nor a {file:} reference can balloon
// memory.
func readFileCapped(path string) ([]byte, error) {
	file, err := os.Open(
		path,
	) //nolint:gosec // user-selected opencode path or {file:} reference; an explicit read-only source
	if err != nil {
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

// readOptional is readFileCapped with a missing file reading as empty.
func readOptional(path string) ([]byte, error) {
	data, err := readFileCapped(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return data, err
}

// resolveModels walks the union of auth.json api providers and config
// `provider` entries. The credential ladder mirrors opencode: the config's
// options.apiKey (already substitution-expanded) wins over an auth.json key,
// and an explicitly empty apiKey blocks the fallback — opencode checks for
// undefined only — so a provider may import keyless either way. opencode also
// layers provider-declared environment variables beneath both; cozyphi's
// catalog carries no environment list, so that layer does not exist here. A
// provider is skipped only when cozyphi cannot speak its protocol or it has
// no models.
func resolveModels(
	auth map[string]authEntry,
	configured map[string]providerConfig,
	catalog []provider.Info,
	disabled map[string]bool,
	allowed map[string]bool,
) []llm.ModelConfig {
	catalogByID := make(map[string]provider.Info, len(catalog))
	for _, item := range catalog {
		catalogByID[item.ID] = item
	}
	var result []llm.ModelConfig
	for _, id := range providerIDs(auth, configured) {
		if disabled[id] || (allowed != nil && !allowed[id]) {
			continue
		}
		item, known := catalogByID[id]
		custom := configured[id]
		protocol, baseURL := llm.ProtocolOpenAI, ""
		if known {
			protocol, baseURL = item.Protocol, item.BaseURL
		}
		switch {
		case custom.NPM != "":
			protocol, known = protocolForNPM(custom.NPM)
		case !known:
			// A config provider absent from the catalog keeps the default:
			// opencode falls back to the openai-compatible adapter for it.
			known = true
		}
		credential := ""
		if custom.Options.APIKey != nil {
			credential = *custom.Options.APIKey
		} else if entry, ok := auth[id]; ok {
			credential = entry.Key
		}
		models := overlayModels(item.Models, custom.Models, baseURL, custom.API)
		if !known || len(models) == 0 {
			continue
		}
		for _, model := range models {
			// opencode's endpoint chain: options.baseURL is provider-level
			// runtime config and wins for every model; below it each model
			// carries the url its overlay resolved (the provider api url for
			// models listed in the config, the catalog url otherwise).
			url := strings.TrimRight(model.BaseURL, "/")
			if custom.Options.BaseURL != "" {
				url = strings.TrimRight(custom.Options.BaseURL, "/")
			}
			result = append(result, llm.ModelConfig{
				Name: "opencode/" + id + "/" + model.ID, APIName: model.APIName, ProviderID: id,
				Protocol: protocol, APIKey: credential, BaseURL: url,
				ContextWindow: model.ContextWindow, MaxOutputTokens: model.MaxOutputTokens,
				Options: model.Options, Variants: model.Variants,
				ReasoningEfforts: variantEfforts(model.Variants),
			})
		}
	}
	slices.SortFunc(result, func(a, b llm.ModelConfig) int { return strings.Compare(a.Name, b.Name) })
	return result
}

// providerIDs lists the union of auth.json and config provider IDs, sorted so
// the walk is deterministic. The final model list is sorted anyway; this keeps
// intermediate behavior reproducible too.
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

// resolvedModel is one model after overlaying config entries onto the
// catalog: ID is the config map key (the public model id), APIName the wire id
// the provider's API expects, BaseURL the endpoint the overlay resolved —
// the catalog url for models the config does not list, the provider api url
// for the ones it does. Options and Variants are the model's request tuning
// as the overlay resolved it.
type resolvedModel struct {
	ID              string
	APIName         string
	BaseURL         string
	ContextWindow   int
	MaxOutputTokens int
	Options         llm.ModelOptions
	Variants        map[string]llm.VariantOptions
}

// overlayModels lays config `models` over the catalog list, porting opencode's
// per-model overlay. The map key is the model's identity; model.id is the API
// id override. A catalog match is looked up by model.id when set, else by the
// key. A matching entry keeps its limits where the config leaves them unset (a
// configured 0 still wins), while a new id is appended with the config value
// or 0. Options and variants arrive from the config only: the catalog carries
// neither today, so opencode's mergeDeep(catalog, config) degenerates to the
// config values, assigned directly. Custom keys are walked in sorted order so
// the result never depends on map iteration order.
func overlayModels(
	catalog []provider.Model,
	custom map[string]providerModel,
	catalogURL, apiURL string,
) []resolvedModel {
	result := make([]resolvedModel, len(catalog))
	index := make(map[string]int, len(catalog)+len(custom))
	for i, model := range catalog {
		result[i] = resolvedModel{
			ID: model.ID, APIName: model.ID, BaseURL: catalogURL,
			ContextWindow: model.ContextWindow, MaxOutputTokens: model.MaxOutputTokens,
		}
		index[model.ID] = i
	}
	if len(custom) == 0 {
		return result
	}
	for _, key := range slices.Sorted(maps.Keys(custom)) {
		model := custom[key]
		matchID := key
		if model.ID != "" {
			matchID = model.ID
		}
		context, output := 0, 0
		i, matched := index[matchID]
		if matched {
			// A matching entry seeds the fallback limits; the config wins
			// wherever it sets an explicit value, zero included.
			context, output = result[i].ContextWindow, result[i].MaxOutputTokens
		}
		if model.Limit.Context != nil {
			context = *model.Limit.Context
		}
		if model.Limit.Output != nil {
			output = *model.Limit.Output
		}
		// opencode folds the provider api url into the models listed in the
		// config only; a listed model without one falls back to the catalog
		// url, unlisted models keep the seed above untouched.
		modelURL := apiURL
		if modelURL == "" {
			modelURL = catalogURL
		}
		// Variants are resolved at import — disabled ones dropped — so a
		// resolvedModel never carries a variant the effort selector must skip.
		if matched && matchID == key {
			result[i].ContextWindow = context
			result[i].MaxOutputTokens = output
			result[i].BaseURL = modelURL
			result[i].Options = model.Options
			result[i].Variants = importVariants(model.Variants)
			continue
		}
		apiName := key
		if model.ID != "" {
			apiName = model.ID
		}
		index[key] = len(result)
		result = append(result, resolvedModel{
			ID: key, APIName: apiName, BaseURL: modelURL, ContextWindow: context, MaxOutputTokens: output,
			Options: model.Options, Variants: importVariants(model.Variants),
		})
	}
	slices.SortFunc(result, func(a, b resolvedModel) int { return strings.Compare(a.ID, b.ID) })
	return result
}

// importVariants ports opencode's variant resolution for one model: base
// variants merged with config variants, then every disabled variant removed
// entirely and the disabled flag stripped from the survivors. The catalog
// carries no variants, so the merge side is empty and the copy below mostly
// exists to return a detached map the resolver can hand out unchanged.
func importVariants(configured map[string]llm.VariantOptions) map[string]llm.VariantOptions {
	if len(configured) == 0 {
		return nil
	}
	imported := make(map[string]llm.VariantOptions, len(configured))
	for name, variant := range configured {
		if variant.Disabled {
			continue
		}
		variant.Disabled = false // stripped at import, not carried forward
		// Keys normalize to the effort ladder's casing: the selector stores a
		// parsed effort value ("high"), and EffectiveOptions looks the variant
		// up by that value — a "High" key would silently miss it.
		imported[strings.ToLower(strings.TrimSpace(name))] = variant
	}
	if len(imported) == 0 {
		return nil
	}
	return imported
}

// variantEfforts feeds the effort selector: the variant names that name an
// effort in cozyphi's ladder, in ladder order (none < minimal < low < medium
// < high < xhigh < max). Variant names outside the ladder ("turbo", say)
// stay reachable as variants through ModelConfig.Variants, but the selector
// is effort-only — a documented deviation in doc/opencode.md.
func variantEfforts(variants map[string]llm.VariantOptions) []llm.ReasoningEffort {
	var efforts []llm.ReasoningEffort
	for name := range variants {
		if effort, ok := llm.ParseReasoningEffort(name); ok && effort != "" {
			efforts = append(efforts, effort)
		}
	}
	llm.SortReasoningEfforts(efforts)
	return efforts
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

// allowedSet indexes enabled_providers; a nil result means no allowlist, so
// every provider stays eligible. A non-nil result filters by membership, and
// an empty-but-present array yields an empty set that allows nothing, the
// way opencode's truthy `[]` allowlist does — the JSON decode leaves the
// slice nil only when the field is absent or null.
func allowedSet(ids []string) map[string]bool {
	if ids == nil {
		return nil
	}
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

// resolveServers turns the raw `mcp` entries into server settings. Header and
// environment values need no expansion here: the raw-text substitution pass
// in loadConfig already resolved every {env:}/{file:} token before parsing.
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
			result[name] = mcp.ServerConfig{
				Transport: "http", URL: item.URL,
				Headers: item.Headers, Timeout: timeout,
			}
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
