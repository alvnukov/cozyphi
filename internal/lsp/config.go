package lsp

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
