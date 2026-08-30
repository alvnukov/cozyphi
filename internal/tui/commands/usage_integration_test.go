package commands

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components/mention"
	"github.com/alvnukov/cozyphi/internal/components/palette"
	"github.com/alvnukov/cozyphi/internal/usage"
)

func TestCommandRegistryRanksSlashCommandsAfterSuccessfulDispatch(t *testing.T) {
	history, err := usage.Open("")
	require.NoError(t, err)
	registry := NewCommandRegistry(history)
	registry.Register(Command{Name: "first", Slash: true, Run: func(CommandContext) error { return nil }})
	registry.Register(Command{Name: "second", Slash: true, Run: func(CommandContext) error { return nil }})
	registry.Register(
		Command{Name: "failed", Slash: true, Run: func(CommandContext) error { return errors.New("nope") }},
	)

	require.True(t, registry.DispatchSlash("/failed", CommandContext{}))
	assert.Equal(t, []string{"first", "second", "failed"}, slashPaths(registry.FilterSlash("")))

	require.True(t, registry.DispatchSlash("/second", CommandContext{}))
	assert.Equal(t, []string{"second", "first", "failed"}, slashPaths(registry.FilterSlash("")))
}

func TestCommandRegistryRanksPaletteRowsIncludingParents(t *testing.T) {
	history, err := usage.Open("")
	require.NoError(t, err)
	registry := NewCommandRegistry(history)
	registry.Register(Command{Name: "first", PaletteRoot: func(CommandContext) palette.PaletteCommand {
		return palette.PaletteCommand{ID: "first", Run: func() {}}
	}})
	registry.Register(Command{Name: "submenu", PaletteRoot: func(CommandContext) palette.PaletteCommand {
		return palette.PaletteCommand{ID: "submenu", Submenu: []palette.PaletteCommand{{ID: "child", Run: func() {}}}}
	}})
	registry.Register(Command{Name: "second", PaletteRoot: func(CommandContext) palette.PaletteCommand {
		return palette.PaletteCommand{ID: "second", Run: func() {}}
	}})

	commands := registry.BuildPalette(CommandContext{})
	commands[2].Run() // accepting the "second" leaf
	commands = registry.BuildPalette(CommandContext{})
	assert.Equal(t, []string{"second", "first", "submenu"}, paletteIDs(commands))

	// Accepting a submenu child credits the parent row, not the child: the
	// parent floats on the next build, unused rows keep registration order.
	submenu := findPaletteCommand(t, commands, "submenu")
	submenu.Submenu[0].Run()
	commands = registry.BuildPalette(CommandContext{})
	assert.Equal(t, []string{"submenu", "second", "first"}, paletteIDs(commands))
}

func TestModelSettingsCommandRanksAndRecordsSuccessfulModels(t *testing.T) {
	history, err := usage.Open("")
	require.NoError(t, err)
	require.NoError(t, history.Record(usage.Models, "beta"))

	cmd := modelSettingsCommand(func(string) error { return nil }, []string{"alpha", "beta"}, history)
	require.Len(t, cmd.Submenu, 2)
	assert.Equal(t, "beta", cmd.Submenu[0].Verb)

	fresh, err := usage.Open("")
	require.NoError(t, err)
	failed := modelSettingsCommand(func(string) error { return errors.New("nope") }, []string{"alpha", "beta"}, fresh)
	failed.Submenu[1].Run()
	assert.Equal(
		t,
		[]string{"alpha", "beta"},
		usage.Rank(fresh, usage.Models, []string{"alpha", "beta"}, func(item string) string { return item }),
	)

	succeeded := modelSettingsCommand(func(string) error { return nil }, []string{"alpha", "beta"}, fresh)
	succeeded.Submenu[1].Run()
	assert.Equal(
		t,
		[]string{"beta", "alpha"},
		usage.Rank(fresh, usage.Models, []string{"alpha", "beta"}, func(item string) string { return item }),
	)
}

func TestSkillsCommandRefreshesRankingAfterAppliedPrompt(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "first")
	writeSkill(t, dir, "second")
	history, err := usage.Open("")
	require.NoError(t, err)
	registry := NewBuiltinRegistry(history)
	host := &fakeHost{skillPath: dir}

	cmd := findPaletteCommand(t, registry.BuildPalette(CommandContext{Host: host}), "skills")
	require.Len(t, cmd.Submenu, 2)
	assert.Equal(t, []string{"first", "second"}, []string{cmd.Submenu[0].Verb, cmd.Submenu[1].Verb})

	registry.RecordSkills([]string{"second"})
	cmd = findPaletteCommand(t, registry.BuildPalette(CommandContext{Host: host}), "skills")
	require.Len(t, cmd.Submenu, 2)
	assert.Equal(t, []string{"second", "first"}, []string{cmd.Submenu[0].Verb, cmd.Submenu[1].Verb})
}

func slashPaths(items []mention.Item) []string {
	paths := make([]string, 0, len(items))
	for _, item := range items {
		paths = append(paths, item.Path)
	}
	return paths
}

func paletteIDs(commands []palette.PaletteCommand) []string {
	ids := make([]string, 0, len(commands))
	for _, command := range commands {
		ids = append(ids, command.ID)
	}
	return ids
}

func findPaletteCommand(t *testing.T, commands []palette.PaletteCommand, id string) palette.PaletteCommand {
	t.Helper()
	for _, command := range commands {
		if command.ID == id {
			return command
		}
	}
	require.FailNow(t, "palette command not found", id)
	return palette.PaletteCommand{}
}

func writeSkill(t *testing.T, root, name string) {
	t.Helper()
	dir := filepath.Join(root, name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	content := "---\nname: " + name + "\ndescription: test\n---\nUse it.\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644))
}
