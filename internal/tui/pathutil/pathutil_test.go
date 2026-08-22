package pathutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGitBranchOutsideRepo(t *testing.T) {
	assert.Empty(t, GitBranch(t.Context(), t.TempDir()), "non-git dir has no branch")
}

func TestPathWithBranchOutsideRepo(t *testing.T) {
	plain := t.TempDir()
	assert.Equal(t, ShortPath(plain), PathWithBranch(plain), "no branch suffix outside git")
}
