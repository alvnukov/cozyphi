package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGitBranchOutsideRepo(t *testing.T) {
	assert.Empty(t, gitBranch(t.Context(), t.TempDir()), "non-git dir has no branch")
}

func TestPathWithBranchOutsideRepo(t *testing.T) {
	plain := t.TempDir()
	assert.Equal(t, shortPath(plain), pathWithBranch(plain), "no branch suffix outside git")
}
