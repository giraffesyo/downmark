package downmark

// Option configures an Engine created by New.
type Option func(*Engine)

// WithoutBuiltins starts the Engine with an empty converter registry
// (omitting even the plain-text converter). Use Register to add converters.
func WithoutBuiltins() Option {
	return func(e *Engine) { e.noBuiltins = true }
}
