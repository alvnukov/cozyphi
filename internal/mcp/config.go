package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
)

const envDisable = "PHI_MCP"

// ServerConfig describes one MCP server.
type ServerConfig struct {
	Transport string            `json:"transport,omitempty"` // "stdio" (default) | "http"
	Command   []string          `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	URL       string            `json:"url,omitempty"`     // http transport
	Headers   map[string]string `json:"headers,omitempty"` // http transport
}

// fileShape is the on-disk JSON document.
type fileShape struct {
	Servers map[string]ServerConfig `json:"servers"`
}

// Disabled reports whether PHI_MCP=off.
func Disabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(envDisable)))
	return v == "0" || v == "false" || v == "off" || v == "no"
}

// UserConfigPath returns ~/.phi/mcp.json.
func UserConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".phi", "mcp.json"), nil
}

// ProjectConfigPath returns <cwd>/.phi/mcp.json.
func ProjectConfigPath(cwd string) string {
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	return filepath.Join(cwd, ".phi", "mcp.json")
}

// LogDir returns ~/.phi/logs/mcp (or PHI_MCP_LOG_DIR if set).
func LogDir() (string, error) {
	if override := strings.TrimSpace(os.Getenv("PHI_MCP_LOG_DIR")); override != "" {
		//nolint:gosec // G703: PHI_MCP_LOG_DIR is an explicit user override
		if err := os.MkdirAll(override, 0o755); err != nil {
			return "", err
		}
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".phi", "logs", "mcp")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// Load merges user + project configs (project overrides same name).
// Missing files yield an empty map without error.
func Load(cwd string) (map[string]ServerConfig, error) {
	servers := map[string]ServerConfig{}
	userPath, err := UserConfigPath()
	if err != nil {
		return nil, err
	}
	if err := mergeFile(userPath, servers); err != nil {
		return nil, err
	}
	if err := mergeFile(ProjectConfigPath(cwd), servers); err != nil {
		return nil, err
	}
	return servers, nil
}

func mergeFile(path string, into map[string]ServerConfig) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read mcp config %s: %w", path, err)
	}
	var doc fileShape
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parse mcp config %s: %w", path, err)
	}
	maps.Copy(into, doc.Servers)
	return nil
}

// SaveUser writes servers to ~/.phi/mcp.json.
func SaveUser(servers map[string]ServerConfig) error {
	path, err := UserConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if servers == nil {
		servers = map[string]ServerConfig{}
	}
	data, err := json.MarshalIndent(fileShape{Servers: servers}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644) //nolint:gosec // G306: mcp.json is meant to be user-readable
}

// AddServer upserts one server in the user config (keeps other user servers;
// does not rewrite project-only entries into the user file).
func AddServer(name string, cfg ServerConfig) error {
	path, err := UserConfigPath()
	if err != nil {
		return err
	}
	servers := map[string]ServerConfig{}
	if err := mergeFile(path, servers); err != nil {
		return err
	}
	servers[name] = cfg
	return SaveUser(servers)
}

// RemoveServer deletes a server from the user config.
func RemoveServer(name string) (bool, error) {
	path, err := UserConfigPath()
	if err != nil {
		return false, err
	}
	servers := map[string]ServerConfig{}
	if err := mergeFile(path, servers); err != nil {
		return false, err
	}
	if _, ok := servers[name]; !ok {
		return false, nil
	}
	delete(servers, name)
	return true, SaveUser(servers)
}

// CmdLine returns the full argv for spawning the server.
func (c ServerConfig) CmdLine() ([]string, error) {
	if len(c.Command) == 0 {
		return nil, errors.New("empty command")
	}
	out := append([]string{}, c.Command...)
	out = append(out, c.Args...)
	return out, nil
}

// IsStdio reports whether this server uses stdio (default).
func (c ServerConfig) IsStdio() bool {
	t := strings.TrimSpace(strings.ToLower(c.Transport))
	return t == "" || t == "stdio"
}

// IsHTTP reports whether this server uses HTTP transport.
func (c ServerConfig) IsHTTP() bool {
	t := strings.TrimSpace(strings.ToLower(c.Transport))
	return t == "http" || t == "streamable-http" || t == "sse"
}
