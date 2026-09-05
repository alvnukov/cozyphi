package statuspane

// SetModel invalidates the old usage view without starting IO. The editor may
// call this while synchronizing draw-time model labels; refreshing remains an
// explicit event, not a rendering side effect.
func (p *Pane) SetModel(model, provider string) {
	if p.snapshot.Model == model && p.provider == provider {
		return
	}
	p.snapshot.Model = model
	p.snapshot.Provider = provider
	p.provider = provider
	p.usageStale = true
}
