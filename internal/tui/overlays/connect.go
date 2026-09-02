package overlays

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/input"
	"github.com/alvnukov/cozyphi/internal/provider"
	"github.com/alvnukov/cozyphi/internal/tui/browse"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/keys"
)

const (
	maxConnectQueryRunes = 256
	maxConnectKeyBytes   = 64 << 10
	maxVisibleProviders  = 7
)

type connectPhase uint8

const (
	connectSelect connectPhase = iota
	connectMethod
	connectSecret
	connectSaving
	connectOAuth
)

type connectState struct {
	providers        []provider.Info
	query            input.Line
	ring             browse.Ring
	phase            connectPhase
	chosen           provider.Info
	methods          []provider.AuthMethod
	methodRing       browse.Ring
	method           provider.AuthMethod
	key              []byte
	errText          string
	browserErrText   string
	authorizationURL string
	verificationURL  string
	userCode         string
	submit           func(provider.ConnectRequest)
	authorize        func(provider.Info, provider.AuthMethod)
	cancel           func()
}

// BeginConnect opens the provider picker. Callbacks must return immediately;
// network and credential persistence belong on background goroutines.
func (o *Overlays) BeginConnect(
	items []provider.Info,
	submit func(provider.ConnectRequest),
	authorize func(provider.Info, provider.AuthMethod),
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
		query:     input.Line{MaxRunes: maxConnectQueryRunes},
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
		o.connect.ring.Select(0)
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
		if st.method.IsOAuth() {
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
	case connectMethod:
		o.handleConnectMethodKey(st, event)
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
		st.ring.Step(-1)
	case xui.KeyDown, xui.KeyTab:
		st.ring.Step(1)
	case xui.KeyEnter:
		if len(matches) == 0 {
			return
		}
		st.chosen = cloneProvider(matches[st.ring.Selected()])
		st.errText = ""
		st.methods = st.chosen.AuthMethods()
		st.methodRing.SetLen(len(st.methods))
		st.methodRing.Select(0)
		// One way in needs no menu; several do, and the first one listed is the
		// one the provider prefers.
		if len(st.methods) == 1 {
			st.startMethod(st.methods[0])
			return
		}
		st.phase = connectMethod
	default:
		// Everything the list does not claim is filter editing. A change restarts
		// the selection at the top, because the list under it just changed.
		before := st.query.Value
		st.query.Key(event)
		if st.query.Value != before {
			st.ring.Select(0)
		}
	}
}

func (*Overlays) handleConnectMethodKey(st *connectState, event xui.KeyEvent) {
	switch event.Code {
	case xui.KeyUp:
		st.methodRing.Step(-1)
	case xui.KeyDown, xui.KeyTab:
		st.methodRing.Step(1)
	case xui.KeyEnter:
		if len(st.methods) == 0 {
			return
		}
		st.startMethod(st.methods[st.methodRing.Selected()])
	case xui.KeyLeft:
		st.phase = connectSelect
	}
}

// startMethod enters the flow the chosen method asks for. An OAuth method hands
// off to the caller immediately, because the browser and the callback listener
// belong outside the overlay.
func (st *connectState) startMethod(method provider.AuthMethod) {
	st.method = method
	st.errText = ""
	if !method.IsOAuth() {
		st.phase = connectSecret
		return
	}
	st.phase = connectSaving
	if st.authorize == nil {
		st.phase = connectOAuth
		st.errText = "subscription sign-in is unavailable"
		return
	}
	st.authorize(st.chosen, method)
}

// handleConnectSecretKey stays hand-rolled instead of using input.Line: the key
// lives in a []byte that is wiped the moment it leaves the overlay, and a Go
// string cannot be overwritten in place.
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
		if st.query.Insert(text) {
			st.ring.Select(0)
		}
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
		ExpectedBaseURL: st.method.BaseURL,
		APIKey:          key,
	})
}

func (st *connectState) filtered() []provider.Info {
	if st == nil {
		return nil
	}
	query := strings.ToLower(st.query.Trimmed())
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
	st.ring.SetLen(len(result))
	return result
}

func (st *connectState) selectedMethod() (provider.AuthMethod, bool) {
	if st == nil || len(st.methods) == 0 {
		return provider.AuthMethod{}, false
	}
	return st.methods[st.methodRing.Selected()], true
}

func (st *connectState) preferredHeight() int {
	if st == nil {
		return 0
	}
	switch st.phase {
	case connectSelect:
		return 13
	case connectMethod:
		return 12
	default:
		return 10
	}
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
		// The filter scrolls on one row instead of wrapping: the panel height is
		// fixed, so a growing query must not push the match list out of it.
		add(o.theme.Foreground, "› "+st.query.Display(innerW-2, ctx.Method))
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
			if st.ring.Selected() >= maxVisibleProviders {
				start = st.ring.Selected() - maxVisibleProviders + 1
			}
			end := min(len(matches), start+maxVisibleProviders)
			for i := start; i < end; i++ {
				item := matches[i]
				prefix := "  "
				style := o.theme.Foreground
				if i == st.ring.Selected() {
					prefix = "▸ "
					style = xui.Style{Bold: true, Fg: o.theme.Success.Fg}
				}
				add(style, fmt.Sprintf("%s%s (%s) · %d models", prefix, item.Name, item.ID, len(item.Models)))
			}
		}
		add(o.theme.Muted, keys.Hints(keys.ScopeConnect))
	case connectMethod:
		add(o.theme.Foreground, st.chosen.Name+" ("+st.chosen.ID+")")
		add(o.theme.Muted, "How do you want to sign in?")
		for i, method := range st.methods {
			prefix := "  "
			style := o.theme.Foreground
			if i == st.methodRing.Selected() {
				prefix = "▸ "
				style = xui.Style{Bold: true, Fg: o.theme.Success.Fg}
			}
			add(style, prefix+method.Label)
		}
		if selected, ok := st.selectedMethod(); ok {
			add(o.theme.Muted, "Endpoint: "+selected.BaseURL)
		}
		if st.errText != "" {
			add(o.theme.Destructive, st.errText)
		}
		add(o.theme.Muted, keys.Hints(keys.ScopeConnectMethod))
	case connectSecret, connectSaving, connectOAuth:
		add(o.theme.Foreground, st.chosen.Name+" ("+st.chosen.ID+")")
		add(o.theme.Muted, "Sign-in: "+st.method.Label)
		add(o.theme.Muted, "Endpoint: "+st.method.BaseURL)
		add(o.theme.Muted, "Protocol: "+string(st.method.Protocol))
		if st.phase == connectSaving {
			if st.method.IsOAuth() {
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
		add(o.theme.Muted, keys.Hints(keys.ScopeConnectKey))
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
	item.Methods = append([]provider.AuthMethod(nil), item.Methods...)
	return item
}

func wipeBytes(data []byte) {
	for i := range data {
		data[i] = 0
	}
}
