package overlays

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/provider"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

const (
	maxConnectQueryRunes = 256
	maxConnectKeyBytes   = 64 << 10
	maxVisibleProviders  = 7
)

type connectPhase uint8

const (
	connectSelect connectPhase = iota
	connectSecret
	connectSaving
	connectOAuth
)

type connectState struct {
	providers        []provider.Info
	query            []rune
	selected         int
	phase            connectPhase
	chosen           provider.Info
	key              []byte
	errText          string
	browserErrText   string
	authorizationURL string
	verificationURL  string
	userCode         string
	submit           func(provider.ConnectRequest)
	authorize        func(provider.Info)
	cancel           func()
}

// BeginConnect opens the provider picker. Callbacks must return immediately;
// network and credential persistence belong on background goroutines.
func (o *Overlays) BeginConnect(
	items []provider.Info,
	submit func(provider.ConnectRequest),
	authorize func(provider.Info),
	cancel func(),
) {
	if o == nil {
		return
	}
	if o.perm != nil {
		o.resolvePermission(controller.AskReply{})
	}
	if o.cont != nil {
		o.resolveContinue(controller.ContinueReply{})
	}
	o.clearConnect()
	if o.composer != nil {
		o.composer.HideCompleters()
		o.composer.HidePalette()
	}
	o.connect = &connectState{
		providers: cloneProviders(items),
		submit:    submit,
		authorize: authorize,
		cancel:    cancel,
	}
	if o.focusEditor != nil {
		o.focusEditor()
	}
}

// HandleConnectEvent captures every event while the credential overlay is
// active. In particular, paste must never fall through to the chat composer.
func (o *Overlays) HandleConnectEvent(ctx *components.EventContext, ev xui.Event) bool {
	if o == nil || o.connect == nil {
		return false
	}
	switch event := ev.(type) {
	case xui.KeyEvent:
		o.handleConnectKey(event)
	case xui.PasteEvent:
		o.handleConnectPaste(event.Text)
	}
	if ctx != nil {
		ctx.ConsumeAndRedraw()
	}
	return true
}

func (o *Overlays) updateConnectCatalog(items []provider.Info, errText string) {
	if o == nil || o.connect == nil {
		return
	}
	if len(items) > 0 {
		o.connect.providers = cloneProviders(items)
		o.connect.selected = 0
	}
	o.connect.errText = errText
}

func (o *Overlays) showDeviceCode(msg controller.ProviderDeviceCodeMsg) {
	if o == nil || o.connect == nil || o.connect.chosen.ID != msg.ProviderID {
		return
	}
	if msg.ErrText != "" {
		o.connect.phase = connectOAuth
		o.connect.errText = msg.ErrText
		return
	}
	o.connect.phase = connectOAuth
	o.connect.verificationURL = msg.VerificationURL
	o.connect.userCode = msg.UserCode
	o.connect.errText = ""
}

func (o *Overlays) showAuthorization(msg controller.ProviderAuthorizationMsg) {
	if o == nil || o.connect == nil || o.connect.chosen.ID != msg.ProviderID {
		return
	}
	o.connect.phase = connectOAuth
	o.connect.authorizationURL = msg.AuthorizationURL
	o.connect.browserErrText = msg.BrowserErrText
	o.connect.errText = msg.ErrText
}

func (o *Overlays) finishConnect(providerID, errText string) {
	if o == nil || o.connect == nil {
		return
	}
	st := o.connect
	if providerID != "" && st.chosen.ID != "" && providerID != st.chosen.ID {
		return
	}
	if errText != "" {
		if st.chosen.Auth == provider.AuthOAuthBrowser || st.chosen.Auth == provider.AuthOAuthDevice {
			st.phase = connectOAuth
		} else {
			st.phase = connectSecret
		}
		st.errText = errText
		return
	}
	o.clearConnect()
	if o.focusChat != nil {
		o.focusChat()
	}
}

func (o *Overlays) clearConnect() {
	if o == nil || o.connect == nil {
		return
	}
	wipeBytes(o.connect.key)
	o.connect.key = nil
	o.connect.submit = nil
	o.connect.authorize = nil
	if o.connect.cancel != nil {
		o.connect.cancel()
		o.connect.cancel = nil
	}
	o.connect = nil
}

func (o *Overlays) handleConnectKey(event xui.KeyEvent) {
	st := o.connect
	if st == nil || !event.Press {
		return
	}
	if event.Code == xui.KeyEscape {
		o.clearConnect()
		if o.focusChat != nil {
			o.focusChat()
		}
		return
	}
	switch st.phase {
	case connectSelect:
		o.handleConnectSelectKey(st, event)
	case connectSecret:
		o.handleConnectSecretKey(st, event)
	case connectSaving, connectOAuth:
		// Authentication is immutable; Escape above still cancels and closes it.
	}
}

func (*Overlays) handleConnectSelectKey(st *connectState, event xui.KeyEvent) {
	matches := st.filtered()
	switch event.Code {
	case xui.KeyUp:
		if len(matches) == 0 {
			return
		}
		if st.selected == 0 {
			st.selected = len(matches) - 1
		} else {
			st.selected--
		}
	case xui.KeyDown, xui.KeyTab:
		if len(matches) > 0 {
			st.selected = (st.selected + 1) % len(matches)
		}
	case xui.KeyBackspace:
		if len(st.query) > 0 {
			st.query = st.query[:len(st.query)-1]
			st.selected = 0
		}
	case xui.KeyEnter:
		if len(matches) == 0 {
			return
		}
		st.chosen = cloneProvider(matches[min(st.selected, len(matches)-1)])
		st.errText = ""
		if st.chosen.Auth == provider.AuthOAuthBrowser || st.chosen.Auth == provider.AuthOAuthDevice {
			st.phase = connectSaving
			if st.authorize == nil {
				st.phase = connectOAuth
				st.errText = "subscription sign-in is unavailable"
				return
			}
			st.authorize(st.chosen)
			return
		}
		st.phase = connectSecret
	case xui.KeyRune:
		if event.Mods.Has(xui.ModCtrl) || event.Mods.Has(xui.ModAlt) || len(st.query) >= maxConnectQueryRunes {
			return
		}
		st.query = append(st.query, event.Rune)
		st.selected = 0
	}
}

func (o *Overlays) handleConnectSecretKey(st *connectState, event xui.KeyEvent) {
	switch event.Code {
	case xui.KeyBackspace:
		if len(st.key) == 0 {
			return
		}
		_, size := utf8.DecodeLastRune(st.key)
		if size <= 0 {
			size = 1
		}
		start := len(st.key) - size
		wipeBytes(st.key[start:])
		st.key = st.key[:start]
		st.errText = ""
	case xui.KeyEnter:
		o.submitConnect(st)
	case xui.KeyRune:
		if event.Mods.Has(xui.ModCtrl) || event.Mods.Has(xui.ModAlt) {
			return
		}
		var encoded [utf8.UTFMax]byte
		size := utf8.EncodeRune(encoded[:], event.Rune)
		if len(st.key)+size > maxConnectKeyBytes {
			st.errText = "API key is too large"
			return
		}
		st.key = append(st.key, encoded[:size]...)
		st.errText = ""
	}
}

func (o *Overlays) handleConnectPaste(text string) {
	st := o.connect
	if st == nil {
		return
	}
	switch st.phase {
	case connectSelect:
		text = strings.ReplaceAll(text, "\r", " ")
		text = strings.ReplaceAll(text, "\n", " ")
		runes := []rune(text)
		remaining := maxConnectQueryRunes - len(st.query)
		if remaining <= 0 {
			return
		}
		if len(runes) > remaining {
			runes = runes[:remaining]
		}
		st.query = append(st.query, runes...)
		st.selected = 0
	case connectSecret:
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		remaining := maxConnectKeyBytes - len(st.key)
		if remaining <= 0 || len(text) > remaining {
			st.errText = "API key is too large"
			return
		}
		st.key = append(st.key, text...)
		st.errText = ""
	}
}

func (*Overlays) submitConnect(st *connectState) {
	if st == nil || st.phase != connectSecret {
		return
	}
	key := strings.TrimSpace(string(st.key))
	wipeBytes(st.key)
	st.key = nil
	if key == "" {
		st.errText = "API key is required"
		return
	}
	submit := st.submit
	st.submit = nil
	st.phase = connectSaving
	st.errText = ""
	if submit == nil {
		st.phase = connectSecret
		st.errText = "provider connection is unavailable"
		return
	}
	submit(provider.ConnectRequest{
		ProviderID:      st.chosen.ID,
		ExpectedBaseURL: st.chosen.BaseURL,
		APIKey:          key,
	})
}

func (st *connectState) filtered() []provider.Info {
	if st == nil {
		return nil
	}
	query := strings.ToLower(strings.TrimSpace(string(st.query)))
	if query == "" {
		return st.providers
	}
	result := make([]provider.Info, 0, len(st.providers))
	for _, item := range st.providers {
		if strings.Contains(strings.ToLower(item.Name), query) ||
			strings.Contains(strings.ToLower(item.ID), query) {
			result = append(result, item)
		}
	}
	if st.selected >= len(result) {
		st.selected = max(0, len(result)-1)
	}
	return result
}

func (st *connectState) preferredHeight() int {
	if st == nil {
		return 0
	}
	if st.phase == connectSelect {
		return 13
	}
	return 10
}

func (o *Overlays) drawConnect(ctx components.DrawContext, width, height int) components.Surface {
	st := o.connect
	if st == nil {
		return components.NewSurface(width, height, nil)
	}
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = st.preferredHeight()
	}
	innerW := max(20, width-4)
	var body []components.RichLine
	add := func(style xui.Style, text string) {
		body = append(body, components.WrapSpans([]components.Span{{Text: text, Style: style}}, innerW, ctx.Method)...)
	}
	add(xui.Style{Bold: true, Fg: o.theme.Foreground.Fg}, "Connect provider")

	switch st.phase {
	case connectSelect:
		query := string(st.query)
		add(o.theme.Foreground, "› "+query+"▎")
		if st.errText != "" {
			add(o.theme.Destructive, "Catalog refresh failed: "+st.errText)
		}
		matches := st.filtered()
		if len(matches) == 0 {
			if st.errText == "" {
				add(o.theme.Muted, "Refreshing provider catalog…")
			} else {
				add(o.theme.Muted, "No validated provider catalog is available.")
			}
		} else {
			start := 0
			if st.selected >= maxVisibleProviders {
				start = st.selected - maxVisibleProviders + 1
			}
			end := min(len(matches), start+maxVisibleProviders)
			for i := start; i < end; i++ {
				item := matches[i]
				prefix := "  "
				style := o.theme.Foreground
				if i == st.selected {
					prefix = "▸ "
					style = xui.Style{Bold: true, Fg: o.theme.Success.Fg}
				}
				add(style, fmt.Sprintf("%s%s (%s) · %d models", prefix, item.Name, item.ID, len(item.Models)))
			}
		}
		add(o.theme.Muted, "Type to filter • ↑↓ navigate • Enter select • Esc cancel")
	case connectSecret, connectSaving, connectOAuth:
		add(o.theme.Foreground, st.chosen.Name+" ("+st.chosen.ID+")")
		add(o.theme.Muted, "Endpoint: "+st.chosen.BaseURL)
		add(o.theme.Muted, "Protocol: "+string(st.chosen.Protocol))
		if st.phase == connectSaving {
			if st.chosen.Auth == provider.AuthOAuthBrowser || st.chosen.Auth == provider.AuthOAuthDevice {
				add(o.theme.Foreground, "Starting subscription sign-in…")
			} else {
				add(o.theme.Foreground, "Saving credential…")
			}
			add(o.theme.Muted, "Esc cancel")
			break
		}
		if st.phase == connectOAuth {
			if st.authorizationURL != "" {
				add(o.theme.Foreground, "Open: "+st.authorizationURL)
				if st.browserErrText != "" {
					add(o.theme.Destructive, "Browser did not open automatically: "+st.browserErrText)
				}
				add(o.theme.Muted, "Waiting for authorization in browser…")
			} else if st.verificationURL != "" {
				add(o.theme.Foreground, "Open: "+st.verificationURL)
				add(xui.Style{Bold: true, Fg: o.theme.Success.Fg}, "Code: "+st.userCode)
				add(o.theme.Muted, "Waiting for authorization in browser…")
			}
			if st.errText != "" {
				add(o.theme.Destructive, st.errText)
			}
			add(o.theme.Muted, "Esc cancel")
			break
		}
		masked := maskedKey(st.key)
		add(o.theme.Foreground, "API key: "+masked+"▎")
		if st.errText != "" {
			add(o.theme.Destructive, st.errText)
		}
		add(o.theme.Muted, "Paste or type key • Enter save • Esc cancel")
	}
	return paintAskPanel(body, width, height, o.theme.Success, ctx.Method)
}

func maskedKey(key []byte) string {
	count := utf8.RuneCount(key)
	if count <= 0 {
		return ""
	}
	shown := min(count, 24)
	masked := strings.Repeat("•", shown)
	if count > shown {
		masked += fmt.Sprintf(" +%d", count-shown)
	}
	return masked
}

func cloneProviders(items []provider.Info) []provider.Info {
	result := make([]provider.Info, len(items))
	for i := range items {
		result[i] = cloneProvider(items[i])
	}
	return result
}

func cloneProvider(item provider.Info) provider.Info {
	item.Models = append([]provider.Model(nil), item.Models...)
	return item
}

func wipeBytes(data []byte) {
	for i := range data {
		data[i] = 0
	}
}
