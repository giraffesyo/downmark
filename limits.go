package downmark

import "context"

type resultLimitContextKey struct{}

// WithResultLimit returns a derived context that limits accepted Markdown
// results to maxBytes. A child context cannot loosen an existing limit.
// It panics if ctx is nil or maxBytes is not positive.
func WithResultLimit(ctx context.Context, maxBytes int) context.Context {
	if ctx == nil {
		panic("downmark: nil context")
	}
	if maxBytes <= 0 {
		panic("downmark: result limit must be positive")
	}
	if current, ok := ResultLimit(ctx); ok && current < maxBytes {
		maxBytes = current
	}
	return context.WithValue(ctx, resultLimitContextKey{}, maxBytes)
}

// ResultLimit reports the maximum accepted Markdown result size carried by
// ctx. Converters may use it to avoid constructing a result that the engine
// will reject.
func ResultLimit(ctx context.Context) (int, bool) {
	limit, ok := ctx.Value(resultLimitContextKey{}).(int)
	return limit, ok
}
