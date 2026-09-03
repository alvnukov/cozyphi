package controller

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/alvnukov/cozyphi/internal/llm"
)

// The classification itself is covered in internal/runerror; what this
// surface owns is the remedy it offers and the text built around it.
func TestRunErrorTextPointsAtTheSlashCommands(t *testing.T) {
	assert.Contains(t,
		runErrorText(llm.APIError("anthropic", 401, []byte("invalid x-api-key"))),
		"/connect")
	assert.Contains(t,
		runErrorText(fmt.Errorf("anthropic: %w", llm.ErrContextOverflow)),
		"/compact")
}

func TestRunErrorTextKeepsTheRawErrorAndNamesTheRetryPath(t *testing.T) {
	text := runErrorText(llm.APIError("anthropic", 401, []byte("invalid x-api-key")))

	assert.True(t, strings.HasPrefix(text, "The provider rejected the credentials"), text)
	assert.Contains(t, text, "anthropic: (401) invalid x-api-key")
	assert.Contains(t, text, "↑")
}

func TestRunErrorTextFallsBackToAPlainHeadline(t *testing.T) {
	text := runErrorText(errors.New("stream ended unexpectedly"))

	assert.True(t, strings.HasPrefix(text, "The run failed."), text)
	assert.Contains(t, text, "stream ended unexpectedly")
}
