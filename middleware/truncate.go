package middleware

import (
	"context"
	"strconv"
	"strings"
)

// MaxResponseTokens returns a middleware that truncates large text responses
// to approximately maxTokens tokens (estimated at 4 characters per token).
// This saves tokens on the AI's reading side.
//
// Only truncates string results. Non-string results (Content, Result) are
// passed through unchanged since they may contain binary data.
func MaxResponseTokens(maxTokens int) Middleware {
	maxChars := maxTokens * 4
	return func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, toolName string, args map[string]any) (any, error) {
			result, err := next(ctx, toolName, args)
			if err != nil {
				return result, err
			}
			// Only truncate string results.
			if s, ok := result.(string); ok {
				if len(s) > maxChars {
					truncated := s[:maxChars]
					// Try to cut at last newline for cleaner output.
					if idx := strings.LastIndex(truncated, "\n"); idx > maxChars/2 {
						truncated = truncated[:idx]
					}
					return truncated + "\n\n... [response truncated at ~" + strconv.Itoa(maxTokens) + " tokens. Use pagination or offset parameters to see more]", nil
				}
			}
			return result, err
		}
	}
}
