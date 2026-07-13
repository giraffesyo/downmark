package downmark

import "net/http"

// Option configures an Engine created by New.
type Option func(*Engine)

// WithoutBuiltins starts the Engine with an empty converter registry
// (omitting even the plain-text converter). Use Register to add converters.
func WithoutBuiltins() Option {
	return func(e *Engine) { e.noBuiltins = true }
}

// WithHTTPClient sets the HTTP client used for URL-based conversions.
func WithHTTPClient(c *http.Client) Option {
	return func(e *Engine) { e.httpClient = c }
}
