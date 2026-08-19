package plugin

// subOpts are shared by Hook.On and Chain.On.
type subOpts struct {
	priority int
	once     bool
}

// SubOption configures a Hook or Chain subscription.
type SubOption func(*subOpts)

// WithPriority runs the subscriber before lower-priority ones.
// Higher values run first; equal priorities keep registration order.
func WithPriority(priority int) SubOption {
	return func(o *subOpts) { o.priority = priority }
}

// WithOnce unregisters the subscriber after it runs once
// (successfully scheduled for Hook; after a non-panic call for Chain).
func WithOnce() SubOption {
	return func(o *subOpts) { o.once = true }
}

func applySubOpts(opts []SubOption) subOpts {
	var o subOpts
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	return o
}
