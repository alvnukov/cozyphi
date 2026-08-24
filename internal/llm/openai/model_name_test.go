package openai

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/pulseaiclub/phi/internal/llm"
)

func TestBuildRequestUsesWireModelName(t *testing.T) {
	t.Parallel()

	req := BuildRequest(llm.ModelConfig{Name: "acme/chat", APIName: "chat"}, "", nil, nil)
	assert.Equal(t, "chat", req.Model)
}
