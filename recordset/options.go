package recordset

// Options reports the name configured for a recordset.
type Options interface {
	// Name returns the recordset name. It is empty when no name was set.
	Name() string
}

// Option configures the recordset options built by NewOptions.
type Option func(o *options)

// WithName returns an Option that sets the recordset name.
func WithName(name string) Option {
	return func(o *options) {
		o.name = name
	}
}

var _ Options = (*options)(nil)

type options struct {
	name string
}

// Name returns the configured recordset name.
func (o options) Name() string {
	return o.name
}

// NewOptions applies opts and returns the resulting Options.
func NewOptions(opts ...Option) Options {
	o := &options{}
	for _, opt := range opts {
		opt(o)
	}
	return o
}
