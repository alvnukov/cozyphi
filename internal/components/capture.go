package components

import "github.com/pulseaiclub/xui"

// EventCapturer lets the root handle process-level navigation before focused
// widgets or mouse hit targets consume it. Unclaimed events keep their ordinary
// target-and-bubble delivery; implementations must consume only their own keys
// and screen regions.
type EventCapturer interface {
	Capture(*EventContext, xui.Event)
}
