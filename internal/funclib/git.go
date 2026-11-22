// Package funclib stores executor functions as immutable Git releases and isolated session drafts.
package funclib

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alvnukov/cozyphi/internal/tools/editledger"
)

const (
	fileLimit   = 256 << 10
	gitLimit    = 2 << 20
	maxPackages = 4096
	sdkVersion  = "1"
)

var (
	validName    = regexp.MustCompile("^[a-z][a-z0-9-]{0,62}$")
	validOID     = regexp.MustCompile("^([0-9a-f]{40}|[0-9a-f]{64})$")
	validVersion = regexp.MustCompile(
		`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`,
	)
)

// Package contains source and tests for one exact revision.
type Package struct {
	Name         string            `json:"name"`
	Version      string            `json:"version,omitempty"`
	Digest       string            `json:"digest"`
	Description  string            `json:"description,omitempty"`
	Dependencies map[string]string `json:"dependencies,omitempty"`
	Source       string            `json:"source,omitempty"`
	Tests        string            `json:"tests,omitempty"`
}

// Store owns an edit ledger for the lifetime of its engine.
type Store struct {
	root       string
	ledger     *editledger.Ledger
	mu         sync.Mutex
	cache      map[string]*snapshot
	identities map[string]identity
}

// New is lazy so enabling the engine or browsing an empty catalog creates no files.
func New(root string) *Store {
	return &Store{
		root:       root,
		ledger:     editledger.New(),
		cache:      make(map[string]*snapshot),
		identities: make(map[string]identity),
	}
}

func nameOK(name string) bool { return validName.MatchString(name) }

func versionOK(version string) bool {
	parts := validVersion.FindStringSubmatch(version)
	if len(version) > 96 || parts == nil {
		return false
	}
	for p := range strings.SplitSeq(parts[4], ".") {
		if len(p) > 1 && p[0] == '0' && strings.Trim(p, "0123456789") == "" {
			return false
		}
	}
	return true
}

func opaque(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func branch(name string) string       { return "refs/heads/packages/" + name }
func tag(name, version string) string { return "refs/tags/" + name + "/v" + version }
func draftRef(session, name string) (string, error) {
	if session == "" || !nameOK(name) {
		return "", errors.New("provide a session and a lowercase package name")
	}
	return "refs/heads/drafts/" + opaque(session) + "/" + name, nil
}

func (s *Store) exists() (bool, error) {
	if s == nil || s.root == "" {
		return false, errors.New("function library directory is not configured")
	}
	info, err := os.Lstat(filepath.Join(s.root, ".git"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect function library: %w", err)
	}
	if !info.IsDir() {
		return false, errors.New("function library requires a dedicated Git directory")
	}
	return true, nil
}

func (s *Store) ensure(ctx context.Context) error {
	exists, err := s.exists()
	if exists || err != nil {
		return err
	}
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return fmt.Errorf("create function library: %w", err)
	}
	// Publish a fully initialized repository in one rename. Other processes must
	// never mistake transient initialization for an empty or corrupt library.
	tmp, err := os.MkdirTemp(s.root, ".init-")
	if err != nil {
		return fmt.Errorf("stage library: %w", err)
	}
	defer os.RemoveAll(tmp)
	if _, err = command(ctx, tmp, "", "init", "-q", "--template=", "-b", "main", "."); err != nil {
		return err
	}
	if err = os.Rename(filepath.Join(tmp, ".git"), filepath.Join(s.root, ".git")); err != nil {
		if ok, check := s.exists(); ok && check == nil {
			return nil
		}
		return fmt.Errorf("install function library: %w", err)
	}
	return nil
}

type boundedBuffer struct {
	data  bytes.Buffer
	limit int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.data.Len() {
		return 0, errors.New("git output exceeds the function library limit")
	}
	return b.data.Write(p)
}

func command(ctx context.Context, dir, input string, args ...string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	// #nosec G204 -- Git receives fixed plumbing verbs and validated names/refs; source uses stdin.
	cmd := exec.CommandContext(
		ctx,
		"git",
		append(
			[]string{"-c", "core.hooksPath=" + os.DevNull, "-c", "commit.gpgsign=false", "-c", "core.fsmonitor=false"},
			args...)...)
	cmd.Dir = dir
	for _, v := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(v), "GIT_") {
			cmd.Env = append(cmd.Env, v)
		}
	}
	cmd.Env = append(
		cmd.Env,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_TERMINAL_PROMPT=0",
		"GIT_NO_REPLACE_OBJECTS=1",
		"GIT_AUTHOR_NAME=CozyPhi",
		"GIT_COMMITTER_NAME=CozyPhi",
		"GIT_AUTHOR_EMAIL=cozyphi@localhost",
		"GIT_COMMITTER_EMAIL=cozyphi@localhost",
	)
	var out, diagnostic boundedBuffer
	out.limit, diagnostic.limit = gitLimit, 4096
	cmd.Stdout, cmd.Stderr = &out, &diagnostic
	cmd.Stdin = strings.NewReader(input)
	cmd.WaitDelay = 2 * time.Second
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(diagnostic.data.String()))
	}
	return out.data.String(), nil
}

func (s *Store) git(ctx context.Context, input string, args ...string) (string, error) {
	return command(ctx, s.root, input, append([]string{"--git-dir=.git"}, args...)...)
}

func (s *Store) ref(ctx context.Context, ref string) (string, error) {
	out, err := s.git(ctx, "", "rev-parse", "--verify", "--quiet", ref)
	var status *exec.ExitError
	if errors.As(err, &status) && status.ExitCode() == 1 {
		return "", nil
	}
	return strings.TrimSpace(out), err
}

type identity struct {
	Generation string         `json:"draft_id,omitempty"`
	Name       string         `json:"name"`
	Version    string         `json:"version,omitempty"`
	Base       string         `json:"base,omitempty"`
	SDK        string         `json:"sdk"`
	Pins       map[string]Pin `json:"pins,omitempty"`
}
type manifest struct {
	Description  string            `json:"description,omitempty"`
	Dependencies map[string]string `json:"dependencies,omitempty"`
}
type snapshot struct {
	id     identity
	files  map[string]string
	digest string
}

var editableFiles = []string{"index.ts", "index.test.ts", "package.json"}

func (sn *snapshot) manifest() (manifest, error) {
	var m manifest
	if raw, ok := sn.files["package.json"]; ok {
		dec := json.NewDecoder(strings.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&m); err != nil {
			return m, fmt.Errorf("correct package.json: %w", err)
		}
		if err := dec.Decode(new(any)); err != io.EOF {
			return m, errors.New("package.json contains trailing data")
		}
	}
	if len(m.Dependencies) > 64 || len(m.Description) > 2048 {
		return m, errors.New("package.json exceeds metadata limits")
	}
	for n, v := range m.Dependencies {
		if !nameOK(n) || !versionOK(v) || n == sn.id.Name {
			return m, errors.New("dependencies must refer to other named packages at exact semantic versions")
		}
	}
	return m, nil
}

func (sn *snapshot) pkg() Package {
	m, _ := sn.manifest() // Invalid drafts remain readable so their manifest can be repaired.
	return Package{
		Name:         sn.id.Name,
		Version:      sn.id.Version,
		Digest:       sn.digest,
		Description:  m.Description,
		Dependencies: m.Dependencies,
		Source:       sn.files["index.ts"],
		Tests:        sn.files["index.test.ts"],
	}
}

// readSnapshot loads all files through one git process and caches only immutable releases.
func (s *Store) readSnapshot(ctx context.Context, oid string) (*snapshot, error) {
	if !validOID.MatchString(oid) {
		return nil, errors.New("invalid library revision")
	}
	s.mu.Lock()
	cached := s.cache[oid]
	s.mu.Unlock()
	if cached != nil {
		return cached, nil
	}
	files := append(slices.Clone(editableFiles), "cozy-release.json")
	var batch strings.Builder
	for _, file := range files {
		fmt.Fprintf(&batch, "%s:%s\n", oid, file)
	}
	raw, err := s.git(ctx, batch.String(), "cat-file", "--batch")
	if err != nil {
		return nil, err
	}
	sn := &snapshot{digest: oid, files: make(map[string]string)}
	for _, file := range files {
		header, rest, found := strings.Cut(raw, "\n")
		if !found {
			return nil, errors.New("truncated git batch result")
		}
		raw = rest
		if strings.HasSuffix(header, " missing") {
			continue
		}
		parts := strings.Fields(header)
		if len(parts) != 3 || parts[1] != "blob" {
			return nil, errors.New("invalid library tree entry")
		}
		size, err := strconv.Atoi(parts[2])
		if err != nil || size < 0 || size > fileLimit || size >= len(raw) || raw[size] != '\n' {
			return nil, errors.New("function library blob exceeds size limit or is truncated")
		}
		body := raw[:size]
		raw = raw[size+1:]
		if file == "cozy-release.json" {
			if err := json.Unmarshal([]byte(body), &sn.id); err != nil {
				return nil, fmt.Errorf("decode library revision: %w", err)
			}
		} else {
			sn.files[file] = body
		}
	}
	if !nameOK(sn.id.Name) || sn.id.SDK != sdkVersion {
		return nil, errors.New("unsupported or corrupt library revision")
	}
	if sn.id.Version != "" {
		s.mu.Lock()
		if len(s.cache) >= 128 {
			clear(s.cache)
		}
		s.cache[oid] = sn
		s.mu.Unlock()
	}
	return sn, nil
}

func (s *Store) object(ctx context.Context, typ, body string) (string, error) {
	out, err := s.git(ctx, body, "hash-object", "-w", "-t", typ, "--stdin")
	return strings.TrimSpace(out), err
}

func (s *Store) commit(ctx context.Context, sn *snapshot, message string, parents ...string) (string, error) {
	meta, err := json.Marshal(sn.id)
	if err != nil {
		return "", err
	}
	files := append(slices.Clone(editableFiles), "cozy-release.json")
	var tree strings.Builder
	for _, file := range files {
		content, ok := sn.files[file]
		if file == "cozy-release.json" {
			content, ok = string(meta), true
		}
		if !ok {
			continue
		}
		if len(content) > fileLimit {
			return "", errors.New("source, test or metadata exceeds 256 KiB")
		}
		blob, err := s.object(ctx, "blob", content)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&tree, "100644 blob %s\t%s\n", blob, file)
	}
	out, err := s.git(ctx, tree.String(), "mktree")
	if err != nil {
		return "", err
	}
	args := []string{"commit-tree", strings.TrimSpace(out)}
	for _, parent := range parents {
		if parent != "" {
			args = append(args, "-p", parent)
		}
	}
	raw, err := s.git(ctx, message+"\n", args...)
	return strings.TrimSpace(raw), err
}

func (s *Store) move(ctx context.Context, ref, next, old string) error {
	var cmd string
	if old == "" {
		cmd = fmt.Sprintf("create %s %s\n", ref, next)
	} else {
		cmd = fmt.Sprintf("update %s %s %s\n", ref, next, old)
	}
	_, err := s.git(ctx, cmd, "update-ref", "--stdin")
	if err != nil {
		return fmt.Errorf("revision changed or could not be saved; reopen the draft: %w", err)
	}
	return nil
}

// releaseIdentity keeps the discovery path independent of source size.
func (s *Store) releaseIdentity(ctx context.Context, oid string) (identity, error) {
	if !validOID.MatchString(oid) {
		return identity{}, errors.New("invalid pinned digest")
	}
	s.mu.Lock()
	id, ok := s.identities[oid]
	s.mu.Unlock()
	if ok {
		return id, nil
	}
	raw, err := s.git(ctx, "", "cat-file", "blob", oid+":cozy-release.json")
	if err != nil {
		return id, err
	}
	if len(raw) > fileLimit {
		return id, errors.New("package metadata exceeds size limit")
	}
	if err = json.Unmarshal([]byte(raw), &id); err != nil {
		return id, fmt.Errorf("decode release metadata: %w", err)
	}
	if !nameOK(id.Name) || !versionOK(id.Version) || id.SDK != sdkVersion {
		return id, errors.New("invalid released package metadata")
	}
	for name, pin := range id.Pins {
		if !nameOK(name) || !versionOK(pin.Version) || !validOID.MatchString(pin.Digest) {
			return id, errors.New("invalid stored dependency pin")
		}
	}
	s.mu.Lock()
	if len(s.identities) >= 512 {
		clear(s.identities)
	}
	s.identities[oid] = id
	s.mu.Unlock()
	return id, nil
}

// Root returns the harness-owned location of the local Git store.
func (s *Store) Root() string { return s.root }
