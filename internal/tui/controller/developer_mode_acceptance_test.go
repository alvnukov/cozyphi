package controller

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/tools/harnesstool"
	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
)

// The three promises no single collector can keep on its own, asked of all
// eleven at once over the real wiring: that nothing secret leaves through any
// channel, that observing changes nothing, and that the one interface eleven
// owners share holds up under several callers, a cancelled one, and sessions
// that must not answer for each other.
//
// Each collector already proves its own half of the first promise for its own
// category. What is left is the sweep: one session configured with a secret in
// every place a secret can be written, asked every question the tool accepts —
// including the ones it has to refuse — with every answer, every refusal, the
// transcript they land in and the audit record they leave searched together.

// secretMark spells every planted secret, so one search finds a leaked key, a
// leaked URL, a leaked command line, a leaked rule and a leaked memory alike.
// Nothing the harness may legitimately report is spelled with it: the models
// are alpha and beta, the servers are shared and vault, the hook is guard-bash.
const secretMark = "sentinel"

// sentinelConfigYAML writes a secret into every value the harness view is not
// allowed to carry: a provider key and endpoint, the voice backend's key,
// endpoint and command line, the capture command and device, and the bash and
// mcp rules a user wrote. What may be reported — a model name, a window, a
// mode — is spelled plainly, so a hit on the mark is always a leak.
const sentinelConfigYAML = `models:
  - name: alpha
    api_name: alpha-wire
    protocol: openai
    api_key: sentinel-model-key
    base_url: http://sentinel-model-host:9/sentinel-path
    context_window: 111000
    default: true
  - name: beta
    api_name: beta-wire
    protocol: openai
    api_key: sentinel-second-key
    base_url: http://sentinel-second-host:9
    context_window: 222000
voice:
  enabled: true
  capture:
    command: sentinel-capture --device sentinel-mic
    device: sentinel-mic
  stt:
    backend: http
    model: whisper-1
    command: sentinel-stt --key sentinel-voice-key
    api_key: sentinel-voice-key
    base_url: http://sentinel-voice-host:9
` + permissionsBlock

// sentinelMCPConfig names a server plainly and hides a token in the two places
// a stdio server carries one: its arguments and its environment.
const sentinelMCPConfig = `{
  "servers": {
    "vault": {
      "command": ["true"],
      "args": ["--token", "sentinel-mcp-arg"],
      "env": {"TOKEN": "sentinel-mcp-env"}
    }
  }
}`

// sentinelHooks names the hook plainly and puts the secret on its command
// line, which is the part a hook manifest most often has one on.
const sentinelHooks = `{"hooks":[
  {"name":"guard-bash","event":"pre_tool","match":"bash","run":"./run.sh --sentinel-hook-arg","timeout":"9s"}
]}`

// auditLog is the sink the registry reports every request to, kept so the
// record can be searched with everything else. It is written from whichever
// goroutine answered, so it locks.
type auditLog struct {
	mu    sync.Mutex
	lines []string
}

func (l *auditLog) sink(event diag.AuditEvent) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, event.Line())
}

func (l *auditLog) taken() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.lines...)
}

// sentinelSession is a granted developer session over a configuration, an mcp
// pool and a hook directory that all carry secrets, with a memory holding one
// more. It returns the session, the call the model makes, and the audit record
// the calls leave.
func sentinelSession(t *testing.T) (*Controller, func(string) (tooldef.Result, error), *auditLog) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COZYPHI_MODEL", "")
	t.Setenv("COZYPHI_API_KEY", "sentinel-env-key")
	t.Setenv("COZYPHI_BASE_URL", "")
	t.Setenv("COZYPHI_MCP", "")
	t.Setenv("COZYPHI_HOOKS", "")

	cwd, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	proj, err := project.Discover(cwd)
	require.NoError(t, err)

	writeConfig(t, proj.Global().ConfigFile(), sentinelConfigYAML)
	writeConfig(t, filepath.Join(home, ".cozyphi", "mcp.json"), sentinelMCPConfig)
	writeHooks(t, proj.Global().HooksDir(), filepath.Join(t.TempDir(), "SENTINEL-hook-ran"), sentinelHooks)
	writeConfig(t, filepath.Join(proj.MemoryDir(), "release-freeze.md"),
		"the freeze window is guarded by sentinel-memory-body\n")

	rt, err := NewRuntime(proj)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rt.Close() })
	require.NoError(t, rt.GrantDeveloperMode())
	ws, err := rt.Workspace(cwd)
	require.NoError(t, err)
	c, err := rt.NewSession(NewBus(nil), ws, "", nil)
	require.NoError(t, err)

	// The sink is attached here, while the registry is being wired and before
	// it has answered anything, which is the only point it may be attached at.
	record := &auditLog{}
	c.diagnostics.WithAudit(record.sink)

	tool := harnesstool.Tool(harnesstool.Deps{Registry: c.diagnostics})
	return c, func(args string) (tooldef.Result, error) {
		return tool.Run(t.Context(), json.RawMessage(args))
	}, record
}

// everyAnswer asks the tool everything it accepts and several things it has to
// refuse, and returns what came back — answers and refusals together, because
// a refusal is a channel out of the process exactly as much as an answer is.
func everyAnswer(t *testing.T, c *Controller, call func(string) (tooldef.Result, error)) []string {
	t.Helper()
	var out []string
	ask := func(args string) {
		t.Helper()
		result, err := call(args)
		out = append(out, args, result.Content, result.Detail)
		if err != nil {
			out = append(out, err.Error())
		}
	}

	ask(`{"action":"catalog"}`)
	ask(`{"action":"snapshot"}`)
	for _, entry := range c.diagnostics.Catalog().Categories {
		ask(`{"action":"snapshot","category":"` + string(entry.Category) + `"}`)
		for _, key := range entry.Keys {
			ask(`{"action":"explain","category":"` + string(entry.Category) + `","key":"` + key + `"}`)
		}
	}

	// The refusals: a category nobody has, a key the category does not
	// declare, an explain with nothing to explain, an action that is not one,
	// and arguments that never parsed.
	ask(`{"action":"snapshot","category":"nosuch"}`)
	ask(`{"action":"explain","category":"model","key":"nosuch"}`)
	ask(`{"action":"explain","category":"model"}`)
	ask(`{"action":"nosuch"}`)
	ask(`{"action":`)
	return out
}

func TestNoSecretReachesAnyAnswerRefusalTranscriptOrRecord(t *testing.T) {
	c, call, record := sentinelSession(t)

	answers := everyAnswer(t, c, call)
	require.NotEmpty(t, answers)

	// Everything the session said now goes where a tool result goes, so the
	// file on disk is searched rather than only the value in memory.
	for _, answer := range answers {
		require.NoError(t, c.engine.Session().Append(llm.Message{Role: llm.RoleAssistant, Content: answer}))
	}
	transcript, err := os.ReadFile(c.SessionFile())
	require.NoError(t, err)

	channels := map[string][]string{
		"answers and refusals": answers,
		"transcript":           {string(transcript)},
		"audit record":         record.taken(),
	}
	for name, channel := range channels {
		for _, text := range channel {
			assert.NotContains(t, strings.ToLower(text), secretMark,
				"a secret reached the %s", name)
		}
	}
	assert.NotEmpty(t, record.taken(), "every request leaves a record, and it was searched")

	// The control: a search finds nothing when nothing was loaded either, so
	// the plain half of each secret-bearing subject has to be in the answers.
	// Every one of these sits beside a value spelled with the mark.
	joined := strings.Join(answers, "\n")
	for _, planted := range []string{"alpha", "autopilot", "vault", "guard-bash", "whisper-1"} {
		assert.Contains(t, joined, planted,
			"the harness never reported %s, so the secret beside it was never at risk", planted)
	}
}

// AC 5, asked of the whole catalog rather than of one collector: sweeping
// every category writes nothing, starts nothing and reads nothing back into
// the session. The spy is the filesystem itself — a hook that ran, a server
// that started, a store that was rewritten and a log that was opened all leave
// a file behind, and none of them may.
func TestObservingTheWholeHarnessWritesNothingAndStartsNothing(t *testing.T) {
	c, call, _ := sentinelSession(t)
	roots := []string{os.Getenv("HOME"), c.cwd, c.SessionDir()}

	before := fingerprint(t, roots)
	require.NotEmpty(t, before, "the trees the spy watches are the ones the session actually uses")
	tools := c.engineRef.Load().ToolNames()
	plan := c.engineRef.Load().Plan()
	catalog := c.diagnostics.Catalog()

	everyAnswer(t, c, call)
	everyAnswer(t, c, call)

	assert.Equal(t, before, fingerprint(t, roots),
		"observing the harness left something on disk")
	assert.Equal(t, tools, c.engineRef.Load().ToolNames(), "an observation registers nothing")
	assert.Equal(t, plan, c.engineRef.Load().Plan(), "and moves no plan step")
	assert.Equal(t, catalog, c.diagnostics.Catalog(), "and answers the same catalog afterwards")

	result, err := call(`{"action":"snapshot","category":"integrations"}`)
	require.NoError(t, err)
	assert.Contains(t, result.Content, "hooks.registered",
		"the hook was loaded, so not running it is a fact about this answer rather than about an empty pool")
}

// fingerprint is what the trees hold: every path with its size, and nothing
// about the contents, which is enough to see a file appear, grow or vanish.
func fingerprint(t *testing.T, roots []string) map[string]int64 {
	t.Helper()
	sizes := map[string]int64{}
	for _, root := range roots {
		if root == "" {
			continue
		}
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, statErr := entry.Info()
			if statErr != nil {
				return statErr
			}
			size := int64(-1)
			if !entry.IsDir() {
				size = info.Size()
			}
			sizes[path] = size
			return nil
		})
		require.NoError(t, err)
	}
	return sizes
}

// AC 6 over the real eleven collectors rather than over fixtures: several
// sessions of one process ask the whole catalog at once, and each answer has
// to be that session's own. Run under -race this is also where a collector
// that reads a session's state without holding it would show up.
func TestSeveralSessionsSweepTheCatalogAtOnceAndEachAnswersForItself(t *testing.T) {
	rt, ws := developerRuntime(t, true)
	sessions := make([]*Controller, 0, 3)
	for range 3 {
		c, err := rt.NewSession(NewBus(nil), ws, "", nil)
		require.NoError(t, err)
		sessions = append(sessions, c)
	}

	var wg sync.WaitGroup
	for _, c := range sessions {
		for range 4 {
			wg.Go(func() {
				overview, err := c.diagnostics.Snapshot(t.Context(), "")
				assert.NoError(t, err)
				assert.Len(t, overview.Categories, len(diag.Categories()))
				for _, category := range diag.Categories() {
					_, err := c.diagnostics.Snapshot(t.Context(), category)
					assert.NoError(t, err)
				}
				explained, err := c.diagnostics.Explain(t.Context(), diag.CategoryRuntime, diag.KeySessionID)
				assert.NoError(t, err)
				assert.Equal(t, c.SessionID(), explained.Field.Effective.Value.Str,
					"a session answered with another session's identity")
			})
		}
	}
	wg.Wait()
}

// A caller that stopped waiting gets its own error back from the real wiring,
// and gets it without any owner being asked anything.
func TestACallerThatStoppedWaitingIsToldSoByTheRealWiring(t *testing.T) {
	c := developerToolRuntime(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := c.diagnostics.Snapshot(ctx, "")
	require.ErrorIs(t, err, context.Canceled)
	_, err = c.diagnostics.Snapshot(ctx, diag.CategoryModel)
	require.ErrorIs(t, err, context.Canceled)
	_, err = c.diagnostics.Explain(ctx, diag.CategoryRuntime, diag.KeySessionID)
	require.ErrorIs(t, err, context.Canceled)
}
