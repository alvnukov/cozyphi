package project

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/alvnukov/cozyphi/internal/configfile"
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/notify"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/tasks"
	"github.com/alvnukov/cozyphi/internal/voice"

	// The keys table owns the command ids and chord grammar, so it is also
	// the keybinds validator; the package is a leaf (xui + stdlib), so the
	// import stays one-way.
	"github.com/alvnukov/cozyphi/internal/tui/keys"
)

// Config is the project-level configuration loaded from ~/.cozyphi/config.yaml.
// All models live in one flat list under the models key; DefaultModel names
// the entry used to start sessions (empty → the first entry). The plan.defaults
// section is owned by internal/harnesssettings and never appears here.
type Config struct {
	Models           []llm.ModelConfig
	DefaultModel     string // name of the default model; "" → first entry
	modelEnvOverride bool   // COZYPHI_MODEL pinned the default model via the environment
	SkillPath        string
	Permissions      permission.Policy
	Agents           AgentsConfig
	Notifications    NotificationsConfig
	OpenCode         OpenCodeConfig
	Voice            voice.Config
	// Keybinds overrides the default chord of a rebindable command, keyed by
	// the command id the config surface names (see internal/tui/keys). It is
	// validated at load and applied once at boot, before any pane exists.
	Keybinds map[string]string
	// warnings collects load-time deprecations and guesses that did not
	// fail the start (a sniffed protocol is the first one). Callers print
	// them on the way out; Warnings exposes them.
	warnings []string
}

// Warnings returns the non-fatal load-time findings: things that still work
// but that the config should say explicitly.
func (c *Config) Warnings() []string {
	if c == nil || len(c.warnings) == 0 {
		return nil
	}
	out := make([]string, len(c.warnings))
	copy(out, c.warnings)
	return out
}

// AgentsConfig controls whether the main agent may spawn sub-agents
// (agent_spawn / agent_wait / …). Default is enabled; set enabled: false
// to keep ordinary sessions lean and avoid loading the extra tool schemas.
//
// Models pins a configured model name per sub-agent role (explore|worker|
// review); a role without an entry inherits the session's model at spawn
// time. An unknown model name is not a load error — it degrades to
// inheritance with a warning — but an unknown role key is, because the
// entry could never take effect.
type AgentsConfig struct {
	Enabled bool              // true when absent from config
	Models  map[string]string // role → configured model name; empty = inherit
}

// OpenCodeConfig controls the optional read-only opencode source.
type OpenCodeConfig struct {
	Enabled bool // true when opencode.enabled is absent
}

// Model returns the default model config with the skill path applied, ready
// for agent.NewEngine. A config with no models yields a zero model and never
// grows one: the TUI resolves a fallback from its runtime catalog, headless
// entry points refuse to run without a name.
func (c *Config) Model() llm.ModelConfig {
	if c == nil {
		return llm.ModelConfig{}
	}
	m := c.defaultModel()
	if m.SkillPath == "" {
		m.SkillPath = c.SkillPath
	}
	return m
}

// ModelEnvOverride reports whether COZYPHI_MODEL pinned the default model via
// the environment. An explicit override outranks the remembered last-used model,
// so callers must not restore UI state when this is true.
func (c *Config) ModelEnvOverride() bool {
	return c != nil && c.modelEnvOverride
}

// AllModels returns every configured model with the skill path applied — the
// complete set of switchable models.
func (c *Config) AllModels() []llm.ModelConfig {
	all := make([]llm.ModelConfig, len(c.Models))
	copy(all, c.Models)
	for i := range all {
		if all[i].SkillPath == "" {
			all[i].SkillPath = c.SkillPath
		}
	}
	return all
}

// FindModel returns the configured model whose name matches, so callers can
// switch to it with its own api_key/base_url/context_window.
func (c *Config) FindModel(name string) (llm.ModelConfig, bool) {
	for _, m := range c.AllModels() {
		if m.Name == name {
			return m, true
		}
	}
	return llm.ModelConfig{}, false
}

// AgentModels is the one interpretation of agents.models pins: which model a
// role runs, and which pins no longer name anything. A pin is a display name,
// so the catalog it resolves against decides what it can name — the TUI hands
// in a lookup that also sees connected-provider models, headless hands in
// nothing and gets the static config models.
type AgentModels struct {
	pins map[string]string
	find func(string) (llm.ModelConfig, bool)
}

// AgentModels returns the resolver for this config's pins. find may be nil,
// which resolves against the configured models alone.
func (c *Config) AgentModels(find func(string) (llm.ModelConfig, bool)) AgentModels {
	if c == nil {
		return AgentModels{}
	}
	if find == nil {
		find = c.FindModel
	}
	return AgentModels{pins: c.Agents.Models, find: find}
}

// For resolves the model pinned to a sub-agent role. A role without a pin — or
// one whose name no longer resolves — reports false so the caller falls back
// to the session model: a stale name degrades to inheritance instead of
// failing the spawn.
func (a AgentModels) For(role job.Role) (llm.ModelConfig, bool) {
	name, ok := a.pins[string(role)]
	if !ok || name == "" || a.find == nil {
		return llm.ModelConfig{}, false
	}
	return a.find(name)
}

// Stale lists pins whose model name no longer resolves, as "role=name" strings
// in canonical role order. These pins degrade to inheritance; the list lets
// startup and the settings apply path warn instead of failing the spawn.
func (a AgentModels) Stale() []string {
	if len(a.pins) == 0 || a.find == nil {
		return nil
	}
	var stale []string
	for _, role := range job.Roles() {
		name := a.pins[string(role)]
		if name == "" {
			continue
		}
		if _, ok := a.find(name); !ok {
			stale = append(stale, string(role)+"="+name)
		}
	}
	return stale
}

// defaultModel is a copy of the default entry (see defaultEntry, the single
// source for that lookup), or a zero model when the config has none. Reading
// it never mutates Models: only config.yaml and the COZYPHI_* environment add
// models.
func (c *Config) defaultModel() llm.ModelConfig {
	if entry := c.defaultEntry(); entry != nil {
		return *entry
	}
	return llm.ModelConfig{}
}

// defaultEntry returns the entry the COZYPHI_* overrides land on: the named
// default, else the first one. nil when the config has no models — the
// environment decides whether one is created (see applyEnvOverrides).
func (c *Config) defaultEntry() *llm.ModelConfig {
	if c.DefaultModel != "" {
		for i := range c.Models {
			if c.Models[i].Name == c.DefaultModel {
				return &c.Models[i]
			}
		}
	}
	if len(c.Models) > 0 {
		return &c.Models[0]
	}
	return nil
}

// defaultConfigTemplate is what a first start writes to ~/.cozyphi/config.yaml.
// Every line is a comment, so the file parses to exactly the built-in defaults
// (TestDefaultTemplateParsesToBuiltInDefaults) and even a torn write can only
// ever leave a file that still loads.
const defaultConfigTemplate = `# cozyphi configuration (~/.cozyphi/config.yaml).
#
# The TUI starts with no model configured: use /connect to sign in to a
# provider, /model to pick one, or uncomment an entry below.
#
# models:
#   - name: my-model
#     api_key: sk-...
#     base_url: https://api.openai.com/v1
#     protocol: openai          # openai | openai-responses | anthropic
#     default: true
#
# Environment overrides for the default entry:
#   COZYPHI_MODEL, COZYPHI_API_KEY, COZYPHI_BASE_URL
#
# The remaining sections (permissions, agents, notifications, opencode,
# keybinds) keep their built-in defaults until written here; run
# ` + "`cozyphi config`" + ` to edit this file in the browser.
`

// ensureDefaultConfigFile plants the commented template when config.yaml does
// not exist yet, so a fresh install starts from a real file instead of an
// error. Creation is exclusive: a file that exists — however briefly — is
// never rewritten.
func ensureDefaultConfigFile(path string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil
		}
		return fmt.Errorf("create default config %s: %w", path, err)
	}
	defer file.Close() //nolint:errcheck // best-effort close; errors surface on write/sync
	// Chmod past the umask so the file is exactly 0600, like every other
	// config write (WriteOwnerOnly).
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("secure default config %s: %w", path, err)
	}
	if _, err := file.WriteString(defaultConfigTemplate); err != nil {
		return fmt.Errorf("write default config %s: %w", path, err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync default config %s: %w", path, err)
	}
	return nil
}

// loadConfig reads the config file, applies environment overrides, and fills
// in defaults. A missing file is planted with the commented template first,
// then loads as the built-in defaults, so env-only and no-model setups work.
func loadConfig(global GlobalLayout) (*Config, error) {
	createErr := ensureDefaultConfigFile(global.ConfigFile())
	cfg, err := parseConfigFile(global.ConfigFile())
	if err != nil {
		return nil, err
	}
	applyEnvOverrides(cfg)
	if createErr != nil {
		// A read-only home must not stop the start; the template is a
		// convenience, and the warning names what did not happen.
		cfg.warnings = append(cfg.warnings, fmt.Sprintf(
			"could not create %s: %v — add a model with /connect, /model, or the COZYPHI_* environment",
			global.ConfigFile(), createErr))
	}
	return finalizeConfig(cfg, global)
}

// finalizeConfig validates the merged entries and applies the path defaults.
// Zero models and a missing api_key are warnings, not errors: the TUI can
// still start and tell the user how to get a model; headless entry points
// check for a model themselves and fail with their own guidance.
func finalizeConfig(cfg *Config, global GlobalLayout) (*Config, error) {
	for i := range cfg.Models {
		if cfg.Models[i].Name == "" {
			return nil, fmt.Errorf(
				"model entry without a name (set COZYPHI_MODEL or models[].name in %s)",
				global.ConfigFile())
		}
	}
	if def := cfg.defaultModel(); def.Name != "" && def.APIKey == "" {
		cfg.warnings = append(cfg.warnings, fmt.Sprintf(
			"default model %s has no api_key — set COZYPHI_API_KEY or models[].api_key in %s",
			def.Name, global.ConfigFile()))
	}
	for i := range cfg.Models {
		warning, err := normalizeModelProtocol(&cfg.Models[i])
		if err != nil {
			return nil, fmt.Errorf("model %q: %w", cfg.Models[i].Name, err)
		}
		if warning != "" {
			cfg.warnings = append(cfg.warnings, warning)
		}
	}
	if cfg.SkillPath == "" {
		cfg.SkillPath = global.SkillsDir()
	}
	return cfg, nil
}

// LoadOpenCodeConfig reads only the optional integration setting without
// requiring a runnable model configuration. It is used by standalone MCP commands.
func LoadOpenCodeConfig(global GlobalLayout) (OpenCodeConfig, error) {
	cfg, err := parseConfigFile(global.ConfigFile())
	if err != nil {
		return OpenCodeConfig{}, err
	}
	return cfg.OpenCode, nil
}

// parseConfigFile reads models, skill_path, and permissions from the YAML
// config file. A missing file yields a zero Config with DefaultPolicy for
// permissions; a malformed file is an error so bad config never silently
// degrades to defaults.
func parseConfigFile(path string) (*Config, error) {
	cfg := &Config{
		Permissions:   permission.DefaultPolicy(),
		Agents:        AgentsConfig{Enabled: true},
		Notifications: NotificationsConfig{Mode: notify.ModeUnfocused, Sound: notify.DefaultSound},
		OpenCode:      OpenCodeConfig{Enabled: true},
		Voice:         voice.Defaults(),
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	// Pointer fields distinguish "key absent" from "zero value", so per-key
	// defaults (and permission.DefaultPolicy) survive decoding and are only
	// overridden by keys that are actually present.
	var raw fileConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	for _, m := range raw.Models {
		mc := modelEntryToConfig(m)
		if m.Default && cfg.DefaultModel == "" {
			cfg.DefaultModel = mc.Name
		}
		cfg.Models = append(cfg.Models, mc)
	}
	if raw.SkillPath != nil {
		cfg.SkillPath = *raw.SkillPath
	}
	if raw.Permissions != nil {
		if err := applyPermissions(&cfg.Permissions, raw.Permissions); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	if raw.Agents != nil {
		// Mirror the OpenCode pair: only an explicit enabled key overrides
		// the default, so an empty `agents: {}` section keeps agents on.
		if raw.Agents.Enabled != nil {
			cfg.Agents.Enabled = *raw.Agents.Enabled
		}
		models, err := job.NormalizeModels(raw.Agents.Models)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		cfg.Agents.Models = models
	}
	if len(raw.Keybinds) > 0 {
		if err := keys.CheckBinds(raw.Keybinds); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		cfg.Keybinds = raw.Keybinds
	}
	if n := raw.Notifications; n != nil {
		mode, sound, err := notify.DecodeConfig(n.Mode, n.Sound)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		cfg.Notifications.Mode, cfg.Notifications.Sound = mode, sound
	}
	if raw.OpenCode != nil && raw.OpenCode.Enabled != nil {
		cfg.OpenCode.Enabled = *raw.OpenCode.Enabled
	}
	if raw.Voice != nil {
		// internal/voice owns its own defaults, enum validation and range
		// checks, the same way notify.DecodeConfig owns the notification ones.
		v, err := voice.DecodeConfig(*raw.Voice)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		cfg.Voice = v
	}
	return cfg, nil
}

func modelEntryToConfig(m modelEntry) llm.ModelConfig {
	cfg := llm.ModelConfig{
		Name:            m.Name,
		APIName:         m.APIName,
		ProviderID:      m.ProviderID,
		Protocol:        m.Protocol,
		APIKey:          m.APIKey,
		BaseURL:         m.BaseURL,
		ReasoningEffort: llm.ReasoningEffort(m.ReasoningEffort),
	}
	if m.ContextWindow != nil && *m.ContextWindow > 0 {
		cfg.ContextWindow = *m.ContextWindow
	}
	if m.MaxOutputTokens != nil && *m.MaxOutputTokens > 0 {
		cfg.MaxOutputTokens = *m.MaxOutputTokens
	}
	return cfg
}

func normalizeModelProtocol(cfg *llm.ModelConfig) (string, error) {
	warning := ""
	if cfg.Protocol == "" {
		// Compatibility belongs at the config boundary. Transports must never
		// guess a protocol from a model name or endpoint. The guess is the
		// single shared heuristic (llm.SniffProtocol) and it costs a warning,
		// because an OpenAI-compatible gateway can serve a claude-* name on
		// the OpenAI wire format and only an explicit protocol can say so.
		cfg.Protocol = llm.SniffProtocol(cfg.Name, cfg.BaseURL)
		// Only `protocol` is named here: `provider` is carried through as
		// metadata and takes no part in this choice, so advising it would
		// send the user to a key that leaves the guess in place.
		warning = fmt.Sprintf(
			"model %s: protocol not set; guessed %s from the model name / base URL — set protocol explicitly",
			cfg.Name,
			cfg.Protocol,
		)
	}
	if cfg.Protocol != llm.ProtocolOpenAI && cfg.Protocol != llm.ProtocolOpenAIResponses &&
		cfg.Protocol != llm.ProtocolAnthropic {
		return "", fmt.Errorf("unsupported protocol %q (use %q, %q, or %q)",
			cfg.Protocol, llm.ProtocolOpenAI, llm.ProtocolOpenAIResponses, llm.ProtocolAnthropic)
	}
	if effort, ok := llm.ParseReasoningEffort(string(cfg.ReasoningEffort)); !ok {
		return "", fmt.Errorf(
			"unsupported reasoning_effort %q (use minimal, low, medium, or high)",
			cfg.ReasoningEffort,
		)
	} else {
		cfg.ReasoningEffort = effort
	}
	if cfg.BaseURL == "" {
		if cfg.Protocol == llm.ProtocolAnthropic {
			cfg.BaseURL = "https://api.anthropic.com"
		} else {
			cfg.BaseURL = "https://api.openai.com/v1"
		}
	}
	return warning, nil
}

// fileConfig mirrors the YAML keys in ~/.cozyphi/config.yaml. plan.defaults is
// deliberately absent: internal/harnesssettings owns that section.
type fileConfig struct {
	Models        []modelEntry             `yaml:"models"`
	SkillPath     *string                  `yaml:"skill_path"`
	Permissions   *permConfig              `yaml:"permissions"`
	Agents        *agentsConfig            `yaml:"agents"`
	Notifications *notificationsFileConfig `yaml:"notifications"`
	OpenCode      *openCodeFileConfig      `yaml:"opencode"`
	Voice         *voice.FileConfig        `yaml:"voice"`
	Keybinds      map[string]string        `yaml:"keybinds"`
}

type agentsConfig struct {
	Enabled *bool             `yaml:"enabled"`
	Models  map[string]string `yaml:"models"`
}

type openCodeFileConfig struct {
	Enabled *bool `yaml:"enabled"`
}

// NotificationsConfig controls desktop notifications for agent state
// changes (turn finished, waiting for input). The default mode is unfocused:
// a ping is worth having when the user is elsewhere, and noise when they are
// watching the turn. A terminal that never reports focus keeps notifying.
// Sound names what each notification plays: the platform default unless the
// config picks another, empty for silence.
type NotificationsConfig struct {
	Mode  notify.Mode `yaml:"-"`
	Sound string      `yaml:"-"`
}

// notificationsFileConfig mirrors the notifications YAML section; the mode
// string is validated into notify.Mode at load time, the sound string is
// taken as written except for "off".
type notificationsFileConfig struct {
	Mode  string `yaml:"mode"`
	Sound string `yaml:"sound"`
}

type modelEntry struct {
	Name    string `yaml:"name"`
	APIName string `yaml:"api_name"`
	// ProviderID labels which provider an entry came from; it is carried into
	// the model config for display and bookkeeping and decides nothing about
	// the connection. protocol and base_url are what the transport reads.
	ProviderID      string       `yaml:"provider"`
	Protocol        llm.Protocol `yaml:"protocol"`
	APIKey          string       `yaml:"api_key"`
	BaseURL         string       `yaml:"base_url"`
	ContextWindow   *int         `yaml:"context_window"`
	MaxOutputTokens *int         `yaml:"max_output_tokens"`
	ReasoningEffort string       `yaml:"reasoning_effort"`
	Default         bool         `yaml:"default"`
}

type permConfig struct {
	Mode                permission.Mode `yaml:"mode"`
	WorkspaceOnlyWrites *bool           `yaml:"workspace_only_writes"`
	AskTimeoutSec       *int            `yaml:"ask_timeout_sec"`
	DangerouslyAllowAll *bool           `yaml:"dangerously_allow_all"`
	Bash                *bashConfig     `yaml:"bash"`
	MCP                 *mcpConfig      `yaml:"mcp"`
	// Tasks is permissions.tasks: off, read, ask or write (the default).
	Tasks *string `yaml:"tasks"`
}

type mcpConfig struct {
	Allow *stringList `yaml:"allow"`
}

type bashConfig struct {
	Default *string     `yaml:"default"`
	Allow   *stringList `yaml:"allow"`
	Deny    *stringList `yaml:"deny"`
}

// applyPermissions merges the file's permissions block over DefaultPolicy.
// An explicitly set list (even an empty one) replaces the default list.
func applyPermissions(p *permission.Policy, raw *permConfig) error {
	if raw.Mode != "" {
		p.Mode = raw.Mode
	}
	if raw.WorkspaceOnlyWrites != nil {
		p.WorkspaceOnlyWrites = *raw.WorkspaceOnlyWrites
	}
	if raw.AskTimeoutSec != nil && *raw.AskTimeoutSec > 0 {
		p.AskTimeoutSec = *raw.AskTimeoutSec
	}
	if raw.DangerouslyAllowAll != nil {
		p.DangerouslyAllowAll = *raw.DangerouslyAllowAll
	}
	if b := raw.Bash; b != nil {
		if b.Default != nil {
			p.BashDefault = parseDecision(*b.Default, p.BashDefault)
		}
		if b.Allow != nil {
			p.BashAllow = *b.Allow
		}
		if b.Deny != nil {
			p.BashDeny = *b.Deny
		}
	}
	if m := raw.MCP; m != nil && m.Allow != nil {
		p.MCPAllow = *m.Allow
	}
	if raw.Tasks != nil {
		level, err := tasks.ParseAccess(*raw.Tasks)
		if err != nil {
			return err
		}
		p.Tasks = level
	}
	return nil
}

// stringList accepts either a single YAML scalar or a sequence, so both
// `allow: "go test ./..."` and the block list form in the README work.
type stringList []string

func (s *stringList) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		*s = stringList{node.Value}
	case yaml.SequenceNode:
		items := make(stringList, 0, len(node.Content))
		for _, n := range node.Content {
			items = append(items, n.Value)
		}
		*s = items
	default:
		return errors.New("expected a string or a list of strings")
	}
	return nil
}

func parseDecision(val string, def permission.Decision) permission.Decision {
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "allow":
		return permission.Allow
	case "deny", "reject":
		return permission.Deny
	case "ask":
		return permission.Ask
	default:
		return def
	}
}

func applyEnvOverrides(c *Config) {
	// entry is the override target: the default entry, or a freshly created
	// one for an env-only setup — the only path besides config.yaml that may
	// add a model. Created lazily so a bare environment with no COZYPHI_*
	// model keys leaves a zero-model config zero.
	entry := func() *llm.ModelConfig {
		if e := c.defaultEntry(); e != nil {
			return e
		}
		c.Models = append(c.Models, llm.ModelConfig{})
		return &c.Models[0]
	}
	if v := firstEnv("COZYPHI_API_KEY"); v != "" {
		entry().APIKey = v
	}
	if v := firstEnv("COZYPHI_BASE_URL"); v != "" {
		entry().BaseURL = v
	}
	if v := firstEnv("COZYPHI_MODEL"); v != "" {
		entry().Name = v
		c.DefaultModel = v
		c.modelEnvOverride = true
	}
	if v := firstEnv("COZYPHI_SKILL_PATH"); v != "" {
		c.SkillPath = v
	}
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

// WriteOwnerOnly writes data to path with owner-only permissions, then
// tightens the existing file: a config.yaml (or its backup) an older release
// left world-readable is corrected on the next write. Callers pass fixed
// config file locations, not user input.
func WriteOwnerOnly(path string, data []byte) error {
	//nolint:gosec // G703: callers pass fixed config file locations, not user input
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

// SetDangerouslyAllowAll persists permissions.dangerously_allow_all in config.yaml
// ("Allow All for Every Session"). The write is one configfile.Edit cycle — the
// single owner of config.yaml writes — so it is serialized against every other
// writer in the process, commits atomically, and touches only this one key.
// A config that cannot be parsed fails closed and is left untouched.
func SetDangerouslyAllowAll(global GlobalLayout, enabled bool) error {
	return configfile.Edit(global.ConfigFile(), func(doc *yaml.Node) error {
		var value yaml.Node
		if err := value.Encode(enabled); err != nil {
			return err
		}
		configfile.Set(doc, &value, "permissions", "dangerously_allow_all")
		return nil
	})
}
