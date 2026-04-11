package middleware

import (
	"context"
	"encoding/json"
	"time"
)

// CostStats contains estimated token usage for a single tool call.
type CostStats struct {
	ToolName       string
	RequestTokens  int           // estimated from args
	ResponseTokens int           // estimated from result
	TotalTokens    int           // request + response
	Duration       time.Duration
}

// CostTracker returns a middleware that estimates token usage for each tool call
// and reports it via the callback. Token estimation uses the approximation of
// 4 characters per token.
//
// This helps developers identify expensive tools and optimize their MCP servers.
func CostTracker(callback func(CostStats)) Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, toolName string, args map[string]any) (any, error) {
			// Estimate request tokens from args.
			argsJSON, _ := json.Marshal(args)
			requestTokens := len(argsJSON) / 4

			start := time.Now()
			result, err := next(ctx, toolName, args)
			duration := time.Since(start)

			// Estimate response tokens.
			responseTokens := 0
			if result != nil {
				switch v := result.(type) {
				case string:
					responseTokens = len(v) / 4
				default:
					// Marshal to JSON for estimation.
					if data, e := json.Marshal(v); e == nil {
						responseTokens = len(data) / 4
					}
				}
			}

			stats := CostStats{
				ToolName:       toolName,
				RequestTokens:  requestTokens,
				ResponseTokens: responseTokens,
				TotalTokens:    requestTokens + responseTokens,
				Duration:       duration,
			}

			callback(stats)

			return result, err
		}
	}
}
