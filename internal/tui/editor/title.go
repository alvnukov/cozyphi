package editor

// TerminalTitle is sampled by App after Draw has drained all session mailboxes.
// It never writes terminal bytes and background sessions cannot select its value.
func (e *Editor) TerminalTitle() string {
	if entry, ok := e.registry.Active(); ok {
		return entry.DisplayName()
	}
	return "cozyphi"
}
