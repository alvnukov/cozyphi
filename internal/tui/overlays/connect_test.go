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

func openAIWithMethods() provider.Info {
	return provider.Info{
		ID: "openai", Name: "OpenAI", BaseURL: "https://api.openai.com/v1",
		Protocol: llm.ProtocolOpenAI, Auth: provider.AuthAPIKey,
		Models: []provider.Model{{ID: "gpt-5.5"}},
		Methods: []provider.AuthMethod{
			{
				Kind: provider.AuthOAuthBrowser, Label: "ChatGPT Pro/Plus (browser)",
				BaseURL: "https://chatgpt.com/backend-api/codex", Protocol: llm.ProtocolOpenAIResponses,
			},
			{
				Kind: provider.AuthOAuthDevice, Label: "ChatGPT Pro/Plus (headless device code)",
				BaseURL: "https://chatgpt.com/backend-api/codex", Protocol: llm.ProtocolOpenAIResponses,
			},
			{
				Kind: provider.AuthAPIKey, Label: "OpenAI API key",
				BaseURL: "https://api.openai.com/v1", Protocol: llm.ProtocolOpenAI,
			},
		},
	}
}

func renderConnect(o *Overlays, width, height int) string {
	surface := o.drawConnect(components.DrawContext{Method: xui.WidthUnicode}, width, height)
	var rendered strings.Builder
	for _, cell := range surface.Buffer {
		rendered.WriteString(cell.Char)
	}
	return rendered.String()
}

func TestConnectOverlayOffersEveryOpenAISignInMethod(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	o.BeginConnect([]provider.Info{openAIWithMethods()}, nil, nil, nil)
	ctx := &components.EventContext{}

	o.HandleConnectEvent(ctx, xui.KeyEvent{Press: true, Code: xui.KeyEnter})
	require.NotNil(t, o.connect)
	require.Equal(t, connectMethod, o.connect.phase, "a provider with several methods asks which one")

	rendered := renderConnect(o, 90, 12)
	assert.Contains(t, rendered, "ChatGPT Pro/Plus (browser)")
	assert.Contains(t, rendered, "ChatGPT Pro/Plus (headless device code)")
	assert.Contains(t, rendered, "OpenAI API key")
	assert.Equal(t, 0, o.connect.methodRing.Selected(), "browser sign-in is preselected")
}

func TestConnectOverlayShowsDeviceCodeAndCancels(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	var authorized provider.AuthMethod
	var canceled bool
	o.BeginConnect([]provider.Info{openAIWithMethods()}, nil, func(got provider.Info, method provider.AuthMethod) {
		if got.ID == "openai" {
			authorized = method
		}
	}, func() {
		canceled = true
	})
	ctx := &components.EventContext{}
	o.HandleConnectEvent(ctx, xui.KeyEvent{Press: true, Code: xui.KeyEnter})
	o.HandleConnectEvent(ctx, xui.KeyEvent{Press: true, Code: xui.KeyDown})
	o.HandleConnectEvent(ctx, xui.KeyEvent{Press: true, Code: xui.KeyEnter})
	require.Equal(t, provider.AuthOAuthDevice, authorized.Kind, "the headless fallback is reachable")
	require.Equal(t, connectSaving, o.connect.phase)

	o.Apply(controller.ProviderDeviceCodeMsg{
		ProviderID: "openai", VerificationURL: "https://auth.openai.com/codex/device", UserCode: "ABCD-EFGH",
	})
	rendered := renderConnect(o, 90, 12)
	assert.Contains(t, rendered, "https://auth.openai.com/codex/device")
	assert.Contains(t, rendered, "ABCD-EFGH")

	o.HandleConnectEvent(ctx, xui.KeyEvent{Press: true, Code: xui.KeyEscape})
	assert.True(t, canceled)
	assert.Nil(t, o.connect)
}

func TestConnectOverlayAsksForAKeyOnTheOpenAIAPIEndpoint(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	var request provider.ConnectRequest
	o.BeginConnect([]provider.Info{openAIWithMethods()}, func(req provider.ConnectRequest) {
		request = req
	}, func(provider.Info, provider.AuthMethod) {
		t.Error("an API key must not start a subscription sign-in")
	}, nil)
	ctx := &components.EventContext{}

	o.HandleConnectEvent(ctx, xui.KeyEvent{Press: true, Code: xui.KeyEnter})
	o.HandleConnectEvent(ctx, xui.KeyEvent{Press: true, Code: xui.KeyUp})
	o.HandleConnectEvent(ctx, xui.KeyEvent{Press: true, Code: xui.KeyEnter})
	require.NotNil(t, o.connect)
	require.Equal(t, connectSecret, o.connect.phase)
	assert.Contains(t, renderConnect(o, 90, 10), "Endpoint: https://api.openai.com/v1")

	o.HandleConnectEvent(ctx, xui.PasteEvent{Text: "sk-openai"})
	o.HandleConnectEvent(ctx, xui.KeyEvent{Press: true, Code: xui.KeyEnter})
	assert.Equal(t, "openai", request.ProviderID)
	assert.Equal(t, "https://api.openai.com/v1", request.ExpectedBaseURL)
	assert.Equal(t, "sk-openai", request.APIKey)
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
