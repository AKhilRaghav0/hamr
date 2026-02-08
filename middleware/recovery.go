package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
)

// Recovery returns a Middleware that catches any panic that occurs in a
// downstream handler, logs the panic value and stack trace at error level, and
// converts it into a regular error so the server remains running.
//
// The returned error message includes the panic value. Stack trace details are
// written to the structured log but not exposed in the error to avoid leaking
// internal implementation details to callers.
func Recovery() Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, toolName string, args map[string]any) (result any, err error) {
			defer func() {
				if r := recover(); r != nil {
					stack := debug.Stack()
					slog.ErrorContext(ctx, "panic in tool handler",
						slog.String("tool", toolName),
						slog.Any("panic", r),
						slog.String("stack", string(stack)),
					)
					err = fmt.Errorf("tool %q panicked: %v", toolName, r)
				}
			}()

			return next(ctx, toolName, args)
		}
	}
}
