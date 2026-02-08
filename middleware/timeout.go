package middleware

import (
	"context"
	"fmt"
	"time"
)

// Timeout returns a Middleware that cancels the request context after duration
// d. If the handler does not complete within d the context deadline is
// exceeded and the middleware returns immediately with a timeout error; the
// handler's response (when it eventually arrives) is discarded.
//
// The inner handler is executed in a separate goroutine so that the caller is
// not blocked past d. Handlers that do not honour context cancellation will
// continue running in the background until they complete, at which point their
// goroutine exits cleanly (the result channel is buffered). To bound resource
// usage, ensure all handlers respect ctx.Done().
func Timeout(d time.Duration) Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, toolName string, args map[string]any) (any, error) {
			ctx, cancel := context.WithTimeout(ctx, d)
			defer cancel()

			type result struct {
				val any
				err error
			}

			ch := make(chan result, 1)

			go func() {
				val, err := next(ctx, toolName, args)
				ch <- result{val, err}
			}()

			select {
			case r := <-ch:
				return r.val, r.err
			case <-ctx.Done():
				return nil, fmt.Errorf("tool %q timed out after %s: %w", toolName, d, ctx.Err())
			}
		}
	}
}
