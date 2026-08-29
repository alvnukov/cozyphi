package settings

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSkillsLinesWrapsUnderIndent: the enumeration indents continuations under
// the names, so a long catalog reads as one block on the plan tab.
func TestSkillsLinesWrapsUnderIndent(t *testing.T) {
	assert.Equal(
		t,
		[]string{"skills: aaaaaaaaaa,", "        bbbbbbbbbb, cccccccccc"},
		skillsLines([]string{"aaaaaaaaaa", "bbbbbbbbbb", "cccccccccc"}, 30),
		"continuation lines align under the enumeration",
	)
	assert.Equal(t, []string{"skills: none"}, skillsLines(nil, 30), "an empty catalog says none")
}
