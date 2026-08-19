// Package tui wires the terminal UI shell (Editor + commands + Ask overlays).
//
// Assembly (cmd owns creation order):
//
//	redraw := controller.NewRedrawRelay()
//	bus := controller.NewBus(redraw.Fire)
//	ctrl, err := controller.NewController(bus, proj, cwd)
//	editor := NewEditor(app, bus, ctrl, cmds, …)
//	redraw.Bind(editor.RequestRedraw)
//
// Subpackages: controller (Engine/Bus/Msg), transcript (Mapper). Version lives in internal/version.
// Editor does not create Controller or call project.GetDefaultProject.
// Collaborators are constructor parameters — not a Deps bag.
// Draw/Handle/session/bash live on EditorLayout, InputRouter, SessionActions, BashMode.
package tui
