// Package middleware provides composable middleware for MCP tool handlers.
//
// Each middleware wraps a HandlerFunc and may inspect or modify the context,
// arguments, and result. Middleware is composed using Chain, which preserves
// left-to-right execution order — the first middleware in the list is the
// outermost wrapper and executes first on the way in, last on the way out.
package middleware

import "context"

// HandlerFunc is the function signature used throughout the middleware chain.
// toolName is the registered MCP tool name, and args is the decoded argument
// map after default injection and schema validation. The return value is the
// raw handler result (string, []mcpx.Content, or mcpx.Result).
type HandlerFunc func(ctx context.Context, toolName string, args map[string]any) (any, error)

// Middleware is a function that wraps a HandlerFunc and returns a new
// HandlerFunc, enabling cross-cutting concerns such as logging, tracing,
// rate limiting, and authentication to be layered around tool calls.
//
// A minimal middleware that logs every call:
//
//	func Logger(next middleware.HandlerFunc) middleware.HandlerFunc {
//	    return func(ctx context.Context, tool string, args map[string]any) (any, error) {
//	        log.Printf("calling %s", tool)
//	        result, err := next(ctx, tool, args)
//	        log.Printf("done %s err=%v", tool, err)
//	        return result, err
//	    }
//	}
type Middleware func(next HandlerFunc) HandlerFunc

// Chain composes multiple middleware into a single Middleware. The first
// middleware in the slice is the outermost wrapper: it runs first on the way
// in and last on the way out.
//
// Given Chain(A, B, C), a call flows as:
//
//	A → B → C → handler → C → B → A
func Chain(middlewares ...Middleware) Middleware {
	return func(final HandlerFunc) HandlerFunc {
		// Apply in reverse order so that the first middleware in the slice
		// ends up as the outermost layer.
		h := final
		for i := len(middlewares) - 1; i >= 0; i-- {
			h = middlewares[i](h)
		}
		return h
	}
}
