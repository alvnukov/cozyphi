package anthropic

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/alvnukov/cozyphi/internal/llm"
)

func TestBuildRequestUsesWireModelName(t *testing.T) {
	t.Parallel()

	req := BuildRequest(llm.ModelConfig{Name: "acme/claude", APIName: "claude"}, "", nil, nil)
	assert.Equal(t, "claude", req.Model)
}
