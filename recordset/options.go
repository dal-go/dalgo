package recordset

type Options interface {
	Name() string
}

type Option func(o *options)

func WithName(name string) Option {
	return func(o *options) {
		o.name = name
	}
}

var _ Options = (*options)(nil)

type options struct {
	name string
}

func (o options) Name() string {
	return o.name
}

func NewOptions(opts ...Option) Options {
	o := &options{}
	for _, opt := range opts {
		opt(o)
	}
	return o
}
