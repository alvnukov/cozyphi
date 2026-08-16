package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/components/palette"
	"github.com/pulseaiclub/phi/internal/hooks"
)

func TestThemeCommand_Submenu(t *testing.T) {
	var got string
	cmd := ThemeCommand(func(name string) { got = name })
	assert.Equal(t, "settings", cmd.Noun)
	assert.Equal(t, "theme", cmd.Verb)
	assert.Equal(t, "Select Theme", cmd.SubmenuTitle)
	require.Len(t, cmd.Submenu, 4)
	assert.Equal(t, "Dark (builtin)", cmd.Submenu[0].Verb)
	assert.Equal(t, "Pink (builtin)", cmd.Submenu[2].Verb)

	cmd.Submenu[2].Run()
	assert.Equal(t, "Pink", got)
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
	pal := &palette.CommandPalette{}
	pal.Show()
	cmd := HooksCommand(pal, func() []palette.PaletteCommand {
		return []palette.PaletteCommand{{
			ID:       "hook-demo",
			Verb:     "demo  pre_tool  match=bash  [project]",
			Disabled: true,
		}}
	}, func() { reloaded = true })

	assert.Equal(t, "hooks", cmd.Noun)
	assert.Equal(t, "manage", cmd.Verb)
	require.Len(t, cmd.Submenu, 2)

	// Simulate opening the hooks submenu, then list.
	pal.Push(cmd.SubmenuTitle, cmd.Submenu)
	cmd.Submenu[0].Run() // list → Push
	require.NotEmpty(t, pal.Commands)
	assert.Equal(t, "hook-demo", pal.Commands[0].ID)

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
	require.Len(t, all, 3)

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
