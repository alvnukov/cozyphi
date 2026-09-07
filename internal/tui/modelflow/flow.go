package modelflow

import "slices"

// Flow is the state machine every model picker shares: pick a model,
// then — only when that model offers its own effort levels — pick one of
// them. The flow holds nothing about where it runs; palette submenus and
// in-pane lists are adapters over the same three moves: SelectModel,
// SelectEffort or Back.
type Flow struct {
	model   string
	efforts []string
}

// New returns an empty flow: no model picked, no effort page open.
func New() *Flow { return &Flow{} }

// SelectModel records the model pick and reports whether an effort
// choice must follow. efforts are the model's own levels, empty when it
// has none — such a model completes the pick in one step.
func (f *Flow) SelectModel(model string, efforts []string) bool {
	f.model = model
	f.efforts = efforts
	return len(efforts) > 0
}

// Model returns the pending model pick, "" before any selection.
func (f *Flow) Model() string { return f.model }

// Efforts lists the pending model's effort choices: its own levels, in
// the order the catalog gives them, and nothing else — the page names
// only depths the model really has. Empty unless SelectModel reported an
// effort step.
func (f *Flow) Efforts() []string {
	if len(f.efforts) == 0 {
		return nil
	}
	return slices.Clone(f.efforts)
}

// SelectEffort commits the pending choice and returns the (model,
// effort) pair the caller must apply. A choice the model does not offer
// fails closed and keeps the pending model, so Back is the only way out
// of a wrong pick.
func (f *Flow) SelectEffort(effort string) (model, effortOut string, ok bool) {
	if f.model == "" || len(f.efforts) == 0 {
		return "", "", false
	}
	if !slices.Contains(f.efforts, effort) {
		return "", "", false
	}
	model, committed := f.model, effort
	f.model, f.efforts = "", nil
	return model, committed, true
}

// Back drops the pending model pick and returns to the model list —
// the Esc-from-effort semantics every picker shares.
func (f *Flow) Back() {
	f.model, f.efforts = "", nil
}
