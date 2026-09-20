package harnesssettings

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/alvnukov/cozyphi/internal/configfile"
)

// WebModel reports the configured web.model pin: the name of the models-list
// entry the quarantined reader is bound to. Empty means unset — one of the
// not-ready states — and that is a configuration fact, not an error. The pin
// is read fresh from disk so a reload after an external edit reports what
// admission will resolve, not what this process remembers.
func (m *Manager) WebModel() (string, error) {
	if m == nil {
		return "", errors.New("harness settings: manager unavailable")
	}
	return loadWebModel(m.path)
}

// SetWebModel pins web.model inside one configfile.Edit cycle: the web
// section is not part of the settings snapshot, so no draft is involved and
// no other section is touched. An empty name removes the key — an unpinned
// web model is a real state (protected web not ready), not a validation
// failure. Whether the name resolves against the model catalog is decided at
// admission, not here: the catalog can change after the pin is written.
func (m *Manager) SetWebModel(ctx context.Context, name string) error {
	if m == nil {
		return errors.New("harness settings: manager unavailable")
	}
	name = strings.TrimSpace(name)
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return configfile.Edit(m.path, func(doc *yaml.Node) error {
		if name == "" {
			configfile.Remove(doc, "web", "model")
			return nil
		}
		var node yaml.Node
		if err := node.Encode(name); err != nil {
			return fmt.Errorf("harness settings: encode web.model: %w", err)
		}
		configfile.Set(doc, &node, "web", "model")
		return nil
	})
}

// loadWebModel reads the web.model pin. A missing section or key is empty;
// a non-scalar value is a configuration error, not a silent unset.
func loadWebModel(path string) (string, error) {
	doc, err := configfile.Read(path)
	if err != nil {
		return "", err
	}
	node := configfile.Lookup(doc, "web", "model")
	if node == nil || node.Tag == "!!null" {
		return "", nil
	}
	var name string
	if err := node.Decode(&name); err != nil {
		return "", fmt.Errorf("harness settings: decode web.model: %w", err)
	}
	return strings.TrimSpace(name), nil
}
