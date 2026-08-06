package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"

	"gopkg.in/yaml.v3"

	"github.com/pulseaiclub/phi/internal/project"
)

//go:embed config.html
var configHTML []byte

// configDoc is the document served to / accepted from the config editor.
// The yaml tags mirror internal/project's fileConfig so the page round-trips
// config.yaml losslessly for every key the parser understands; the json tags
// drive the editor API. Pointer fields preserve "key absent" across saves so
// untouched sections are never rewritten.
type configDoc struct {
	Path        string     `yaml:"-" json:"path,omitempty"`
	Models      []modelDoc `yaml:"models" json:"models"`
	SkillPath   *string    `yaml:"skill_path,omitempty" json:"skillPath,omitempty"`
	Permissions *permDoc   `yaml:"permissions,omitempty" json:"permissions,omitempty"`
}

type modelDoc struct {
	Name          string `yaml:"name" json:"name"`
	APIKey        string `yaml:"api_key" json:"apiKey"`
	BaseURL       string `yaml:"base_url" json:"baseUrl"`
	ContextWindow *int   `yaml:"context_window,omitempty" json:"contextWindow,omitempty"`
	Default       bool   `yaml:"default,omitempty" json:"default"`
}

type permDoc struct {
	Mode                *string   `yaml:"mode,omitempty" json:"mode,omitempty"`
	WorkspaceOnlyWrites *bool     `yaml:"workspace_only_writes,omitempty" json:"workspaceOnlyWrites,omitempty"`
	AskTimeoutSec       *int      `yaml:"ask_timeout_sec,omitempty" json:"askTimeoutSec,omitempty"`
	DangerouslyAllowAll *bool     `yaml:"dangerously_allow_all,omitempty" json:"dangerouslyAllowAll,omitempty"`
	Bash                *bashDoc  `yaml:"bash,omitempty" json:"bash,omitempty"`
	Fetch               *fetchDoc `yaml:"fetch,omitempty" json:"fetch,omitempty"`
}

type bashDoc struct {
	Default *string  `yaml:"default,omitempty" json:"default,omitempty"`
	Allow   []string `yaml:"allow" json:"allow,omitempty"`
	Deny    []string `yaml:"deny" json:"deny,omitempty"`
}

type fetchDoc struct {
	Default      *string  `yaml:"default,omitempty" json:"default,omitempty"`
	AllowedHosts []string `yaml:"allowed_hosts" json:"allowedHosts,omitempty"`
}

// configCmd starts a local web server (loopback only) that edits config.yaml
// in the browser.
func configCmd(args []string) int {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			fmt.Fprintln(os.Stdout, "usage: phi config\n\nOpen the HTML config editor (starts a local web server on 127.0.0.1).")
			return ExitOK
		}
	}
	proj := project.GetDefaultProject()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintln(os.Stderr, "phi config:", err)
		return ExitError
	}
	addr := ln.Addr().(*net.TCPAddr)
	url := fmt.Sprintf("http://127.0.0.1:%d/", addr.Port)
	fmt.Fprintf(os.Stderr, "phi config: %s\n  config: %s\n  Ctrl-C to stop\n", url, proj.Global().ConfigFile())
	openBrowser(url)

	srv := &http.Server{Handler: &configHandler{configPath: proj.Global().ConfigFile()}}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	select {
	case err := <-errc:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintln(os.Stderr, "phi config:", err)
			return ExitError
		}
	case <-ctx.Done():
		srv.Close()
	}
	return ExitOK
}

// configHandler serves the embedded editor page and its /api/config endpoints.
type configHandler struct {
	configPath string
}

func (h *configHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(configHTML)
	case "/api/config":
		h.handleConfig(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *configHandler) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		doc, err := readConfigDoc(h.configPath)
		if err != nil {
			writeConfigErr(w, http.StatusInternalServerError, err)
			return
		}
		doc.Path = h.configPath
		writeConfigJSON(w, doc)
	case http.MethodPost:
		var doc configDoc
		if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
			writeConfigErr(w, http.StatusBadRequest, fmt.Errorf("bad request: %w", err))
			return
		}
		if err := validateConfigDoc(&doc); err != nil {
			writeConfigErr(w, http.StatusBadRequest, err)
			return
		}
		if err := writeConfigDoc(h.configPath, &doc); err != nil {
			writeConfigErr(w, http.StatusInternalServerError, err)
			return
		}
		writeConfigJSON(w, map[string]string{"status": "saved"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// readConfigDoc loads the current config file into the editor document. A
// missing file yields an empty document so the page can bootstrap a config.
func readConfigDoc(path string) (*configDoc, error) {
	doc := &configDoc{}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return doc, nil
		}
		return nil, err
	}
	if err := yaml.Unmarshal(data, doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func validateConfigDoc(doc *configDoc) error {
	if len(doc.Models) == 0 {
		return fmt.Errorf("at least one model is required")
	}
	hasDefault := false
	for i := range doc.Models {
		m := &doc.Models[i]
		if m.Name == "" {
			return fmt.Errorf("model %d has no name", i+1)
		}
		if !m.Default {
			continue
		}
		if hasDefault {
			return fmt.Errorf("only one model may be marked default")
		}
		hasDefault = true
		if m.APIKey == "" {
			return fmt.Errorf("default model %q is missing api_key", m.Name)
		}
	}
	if !hasDefault {
		doc.Models[0].Default = true
		if doc.Models[0].APIKey == "" {
			return fmt.Errorf("default model %q is missing api_key", doc.Models[0].Name)
		}
	}
	return nil
}

// writeConfigDoc backs up the current file and writes the document as YAML.
func writeConfigDoc(path string, doc *configDoc) error {
	data, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	if cur, err := os.ReadFile(path); err == nil {
		if err := os.WriteFile(path+".bak", cur, 0o644); err != nil {
			return fmt.Errorf("backup config: %w", err)
		}
	}
	return os.WriteFile(path, data, 0o644)
}

func writeConfigJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func writeConfigErr(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

// openBrowser best-effort opens the editor URL in the default browser.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return
	}
	_ = cmd.Start()
}
