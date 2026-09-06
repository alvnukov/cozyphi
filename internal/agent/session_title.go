package agent

import "errors"

// SetTitle changes this session's durable name without altering model context.
func (s *Session) SetTitle(title, source string) error {
	if s == nil || s.manager == nil {
		return errors.New("agent: session unavailable")
	}
	return s.manager.SetTitle(title, source)
}

// Title returns the explicit session name and its owner.
func (s *Session) Title() (title, source string) {
	if s == nil || s.manager == nil {
		return "", ""
	}
	return s.manager.Title()
}

// DisplayTitle includes the first-prompt and short-ID fallbacks.
func (s *Session) DisplayTitle() string {
	if s == nil || s.manager == nil {
		return ""
	}
	return s.manager.DisplayTitle()
}
