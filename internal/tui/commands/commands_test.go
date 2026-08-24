package commands

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/palette"
	"github.com/pulseaiclub/phi/internal/components/toast"
	"github.com/pulseaiclub/phi/internal/hooks"
)

type fakeHost struct {
	toastMsg   string
	sessions   int
	resumeID   string
	cleared    int
	model      string
	modelNames []string
	pushed     bool
	listHooks  []palette.PaletteCommand
	skillPath  string
	addSkill   string
	copied     bool
	exports    int
	exportPath string
	compacted  int
	connected  int
	theme      string
	bypass     *bool
	agents     *bool
	reloaded   bool
}

func (f *fakeHost) Toast(msg string, _ toast.ToastKind, _ time.Duration) { f.toastMsg = msg }
func (f *fakeHost) PushSubmenu(_ string, _ []palette.PaletteCommand)     { f.pushed = true }
func (f *fakeHost) ShowSessions()                                        { f.sessions++ }
func (f *fakeHost) ResumeSession(id string)                              { f.resumeID = id }
func (f *fakeHost) ClearSession()                                        { f.cleared++ }
func (f *fakeHost) SetModel(name string)                                 { f.model = name }
func (f *fakeHost) ApplyTheme(name string)                               { f.theme = name }
func (f *fakeHost) SetPermissions(v bool)                                { f.bypass = &v }
func (f *fakeHost) SetAgents(v bool)                                     { f.agents = &v }
func (f *fakeHost) ReloadHooks()                                         { f.reloaded = true }
func (f *fakeHost) ListHooks() []palette.PaletteCommand                  { return f.listHooks }
func (f *fakeHost) AddSkill(name string)                                 { f.addSkill = name }
func (f *fakeHost) CopyLastMessage()                                     { f.copied = true }
func (f *fakeHost) ExportSession(path string)                            { f.exports++; f.exportPath = path }
func (f *fakeHost) RunCompact()                                          { f.compacted++ }
func (f *fakeHost) ConnectProvider()                                     { f.connected++ }
func (f *fakeHost) ModelNames() []string                                 { return f.modelNames }
func (f *fakeHost) SkillPath() string                                    { return f.skillPath }

func TestThemeCommand_Submenu(t *testing.T) {
	var got string
	cmd := ThemeCommand(func(name string) { got = name })
	assert.Equal(t, "settings", cmd.Noun)
	assert.Equal(t, "theme", cmd.Verb)
	assert.Equal(t, "Select Theme", cmd.SubmenuTitle)
	names := components.ThemeNames()
	require.Len(t, cmd.Submenu, len(names), "submenu must list every builtin theme")
	for i, name := range names {
		assert.Equal(t, name+" (builtin)", cmd.Submenu[i].Verb)
	}

	cmd.Submenu[0].Run()
	assert.Equal(t, names[0], got)
}

func TestPermissionsCommand_Toggle(t *testing.T) {
	var bypass *bool
	cmd := PermissionsCommand(func(v bool) { bypass = &v })
	assert.Equal(t, "settings", cmd.Noun)
	assert.Equal(t, "permissions", cmd.Verb)
	require.Len(t, cmd.Submenu, 2)

	cmd.Submenu[0].Run()
	require.NotNil(t, bypass)
	assert.True(t, *bypass)

	cmd.Submenu[1].Run()
	assert.False(t, *bypass)
}

func TestAgentsCommand_Toggle(t *testing.T) {
	var enabled *bool
	cmd := AgentsCommand(func(v bool) { enabled = &v })
	assert.Equal(t, "settings", cmd.Noun)
	assert.Equal(t, "agents", cmd.Verb)
	require.Len(t, cmd.Submenu, 2)

	cmd.Submenu[0].Run()
	require.NotNil(t, enabled)
	assert.True(t, *enabled)

	cmd.Submenu[1].Run()
	assert.False(t, *enabled)
}

func TestHooksCommand_ListAndReload(t *testing.T) {
	var reloaded bool
	var pushedTitle string
	var pushed []palette.PaletteCommand
	cmd := HooksCommand(func() []palette.PaletteCommand {
		return []palette.PaletteCommand{{
			ID:       "hook-demo",
			Verb:     "demo  pre_tool  match=bash  [project]",
			Disabled: true,
		}}
	}, func() { reloaded = true }, func(title string, cmds []palette.PaletteCommand) {
		pushedTitle = title
		pushed = cmds
	})

	assert.Equal(t, "hooks", cmd.Noun)
	assert.Equal(t, "manage", cmd.Verb)
	require.Len(t, cmd.Submenu, 2)

	cmd.Submenu[0].Run() // list → PushSubmenu
	assert.Equal(t, "Hooks on disk", pushedTitle)
	require.NotEmpty(t, pushed)
	assert.Equal(t, "hook-demo", pushed[0].ID)

	cmd.Submenu[1].Run() // reload
	assert.True(t, reloaded)
}

func TestHookListEntries(t *testing.T) {
	entries := HookListEntries(nil, nil, nil)
	require.Len(t, entries, 1)
	assert.True(t, entries[0].Disabled)
	assert.Contains(t, entries[0].Verb, "No hooks")

	entries = HookListEntries(nil, []hooks.Warning{{Path: "x", Message: "bad"}}, nil)
	require.Len(t, entries, 1)
	assert.Contains(t, entries[0].Verb, "warn:")
}

func TestSkillsCommand_SubmenuFromDisk(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "extract-and-distill")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	content := `---
name: extract-and-distill
description: Distill ideas from source material
---
Do the work.
`
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644))

	var got string
	cmd := SkillsCommand(dir, func(name string) { got = name })
	assert.Equal(t, "skills", cmd.Noun)
	assert.Equal(t, "invoke", cmd.Verb)
	require.Len(t, cmd.Submenu, 1)
	assert.Equal(t, "extract-and-distill", cmd.Submenu[0].Verb)

	cmd.Submenu[0].Run()
	assert.Equal(t, "extract-and-distill", got)
}

func TestSkillsCommand_Empty(t *testing.T) {
	cmd := SkillsCommand(t.TempDir(), nil)
	require.Len(t, cmd.Submenu, 1)
	assert.True(t, cmd.Submenu[0].Disabled)
}

func TestFilterSlashCommands(t *testing.T) {
	all := FilterSlashCommands("")
	require.Len(t, all, 8)

	resu := FilterSlashCommands("resu")
	require.Len(t, resu, 1)
	assert.Equal(t, "resume", resu[0].Path)
	assert.Contains(t, resu[0].Description, "Resume")

	clr := FilterSlashCommands("cle")
	require.Len(t, clr, 1)
	assert.Equal(t, "clear", clr[0].Path)

	none := FilterSlashCommands("zzz")
	assert.Empty(t, none)

	assert.Equal(t, "/resume ", LookupSlashInsert("resume"))
	assert.Equal(t, "/sessions", LookupSlashInsert("sessions"))
	assert.Equal(t, "/clear", LookupSlashInsert("clear"))
}

func TestCommandRegistry_DispatchSlash(t *testing.T) {
	r := NewBuiltinRegistry()
	host := &fakeHost{}
	ctx := CommandContext{Host: host}

	assert.True(t, r.DispatchSlash("/sessions", ctx))
	assert.Equal(t, 1, host.sessions)

	assert.True(t, r.DispatchSlash("/resume abc", ctx))
	assert.Equal(t, "abc", host.resumeID)

	assert.True(t, r.DispatchSlash("/resume", ctx))
	assert.Contains(t, host.toastMsg, "Usage:")

	assert.True(t, r.DispatchSlash("/clear", ctx))
	assert.Equal(t, 1, host.cleared)

	assert.True(t, r.DispatchSlash("/connect", ctx))
	assert.Equal(t, 1, host.connected)

	assert.False(t, r.DispatchSlash("/unknown", ctx))
	assert.False(t, r.DispatchSlash("not-slash", ctx))
}

func TestCommandRegistry_BuildPalette(t *testing.T) {
	r := NewBuiltinRegistry()
	host := &fakeHost{
		modelNames: []string{"gpt"},
		listHooks:  []palette.PaletteCommand{{ID: "hook-x", Verb: "x", Disabled: true}},
	}
	cmds := r.BuildPalette(CommandContext{Host: host})
	require.GreaterOrEqual(t, len(cmds), 6)

	// settings → model → gpt
	require.NotEmpty(t, cmds[0].Submenu)
	cmds[0].Submenu[0].Run()
	assert.Equal(t, "gpt", host.model)

	// hooks → list uses PushSubmenu, not *palette
	var hooksCmd palette.PaletteCommand
	for _, c := range cmds {
		if c.ID == "hooks" {
			hooksCmd = c
			break
		}
	}
	require.Equal(t, "hooks", hooksCmd.ID)
	hooksCmd.Submenu[0].Run()
	assert.True(t, host.pushed)
}

func TestCommandRegistry_BuildPaletteWithoutHost(t *testing.T) {
	// No host bound: every root still builds (its callbacks no-op) instead of
	// panicking on a nil interface — the guard lives in hostFn, not per builder.
	cmds := NewBuiltinRegistry().BuildPalette(CommandContext{})
	require.NotEmpty(t, cmds)
	for _, c := range cmds {
		assert.NotEmpty(t, c.ID)
	}
}

func TestCommandRegistry_RegisterReplace(t *testing.T) {
	r := NewCommandRegistry()
	r.Register(Command{
		Name:  "foo",
		Slash: true,
		Run:   func(CommandContext) error { return nil },
	})
	r.Register(Command{
		Name:        "foo",
		Description: "replaced",
		Slash:       true,
		Insert:      "/foo ",
		Run:         func(CommandContext) error { return nil },
	})
	assert.Equal(t, "/foo ", r.LookupInsert("foo"))
	assert.Equal(t, "replaced", r.SlashCommands()[0].Description)
}

func TestCommandRegistry_HookCommandsDoNotReplaceBuiltins(t *testing.T) {
	r := NewBuiltinRegistry()
	assert.False(t, r.registerHook(Command{Name: "clear", Slash: true, Insert: "/hijack"}))
	assert.Equal(t, "/clear", r.LookupInsert("clear"))

	assert.True(t, r.registerHook(Command{Name: "review", Slash: true, Insert: "/review"}))
	assert.Equal(t, "/review", r.LookupInsert("review"))
	r.clearHookCommands()
	assert.Empty(t, r.LookupInsert("review"))
	assert.Equal(t, "/clear", r.LookupInsert("clear"))
}
