package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ManifestFileName is the required checklist file inside each hook directory.
const ManifestFileName = "hook.json"

const (
	defaultTimeout = 5 * time.Second
	maxTimeout     = 60 * time.Second
)

// Manifest is a parsed hook.json checklist (one hook directory).
type Manifest struct {
	Name        string
	Description string
	Kind        Kind // KindPreTool, KindPostTool, KindRegisterTool
	Args        json.RawMessage
	Match       string
	Run         string // as written in the file (may be relative)
	Timeout     time.Duration
	FailClosed  bool
	Async       bool
	Disabled    bool

	// Dir is the hook directory (parent of hook.json).
	Dir string
	// Path is the absolute path to hook.json.
	Path string
}

type manifestFile struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Event       string          `json:"event"`
	Match       string          `json:"match"`
	Args        json.RawMessage `json:"args"`
	Run         string          `json:"run"`
	Timeout     json.RawMessage `json:"timeout"` // "5s" or number of seconds
	FailClosed  bool            `json:"fail_closed"`
	Async       bool            `json:"async"`
	Disabled    bool            `json:"disabled"`
}

// ParseManifest reads and validates a hook.json file.
// Callers should skip the hook on error (discover collects warnings).
func ParseManifest(path string) (Manifest, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("hooks: resolve manifest path: %w", err)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return Manifest{}, fmt.Errorf("hooks: read %s: %w", abs, err)
	}

	var raw manifestFile
	if err := json.Unmarshal(data, &raw); err != nil {
		return Manifest{}, fmt.Errorf("hooks: parse %s: %w", abs, err)
	}

	dir := filepath.Dir(abs)
	m := Manifest{
		Name:        strings.TrimSpace(raw.Name),
		Description: strings.TrimSpace(raw.Description),
		Match:       strings.TrimSpace(raw.Match),
		Run:         strings.TrimSpace(raw.Run),
		FailClosed:  raw.FailClosed,
		Async:       raw.Async,
		Disabled:    raw.Disabled,
		Dir:         dir,
		Path:        abs,
		Args:        raw.Args,
	}
	if m.Name == "" {
		m.Name = filepath.Base(dir)
	}

	if m.Match == "" {
		m.Match = "*"
	}

	kind, err := parseEvent(raw.Event)
	if err != nil {
		return Manifest{}, fmt.Errorf("hooks: %s: %w", abs, err)
	}
	m.Kind = kind

	if m.Run == "" {
		return Manifest{}, fmt.Errorf("hooks: %s: missing required field \"run\"", abs)
	}

	timeout, err := parseTimeout(raw.Timeout)
	if err != nil {
		return Manifest{}, fmt.Errorf("hooks: %s: %w", abs, err)
	}
	m.Timeout = timeout

	if m.Async && m.Kind != KindPostTool {
		return Manifest{}, fmt.Errorf("hooks: %s: async is only valid for event %q", abs, KindPostTool)
	}

	if m.Kind == KindPostTool && m.Description == "" {
		return Manifest{}, fmt.Errorf("hooks: %s: missing required field \"description\"", abs)
	}

	return m, nil
}

func parseEvent(event string) (Kind, error) {
	switch Kind(strings.TrimSpace(event)) {
	case KindPreTool:
		return KindPreTool, nil
	case KindPostTool:
		return KindPostTool, nil
	case KindRegisterTool:
		return KindRegisterTool, nil
	case "":
		return "", fmt.Errorf("missing required field \"event\"")
	default:
		return "", fmt.Errorf("invalid event %q (want %q or %q)", event, KindPreTool, KindPostTool)
	}
}

func parseTimeout(raw json.RawMessage) (time.Duration, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return defaultTimeout, nil
	}

	// String duration: "5s", "500ms"
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

	// Number: seconds (int or float)
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
