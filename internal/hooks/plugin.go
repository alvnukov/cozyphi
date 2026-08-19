package hooks

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PluginFileName is the manifest that lists one or more hooks in a hooks
// directory (or a plugin subdirectory).
const PluginFileName = "plugin.json"

const (
	defaultTimeout = 5 * time.Second
	maxTimeout     = 60 * time.Second
)

// Manifest is one hook entry parsed from plugin.json.
type Manifest struct {
	Name       string
	Kind       Kind // KindPreTool, KindPostTool, or KindCommand
	Match      string
	Run        string // as written in the file (may be relative)
	Timeout    time.Duration
	FailClosed bool
	Async      bool
	Disabled   bool

	// Plugin is the enclosing plugin id (plugin.json "name", or directory name).
	Plugin string
	// Dir is the directory containing plugin.json (cwd for relative run).
	Dir string
	// Path is the absolute path to plugin.json.
	Path string
}

type pluginFile struct {
	Name  string          `json:"name"`
	Hooks []pluginHookRaw `json:"hooks"`
}

type pluginHookRaw struct {
	Name       string          `json:"name"`
	Event      string          `json:"event"`
	Match      string          `json:"match"`
	Run        string          `json:"run"`
	Timeout    json.RawMessage `json:"timeout"` // "5s" or number of seconds
	FailClosed bool            `json:"fail_closed"`
	Async      bool            `json:"async"`
	Disabled   bool            `json:"disabled"`
}

// ParsePlugin reads plugin.json and returns every hook entry.
// A file may be either {"name":"…","hooks":[…]} or a top-level […].
func ParsePlugin(path string) ([]Manifest, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("hooks: resolve plugin path: %w", err)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("hooks: read %s: %w", abs, err)
	}
	return parsePluginBytes(abs, data)
}

func parsePluginBytes(abs string, data []byte) ([]Manifest, error) {
	dir := filepath.Dir(abs)
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("hooks: %s: empty file", abs)
	}

	var (
		pluginName string
		rawHooks   []pluginHookRaw
	)
	if trimmed[0] == '[' {
		if err := json.Unmarshal(trimmed, &rawHooks); err != nil {
			return nil, fmt.Errorf("hooks: parse %s: %w", abs, err)
		}
		pluginName = filepath.Base(dir)
	} else {
		var raw pluginFile
		if err := json.Unmarshal(trimmed, &raw); err != nil {
			return nil, fmt.Errorf("hooks: parse %s: %w", abs, err)
		}
		pluginName = strings.TrimSpace(raw.Name)
		if pluginName == "" {
			pluginName = filepath.Base(dir)
		}
		rawHooks = raw.Hooks
	}
	if len(rawHooks) == 0 {
		return nil, fmt.Errorf("hooks: %s: missing hooks (want a non-empty \"hooks\" array)", abs)
	}

	out := make([]Manifest, 0, len(rawHooks))
	seen := make(map[string]struct{}, len(rawHooks))
	single := len(rawHooks) == 1
	for i, raw := range rawHooks {
		m, err := manifestFromRaw(abs, dir, pluginName, single, raw)
		if err != nil {
			return nil, fmt.Errorf("hooks: %s: hooks[%d]: %w", abs, i, err)
		}
		if _, dup := seen[m.Name]; dup {
			return nil, fmt.Errorf("hooks: %s: duplicate hook name %q", abs, m.Name)
		}
		seen[m.Name] = struct{}{}
		out = append(out, m)
	}
	return out, nil
}

func manifestFromRaw(abs, dir, pluginName string, single bool, raw pluginHookRaw) (Manifest, error) {
	m := Manifest{
		Name:       strings.TrimSpace(raw.Name),
		Match:      strings.TrimSpace(raw.Match),
		Run:        strings.TrimSpace(raw.Run),
		FailClosed: raw.FailClosed,
		Async:      raw.Async,
		Disabled:   raw.Disabled,
		Plugin:     pluginName,
		Dir:        dir,
		Path:       abs,
	}
	if m.Name == "" {
		if !single || pluginName == "" {
			return Manifest{}, errors.New("missing required field \"name\"")
		}
		m.Name = pluginName
	}
	if m.Match == "" {
		m.Match = "*"
	}

	kind, err := parseEvent(raw.Event)
	if err != nil {
		return Manifest{}, err
	}
	m.Kind = kind

	if m.Kind == KindCommand {
		m.Name = strings.TrimLeft(m.Name, "/")
		m.Name = strings.ToLower(m.Name)
		if m.Name == "" {
			return Manifest{}, errors.New("command hook name is empty")
		}
		if strings.ContainsAny(m.Name, " \t") {
			return Manifest{}, fmt.Errorf("command name %q must be a single slash token (no spaces)", m.Name)
		}
	}

	if m.Run == "" {
		return Manifest{}, errors.New("missing required field \"run\"")
	}

	timeout, err := parseTimeout(raw.Timeout)
	if err != nil {
		return Manifest{}, err
	}
	m.Timeout = timeout

	if m.Async && m.Kind != KindPostTool {
		return Manifest{}, fmt.Errorf("async is only valid for event %q", KindPostTool)
	}
	if m.FailClosed && m.Kind == KindCommand {
		return Manifest{}, fmt.Errorf("fail_closed is not valid for event %q", KindCommand)
	}

	return m, nil
}

func parseEvent(event string) (Kind, error) {
	switch Kind(strings.TrimSpace(event)) {
	case KindPreTool:
		return KindPreTool, nil
	case KindPostTool:
		return KindPostTool, nil
	case KindCommand:
		return KindCommand, nil
	case "":
		return "", errors.New("missing required field \"event\"")
	default:
		return "", fmt.Errorf("invalid event %q (want %q, %q, or %q)", event, KindPreTool, KindPostTool, KindCommand)
	}
}

func parseTimeout(raw json.RawMessage) (time.Duration, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return defaultTimeout, nil
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		asString = strings.TrimSpace(asString)
		if asString == "" {
			return defaultTimeout, nil
		}
		d, err := time.ParseDuration(asString)
		if err != nil {
			return 0, fmt.Errorf("invalid timeout %q: %w", asString, err)
		}
		return clampTimeout(d)
	}

	var asFloat float64
	if err := json.Unmarshal(raw, &asFloat); err == nil {
		if asFloat < 0 {
			return 0, fmt.Errorf("invalid timeout %v: must be >= 0", asFloat)
		}
		return clampTimeout(time.Duration(asFloat * float64(time.Second)))
	}

	return 0, fmt.Errorf("invalid timeout %s (want duration string or seconds number)", string(raw))
}

func clampTimeout(d time.Duration) (time.Duration, error) {
	if d == 0 {
		return defaultTimeout, nil
	}
	if d < 0 {
		return 0, fmt.Errorf("invalid timeout %s: must be > 0", d)
	}
	if d > maxTimeout {
		return 0, fmt.Errorf("invalid timeout %s: max is %s", d, maxTimeout)
	}
	return d, nil
}
