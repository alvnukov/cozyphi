package controller

import "sync"

// childAttachment is a broadcast completion fence, not a consumable Ready reply.
// A follow-up may observe the same fence before the initial runner clears it.
// Closing done publishes err to every assignment waiting on this View.
type childAttachment struct {
	done chan struct{}
	once sync.Once
	err  error
}

func newChildAttachment() *childAttachment {
	return &childAttachment{done: make(chan struct{})}
}

func (a *childAttachment) finish(err error) {
	a.once.Do(func() {
		a.err = err
		close(a.done)
	})
}
