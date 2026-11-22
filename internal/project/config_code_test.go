package project

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCodeModeConfig(t *testing.T) {
	p := discoverInTempHome(t)
	writeTestConfigBody(
		t,
		p,
		"code_mode:\n  enabled: true\n  advisor_model: helper\n  discovery: conditional\n  advisor_timeout: 650ms\n",
	)
	require.NoError(t, p.LoadConfig())
	require.Equal(
		t,
		CodeModeConfig{
			Enabled:        true,
			AdvisorModel:   "helper",
			Discovery:      "conditional",
			AdvisorTimeout: 650 * time.Millisecond,
		},
		p.Config().CodeMode,
	)
	for _, invalid := range []string{"advisor_timeout: -1ms", "discovery: enforced"} {
		writeTestConfigBody(t, p, "code_mode:\n  "+invalid+"\n")
		require.ErrorContains(t, p.LoadConfig(), "code_mode.")
	}
}
