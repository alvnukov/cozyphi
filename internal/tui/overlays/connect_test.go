package overlays

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/provider"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

func TestConnectOverlayCapturesPasteMasksAndClearsSecret(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	item := provider.Info{
		ID:       "openai",
		Name:     "OpenAI",
		BaseURL:  "https://api.openai.com/v1",
		Protocol: llm.ProtocolOpenAI,
		Models:   []provider.Model{{ID: "gpt-5"}},
	}
	var request provider.ConnectRequest
	o.BeginConnect([]provider.Info{item}, func(req provider.ConnectRequest) {
		request = req
	}, nil, nil)
	ctx := &components.EventContext{}

	require.True(t, o.HandleConnectEvent(ctx, xui.KeyEvent{Press: true, Code: xui.KeyEnter}))
	require.NotNil(t, o.connect)
	assert.Equal(t, connectSecret, o.connect.phase)

	const secret = "sk-test-never-render"
	require.True(t, o.HandleConnectEvent(ctx, xui.PasteEvent{Text: secret}))
	surface := o.drawConnect(components.DrawContext{Method: xui.WidthUnicode}, 80, 10)
	var rendered strings.Builder
	for _, cell := range surface.Buffer {
		rendered.WriteString(cell.Char)
	}
	assert.NotContains(t, rendered.String(), secret)
	assert.Contains(t, rendered.String(), "Endpoint: https://api.openai.com/v1")

	require.True(t, o.HandleConnectEvent(ctx, xui.KeyEvent{Press: true, Code: xui.KeyEnter}))
	assert.Equal(t, secret, request.APIKey)
	assert.Equal(t, "openai", request.ProviderID)
	require.NotNil(t, o.connect)
	assert.Equal(t, connectSaving, o.connect.phase)
	assert.Empty(t, o.connect.key, "overlay state must release the secret immediately after submit")

	o.Apply(controller.ProviderConnectResultMsg{ProviderID: "openai"})
	assert.Nil(t, o.connect)
}

func TestConnectOverlayShowsDeviceCodeAndCancels(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	item := provider.Info{
		ID: "codex", Name: "Codex", BaseURL: "https://chatgpt.com/backend-api/codex",
		Protocol: llm.ProtocolOpenAIResponses, Auth: provider.AuthOAuthDevice,
		Models: []provider.Model{{ID: "gpt-5.4"}},
	}
	var authorized, canceled bool
	o.BeginConnect([]provider.Info{item}, nil, func(got provider.Info) {
		authorized = got.ID == "codex"
	}, func() {
		canceled = true
	})
	ctx := &components.EventContext{}
	o.HandleConnectEvent(ctx, xui.KeyEvent{Press: true, Code: xui.KeyEnter})
	require.True(t, authorized)
	require.Equal(t, connectSaving, o.connect.phase)

	o.Apply(controller.ProviderDeviceCodeMsg{
		ProviderID: "codex", VerificationURL: "https://auth.openai.com/codex/device", UserCode: "ABCD-EFGH",
	})
	surface := o.drawConnect(components.DrawContext{Method: xui.WidthUnicode}, 90, 12)
	var rendered strings.Builder
	for _, cell := range surface.Buffer {
		rendered.WriteString(cell.Char)
	}
	assert.Contains(t, rendered.String(), "https://auth.openai.com/codex/device")
	assert.Contains(t, rendered.String(), "ABCD-EFGH")

	o.HandleConnectEvent(ctx, xui.KeyEvent{Press: true, Code: xui.KeyEscape})
	assert.True(t, canceled)
	assert.Nil(t, o.connect)
}

func TestConnectOverlayEscapeWipesSecretBuffer(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	o.BeginConnect([]provider.Info{{
		ID:       "anthropic",
		Name:     "Anthropic",
		BaseURL:  "https://api.anthropic.com",
		Protocol: llm.ProtocolAnthropic,
		Models:   []provider.Model{{ID: "claude"}},
	}}, func(provider.ConnectRequest) {}, nil, nil)
	ctx := &components.EventContext{}
	o.HandleConnectEvent(ctx, xui.KeyEvent{Press: true, Code: xui.KeyEnter})
	o.HandleConnectEvent(ctx, xui.PasteEvent{Text: "secret"})
	require.NotNil(t, o.connect)
	backing := o.connect.key

	o.HandleConnectEvent(ctx, xui.KeyEvent{Press: true, Code: xui.KeyEscape})

	assert.Nil(t, o.connect)
	assert.Equal(t, make([]byte, len(backing)), backing)
}

func TestConnectOverlayPasteNeverFallsThrough(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	o.BeginConnect(nil, nil, nil, nil)

	assert.True(t, o.HandleConnectEvent(&components.EventContext{}, xui.PasteEvent{Text: "typed into filter"}))
	require.NotNil(t, o.connect)
	assert.Equal(t, "typed into filter", o.connect.query.Value)
}

func TestConnectFilterEditsInPlace(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	o.BeginConnect(nil, nil, nil, nil)
	ctx := &components.EventContext{}
	for _, r := range "opus" {
		o.HandleConnectEvent(ctx, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: r})
	}
	// The filter is a real editor now: the caret moves and deletes mid-word.
	o.HandleConnectEvent(ctx, xui.KeyEvent{Press: true, Code: xui.KeyHome})
	o.HandleConnectEvent(ctx, xui.KeyEvent{Press: true, Code: xui.KeyDelete})
	require.NotNil(t, o.connect)
	assert.Equal(t, "pus", o.connect.query.Value)
}
