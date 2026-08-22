package shell

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/tui/composer"
	"github.com/pulseaiclub/phi/internal/tui/controller"
)

func TestResolveGitDir(t *testing.T) {
	dir := t.TempDir()
	assert.Equal(t, filepath.Join(dir, ".git"), resolveGitDir(dir))

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: ../modules/sub\n"), 0o644))
	assert.Equal(t, filepath.Join(filepath.Dir(dir), "modules", "sub"), resolveGitDir(dir))

	abs := filepath.Join(t.TempDir(), "modules", "sub")
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: "+abs+"\n"), 0o644))
	assert.Equal(t, abs, resolveGitDir(dir))
}

func TestBranchState(t *testing.T) {
	dir := t.TempDir()
	assert.Equal(t, "missing", branchState(dir))

	gitDir := filepath.Join(dir, ".git")
	require.NoError(t, os.Mkdir(gitDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644))
	assert.Equal(t, "ref: refs/heads/main", branchState(dir))

	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("abc123\n"), 0o644))
	assert.Equal(t, "abc123", branchState(dir))
}

func TestShellAppliesBranchLabel(t *testing.T) {
	sh := &Shell{composer: composer.NewComposerPane(components.DefaultTheme(), "m", "/tmp")}
	sh.composer.Wire(composer.ComposerWire{})
	sh.composer.Chat.BottomRightLabel.Text = "~ (old)"
	sh.Update(controller.BranchLabelMsg{Text: "~ (new)"})
	assert.Equal(t, "~ (new)", sh.composer.Chat.BottomRightLabel.Text)
}
