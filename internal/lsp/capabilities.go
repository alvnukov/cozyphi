package lsp

// supports reports whether the initialize capabilities stored for this
// client generation advertise key. A capability counts when it is true or an
// options object; false or missing means the operation must fail closed.
func (c *client) supports(key string) bool {
	c.capsMu.Lock()
	defer c.capsMu.Unlock()
	switch v := c.caps[key].(type) {
	case bool:
		return v
	case map[string]any:
		return true
	default:
		return false
	}
}

// requireCapability fails closed with a typed unsupported error naming the
// missing capability, keeping unsupported operations distinguishable from
// successful empty results.
func (c *client) requireCapability(key string, op Operation) error {
	if !c.supports(key) {
		return newError(ErrUnsupported, "%s needs %s, which the server does not advertise", op, key)
	}
	return nil
}
