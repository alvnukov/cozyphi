package project

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/pulseaiclub/phi/internal/llm"
	"github.com/pulseaiclub/phi/internal/permission"
)

// Config is the project-level configuration loaded from ~/.phi/config.yaml.
// It keeps the primary model separate from agent-wide settings such as the
// skill directory (mirroring panda's project.Config).
type Config struct {
	PrimaryModel llm.ModelConfig
	SkillPath    string
	Permissions  permission.Policy
}

// Model returns the primary model config with the skill path applied, ready
// for agent.NewEngine.
func (c *Config) Model() llm.ModelConfig {
	m := c.PrimaryModel
	if m.SkillPath == "" {
		m.SkillPath = c.SkillPath
	}
	return m
}

// loadConfig reads the config file, applies environment overrides, and fills
// in defaults. A missing file yields a zero Config so env-only setups work.
func loadConfig(global GlobalLayout) (*Config, error) {
	cfg := parseConfigFile(global.ConfigFile())
	applyEnvOverrides(cfg)

	if cfg.PrimaryModel.APIKey == "" {
		return nil, fmt.Errorf("missing api_key (set PHI_API_KEY or primary_model.api_key in %s)", global.ConfigFile())
	}
	if cfg.PrimaryModel.Name == "" {
		return nil, fmt.Errorf("missing model name (set PHI_MODEL or primary_model.name in %s)", global.ConfigFile())
	}
	if cfg.PrimaryModel.BaseURL == "" {
		cfg.PrimaryModel.BaseURL = "https://api.openai.com/v1"
	}
	if cfg.SkillPath == "" {
		cfg.SkillPath = global.SkillsDir()
	}
	return cfg, nil
}

// parseConfigFile reads primary_model, skill_path, and permissions with a tiny
// line scanner so we don't need a YAML dependency. Missing file → zero Config
// with DefaultPolicy for permissions.
func parseConfigFile(path string) *Config {
	cfg := &Config{Permissions: permission.DefaultPolicy()}
	f, err := os.Open(path)
	if err != nil {
		return cfg
	}
	defer f.Close()

	const (
		blockNone = iota
		blockPrimary
		blockPerm
	)
	block := blockNone
	// Within permissions: "" | "bash" | "fetch" | "bash.allow" | "bash.deny" | "fetch.allowed_hosts"
	permSub := ""
	permHad := false

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := countIndent(line)

		if indent == 0 {
			block = blockNone
			permSub = ""
			if strings.HasPrefix(trimmed, "skill_path:") {
				_, val, ok := splitYAMLKV(trimmed)
				if ok {
					cfg.SkillPath = val
				}
				continue
			}
			if strings.HasPrefix(trimmed, "primary_model:") {
				block = blockPrimary
				continue
			}
			if strings.HasPrefix(trimmed, "permissions:") {
				block = blockPerm
				if !permHad {
					cfg.Permissions = permission.DefaultPolicy()
					permHad = true
				}
				continue
			}
			continue
		}

		switch block {
		case blockPrimary:
			if indent < 1 {
				continue
			}
			key, val, ok := splitYAMLKV(trimmed)
			if !ok {
				continue
			}
			switch key {
			case "name":
				cfg.PrimaryModel.Name = val
			case "api_key":
				cfg.PrimaryModel.APIKey = val
			case "base_url":
				cfg.PrimaryModel.BaseURL = val
			case "context_window":
				if n, err := strconv.Atoi(val); err == nil && n > 0 {
					cfg.PrimaryModel.ContextWindow = n
				}
			}

		case blockPerm:
			parsePermissionsLine(cfg, &permSub, indent, trimmed)
		}
	}
	return cfg
}

func parsePermissionsLine(cfg *Config, permSub *string, indent int, trimmed string) {
	// List item
	if strings.HasPrefix(trimmed, "- ") {
		item := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
		item = strings.Trim(item, `"'`)
		switch *permSub {
		case "bash.allow":
			cfg.Permissions.BashAllow = append(cfg.Permissions.BashAllow, item)
		case "bash.deny":
			cfg.Permissions.BashDeny = append(cfg.Permissions.BashDeny, item)
		case "fetch.allowed_hosts":
			cfg.Permissions.FetchAllowedHosts = append(cfg.Permissions.FetchAllowedHosts, item)
		}
		return
	}

	key, val, ok := splitYAMLKV(trimmed)
	if !ok {
		return
	}

	// Section headers / keys under permissions
	switch {
	case indent == 1 && key == "bash":
		*permSub = "bash"
		return
	case indent == 1 && key == "fetch":
		*permSub = "fetch"
		return
	case indent == 1:
		*permSub = ""
		switch key {
		case "mode":
			cfg.Permissions.Mode = permission.Mode(val)
		case "workspace_only_writes":
			cfg.Permissions.WorkspaceOnlyWrites = parseBool(val, true)
		case "ask_timeout_sec":
			if n, err := strconv.Atoi(val); err == nil && n > 0 {
				cfg.Permissions.AskTimeoutSec = n
			}
		case "dangerously_allow_all":
			cfg.Permissions.DangerouslyAllowAll = parseBool(val, false)
		}
		return
	case *permSub == "bash" && indent >= 2:
		switch key {
		case "default":
			cfg.Permissions.BashDefault = parseDecision(val, permission.Ask)
		case "allow":
			*permSub = "bash.allow"
			cfg.Permissions.BashAllow = nil // replace defaults when explicitly set
			if val != "" && val != "[]" {
				cfg.Permissions.BashAllow = append(cfg.Permissions.BashAllow, strings.Trim(val, `"'`))
			}
		case "deny":
			*permSub = "bash.deny"
			cfg.Permissions.BashDeny = nil
			if val != "" && val != "[]" {
				cfg.Permissions.BashDeny = append(cfg.Permissions.BashDeny, strings.Trim(val, `"'`))
			}
		}
		return
	case *permSub == "fetch" && indent >= 2:
		switch key {
		case "default":
			cfg.Permissions.FetchDefault = parseDecision(val, permission.Ask)
		case "allowed_hosts":
			*permSub = "fetch.allowed_hosts"
			cfg.Permissions.FetchAllowedHosts = nil
			if val != "" && val != "[]" {
				cfg.Permissions.FetchAllowedHosts = append(cfg.Permissions.FetchAllowedHosts, strings.Trim(val, `"'`))
			}
		}
		return
	case strings.HasPrefix(*permSub, "bash.") && indent == 2:
		// Leaving a list for another bash key
		switch key {
		case "default":
			*permSub = "bash"
			cfg.Permissions.BashDefault = parseDecision(val, permission.Ask)
		case "allow":
			*permSub = "bash.allow"
			cfg.Permissions.BashAllow = nil
		case "deny":
			*permSub = "bash.deny"
			cfg.Permissions.BashDeny = nil
		}
		return
	case strings.HasPrefix(*permSub, "fetch.") && indent == 2:
		switch key {
		case "default":
			*permSub = "fetch"
			cfg.Permissions.FetchDefault = parseDecision(val, permission.Ask)
		case "allowed_hosts":
			*permSub = "fetch.allowed_hosts"
			cfg.Permissions.FetchAllowedHosts = nil
		}
		return
	}
}

func countIndent(line string) int {
	n := 0
	for _, r := range line {
		if r == ' ' {
			n++
		} else if r == '\t' {
			n += 2
		} else {
			break
		}
	}
	// Treat 2 spaces as one indent level for our hand-rolled parser.
	return n / 2
}

func parseBool(val string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "true", "yes", "1":
		return true
	case "false", "no", "0":
		return false
	default:
		return def
	}
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
	if v := firstEnv("PHI_API_KEY"); v != "" {
		c.PrimaryModel.APIKey = v
	}
	if v := firstEnv("PHI_BASE_URL"); v != "" {
		c.PrimaryModel.BaseURL = v
	}
	if v := firstEnv("PHI_MODEL"); v != "" {
		c.PrimaryModel.Name = v
	}
	if v := firstEnv("PHI_SKILL_PATH"); v != "" {
		c.SkillPath = v
	}
}

func splitYAMLKV(line string) (key, val string, ok bool) {
	i := strings.IndexByte(line, ':')
	if i < 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:i])
	val = strings.TrimSpace(line[i+1:])
	val = strings.Trim(val, `"'`)
	return key, val, key != ""
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

// SetDangerouslyAllowAll persists permissions.dangerously_allow_all in config.yaml
// (Amp-compatible "Allow All for Every Session"). Best-effort rewrite of that key.
func SetDangerouslyAllowAll(global GlobalLayout, enabled bool) error {
	path := global.ConfigFile()
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	lines := []string{}
	if len(data) > 0 {
		lines = strings.Split(string(data), "\n")
	}
	val := "false"
	if enabled {
		val = "true"
	}
	inPerm := false
	found := false
	out := make([]string, 0, len(lines)+2)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		indent := countIndent(line)
		if indent == 0 && strings.HasPrefix(trimmed, "permissions:") {
			inPerm = true
			out = append(out, line)
			continue
		}
		if indent == 0 && trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			if inPerm && !found {
				out = append(out, "  dangerously_allow_all: "+val)
				found = true
			}
			inPerm = false
		}
		if inPerm && indent == 1 && strings.HasPrefix(trimmed, "dangerously_allow_all:") {
			out = append(out, "  dangerously_allow_all: "+val)
			found = true
			continue
		}
		out = append(out, line)
	}
	if inPerm && !found {
		out = append(out, "  dangerously_allow_all: "+val)
		found = true
	}
	if !found {
		if len(out) > 0 && out[len(out)-1] != "" {
			out = append(out, "")
		}
		out = append(out, "permissions:", "  dangerously_allow_all: "+val)
	}
	return os.WriteFile(path, []byte(strings.Join(out, "\n")+"\n"), 0o644)
}
