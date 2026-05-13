package ddl

// Options is the resolved set of functional options for the
// collection-level and AlterOp-level operations.
//
// IfNotExists makes Create / Add operations idempotent — the target
// existing already is a no-op rather than an error. IfExists makes
// Drop operations idempotent — the target being absent is a no-op.
//
// Drivers MUST silently ignore semantically-mismatched options:
//   - IfNotExists on Drop operations
//   - IfExists on Create / Add operations
//   - Any option on ModifyField or RenameField
//
// The meaning is unambiguous (there is nothing to do with a
// mismatched hint), so a real error there would be a footgun.
type Options struct {
	IfNotExists bool
	IfExists    bool
}

// Option is the functional-option type accepted by CreateCollection,
// DropCollection, and all six AlterOp constructors via variadic
// opts ...Option. AlterCollection itself does NOT accept Option
// values directly — each AlterOp carries its own resolved Options
// set by the caller through the constructor.
type Option func(*Options)

// IfNotExists makes a Create / Add operation idempotent: the target
// already existing is a no-op rather than an error. Meaningless and
// silently ignored on Drop operations.
func IfNotExists() Option {
	return func(o *Options) { o.IfNotExists = true }
}

// IfExists makes a Drop operation idempotent: the target being
// absent is a no-op rather than an error. Meaningless and silently
// ignored on Create / Add operations.
func IfExists() Option {
	return func(o *Options) { o.IfExists = true }
}

// ResolveOptions applies opts to a zero-value Options and returns
// the result. Drivers and AlterOp constructors use this to fold a
// variadic options slice into a struct.
func ResolveOptions(opts ...Option) Options {
	var o Options
	for _, fn := range opts {
		if fn != nil {
			fn(&o)
		}
	}
	return o
}
