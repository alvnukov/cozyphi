package lsp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Config is the V1 assembly-facing configuration. Ticket 6 owns secure loading
// from ~/.cozyphi/lsp.json; this type and its defaults are the frozen seam the
// loader fills. Enabled defaults to true.
type Config struct {
	Enabled bool
	Gopls   GoplsConfig
}

// GoplsConfig holds the one production server profile. Command is the exact
// argv launched without a shell; Env are "K=V" additions to a sanitized
// inherited environment. InitializationOptions and Settings are only ever sent
// inside initialize and configuration responses, never to the model.
type GoplsConfig struct {
	Command               []string
	Env                   []string
	InitializationOptions map[string]any
	Settings              map[string]any
}

// DefaultConfig returns the built-in gopls profile. It intentionally does no
// filesystem or binary lookup: missing gopls must not break startup, and
// languages can report an install hint without ever starting a process.
func DefaultConfig() Config {
	return Config{
		Enabled: true,
		Gopls: GoplsConfig{
			Command: []string{"gopls"},
		},
	}
}

// configFile is the on-disk shape of ~/.cozyphi/lsp.json. Pointer fields
// distinguish "absent" from zero values so defaults survive partial files.
// Unknown keys fail closed at every level: only the frozen gopls profile
// exists in V1.
type configFile struct {
	Enabled *bool            `json:"enabled"`
	Gopls   *configFileGopls `json:"gopls"`
}

type configFileGopls struct {
	Command               []string          `json:"command"`
	Env                   map[string]string `json:"env"`
	InitializationOptions map[string]any    `json:"initialization_options"`
	Settings              map[string]any    `json:"settings"`
}

// LoadConfig reads the owner-controlled global LSP config. A missing file
// means built-in defaults; anything malformed, semantically invalid, or not a
// secure owner-controlled regular file (symlinked, wrong owner, group- or
// world-writable) fails closed with a sanitized error. Error messages never
// echo file contents, env values, or argv, so secrets stay out of logs.
// Project-local .cozyphi/lsp.json is intentionally unsupported: only path is
// ever read.
func LoadConfig(path string) (Config, error) {
	if strings.TrimSpace(path) == "" {
		return Config{}, errors.New("lsp: config path must not be empty")
	}
	st, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return DefaultConfig(), nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("lsp: stat config %s: %w", path, err)
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return Config{}, fmt.Errorf("lsp: config %s is a symlink; a regular owner-owned file is required", path)
	}
	f, err := openConfigFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("lsp: open config %s: %w", path, err)
	}
	defer f.Close() //nolint:errcheck // read-only descriptor
	fst, err := f.Stat()
	if err != nil {
		return Config{}, fmt.Errorf("lsp: stat config %s: %w", path, err)
	}
	if !fst.Mode().IsRegular() {
		return Config{}, fmt.Errorf("lsp: config %s is not a regular file", path)
	}
	if !configOwnedByCurrentUser(fst) {
		return Config{}, fmt.Errorf("lsp: config %s is not owned by the current user", path)
	}
	if fst.Mode().Perm()&0o022 != 0 {
		return Config{}, fmt.Errorf("lsp: config %s must not be group- or world-writable", path)
	}
	if fst.Size() > MaxConfigBytes {
		return Config{}, fmt.Errorf("lsp: config %s exceeds %d bytes", path, MaxConfigBytes)
	}
	dec := json.NewDecoder(io.LimitReader(f, MaxConfigBytes+1))
	dec.DisallowUnknownFields()
	var cf configFile
	// The wrapped error text is dropped on purpose: encoding/json messages can
	// quote fragments of file contents, and env values are secrets.
	if err := dec.Decode(&cf); err != nil {
		return Config{}, fmt.Errorf("lsp: invalid config %s: malformed or unknown-field JSON", path)
	}
	if dec.More() {
		return Config{}, fmt.Errorf("lsp: invalid config %s: trailing data", path)
	}
	return configFrom(cf)
}

// configFrom validates the decoded file semantically and fills a complete
// Config on top of the defaults.
func configFrom(cf configFile) (Config, error) {
	cfg := DefaultConfig()
	if cf.Enabled != nil {
		cfg.Enabled = *cf.Enabled
	}
	if cf.Gopls == nil {
		return cfg, nil
	}
	if cf.Gopls.Command != nil {
		if err := validateCommand(cf.Gopls.Command); err != nil {
			return Config{}, err
		}
		cfg.Gopls.Command = cf.Gopls.Command
	}
	env, err := validatedEnv(cf.Gopls.Env)
	if err != nil {
		return Config{}, err
	}
	cfg.Gopls.Env = env
	cfg.Gopls.InitializationOptions = cf.Gopls.InitializationOptions
	cfg.Gopls.Settings = cf.Gopls.Settings
	return cfg, nil
}

// validateCommand enforces the non-shell argv contract: a non-empty argv whose
// first element is an absolute path or a bare program name. Relative paths,
// subdirectories, and volume-relative forms are rejected so no value can ever
// resolve against the process working directory. Colons are rejected on every
// platform uniformly: they carry volume semantics on Windows and a basename
// containing one is never a legitimate lookup target.
func validateCommand(command []string) error {
	if len(command) == 0 {
		return errors.New("lsp: invalid config: gopls.command must not be empty")
	}
	for i, arg := range command {
		if arg == "" {
			return fmt.Errorf("lsp: invalid config: gopls.command[%d] must not be empty", i)
		}
	}
	first := command[0]
	if filepath.IsAbs(first) {
		return nil
	}
	if strings.ContainsAny(first, `/\\:`) {
		return errors.New("lsp: invalid config: gopls.command[0] must be an absolute path or a bare program name")
	}
	return nil
}

// validatedEnv converts the env map to sorted K=V additions. Keys must be
// non-empty and separator-free; values are never validated or rendered —
// they are secrets.
func validatedEnv(env map[string]string) ([]string, error) {
	if len(env) == 0 {
		return nil, nil
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		if k == "" || strings.Contains(k, "=") {
			return nil, errors.New("lsp: invalid config: gopls.env keys must be non-empty and contain no '='")
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+env[k])
	}
	return out, nil
}
