package components

// InterruptAcceptor is implemented by the root widget to claim Ctrl+C before
// the runtime quits. The App consults it on every Ctrl+C no focused widget
// took as a copy: true means the press interrupted something (a modal ask, a
// running command, an unsent draft) and the app stays up; false means nothing
// was left to interrupt and the app exits.
type InterruptAcceptor interface {
	AcceptInterrupt() bool
}
