package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.Len(t, all, 2)

	resu := FilterSlashCommands("resu")
	require.Len(t, resu, 1)
	assert.Equal(t, "resume", resu[0].Path)
	assert.Contains(t, resu[0].Description, "Resume")

	none := FilterSlashCommands("zzz")
	assert.Empty(t, none)

	assert.Equal(t, "/resume ", LookupSlashInsert("resume"))
	assert.Equal(t, "/sessions", LookupSlashInsert("sessions"))
}
