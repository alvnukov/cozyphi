// Package tui wires the terminal UI.
//
// Assembly (cmd owns creation order):
//
//	redraw := NewRedrawRelay()
//	bus := NewBus(redraw.Fire)
//	ctrl, err := NewController(bus, proj, cwd)
//	editor := NewEditor(app, bus, ctrl, cmds, …)
//	redraw.Bind(editor.RequestRedraw)
//
// Editor does not create Controller or call project.GetDefaultProject.
// Collaborators are constructor parameters — not a Deps bag.
package tui
