package skills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// writeSkill plants dir/<sub>/SKILL.md with the given frontmatter name.
func writeSkill(t *testing.T, dir, sub, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, sub, SkillFileName)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	content := "---\nname: " + name + "\ndescription: " + name + " skill\n---\n" + body
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func names(list []*Skill) []string {
	out := make([]string, 0, len(list))
	for _, s := range list {
		out = append(out, s.Name)
	}
	return out
}

func TestSourcesNamespacePluginSkills(t *testing.T) {
	user, plugin := t.TempDir(), t.TempDir()
	writeSkill(t, user, "brainstorming", "brainstorming", "user body")
	writeSkill(t, plugin, "brainstorming", "brainstorming", "plugin body")

	list, err := Sources{{Dir: user}, {Dir: plugin, Namespace: "superpowers"}}.Load()
	require.NoError(t, err)
	require.Equal(t, []string{"brainstorming", "superpowers:brainstorming"}, names(list))
	require.Equal(t, filepath.Join(plugin, "brainstorming"), list[1].Path)
}

func TestSourcesExpandVarsInBodiesOnly(t *testing.T) {
	dir := t.TempDir()
	file := writeSkill(t, dir, "tool", "tool", "run ${CLAUDE_PLUGIN_ROOT}/bin/x and ${UNKNOWN}")

	list, err := Sources{{
		Dir:       dir,
		Namespace: "p",
		Vars:      map[string]string{"CLAUDE_PLUGIN_ROOT": "/opt/p"},
	}}.Load()
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "run /opt/p/bin/x and ${UNKNOWN}", list[0].Body)

	raw, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Contains(t, string(raw), "${CLAUDE_PLUGIN_ROOT}", "the file the model reads stays raw")
}

func TestSourcesFollowSymlinksAndSurviveCycles(t *testing.T) {
	root, target := t.TempDir(), t.TempDir()
	writeSkill(t, target, "linked", "linked", "via symlink")
	writeSkill(t, root, "plain", "plain", "plain")
	require.NoError(t, os.Symlink(target, filepath.Join(root, "link")))
	require.NoError(t, os.Symlink(root, filepath.Join(root, "plain", "loop")))
	require.NoError(t, os.Symlink(filepath.Join(root, "missing"), filepath.Join(root, "dangling")))

	list, err := Sources{{Dir: root}}.Load()
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"linked", "plain"}, names(list))
}

func TestSourcesLoadSymlinkedSkillFile(t *testing.T) {
	root, store := t.TempDir(), t.TempDir()
	file := writeSkill(t, store, "kept", "kept", "linked file")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "kept"), 0o755))
	require.NoError(t, os.Symlink(file, filepath.Join(root, "kept", SkillFileName)))

	list, err := Sources{{Dir: root}}.Load()
	require.NoError(t, err)
	require.Equal(t, []string{"kept"}, names(list), "a SKILL.md linked from a dotfiles store still loads")
}

func TestSourcesDeduplicateTheSameFile(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "one", "one", "")
	link := filepath.Join(t.TempDir(), "alias")
	require.NoError(t, os.Symlink(dir, link))

	list, err := Sources{{Dir: dir}, {Dir: link, Namespace: "p"}}.Load()
	require.NoError(t, err)
	require.Equal(t, []string{"one"}, names(list), "the first source to reach a file owns it")
}

func TestSourcesReturnPartialListWithError(t *testing.T) {
	good := t.TempDir()
	writeSkill(t, good, "ok", "ok", "")
	file := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(file, nil, 0o600))

	list, err := Sources{{Dir: file}, {Dir: good}, {Dir: filepath.Join(good, "absent")}, {}}.Load()
	require.ErrorContains(t, err, "is not a directory")
	require.Equal(t, []string{"ok"}, names(list))
}

func TestSourcesString(t *testing.T) {
	require.Equal(t, "(no skill directories)", Sources{}.String())
	require.Equal(t, "/a, /b", Sources{{Dir: "/a"}, {}, {Dir: "/b"}}.String())
	require.False(t, Sources{{Dir: "/a"}}.Namespaced())
	require.True(t, Sources{{Dir: "/a"}, {Dir: "/b", Namespace: "p"}}.Namespaced())
}

func TestFindPrefersExactThenRefusesAmbiguousBareName(t *testing.T) {
	user, a, b := t.TempDir(), t.TempDir(), t.TempDir()
	writeSkill(t, user, "brainstorming", "brainstorming", "")
	writeSkill(t, a, "tdd", "tdd", "")
	writeSkill(t, b, "tdd", "tdd", "")
	writeSkill(t, a, "only-here", "only-here", "")
	list, err := Sources{{Dir: user}, {Dir: a, Namespace: "alpha"}, {Dir: b, Namespace: "beta"}}.Load()
	require.NoError(t, err)

	got, err := Find(list, "brainstorming")
	require.NoError(t, err)
	require.Equal(t, "brainstorming", got.Name)

	got, err = Find(list, "ALPHA:TDD")
	require.NoError(t, err)
	require.Equal(t, "alpha:tdd", got.Name)

	got, err = Find(list, "only-here")
	require.NoError(t, err)
	require.Equal(t, "alpha:only-here", got.Name)

	_, err = Find(list, "tdd")
	require.ErrorContains(t, err, `skill "tdd" is ambiguous — use one of: alpha:tdd, beta:tdd`)

	got, err = Find(list, "nope")
	require.NoError(t, err)
	require.Nil(t, got)

	got, err = Find(list, "  ")
	require.NoError(t, err)
	require.Nil(t, got)
}
