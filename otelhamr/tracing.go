// Package otelhamr provides OpenTelemetry tracing middleware for hamr MCP servers.
//
// This is a separate Go module to avoid adding OTel dependencies to the core
// hamr module. Only import this package if you need distributed tracing.
//
// Usage:
//
//	import "github.com/AKhilRaghav0/hamr/otelhamr"
//	s.Use(otelhamr.Tracing(tracer))
package otelhamr

import (
	"context"
	"encoding/json"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/AKhilRaghav0/hamr/middleware"
)

// Tracing returns a hamr middleware that creates an OpenTelemetry span for
// each tool call. The span includes the tool name, argument summary, duration,
// and error status.
//
// Attributes set on each span:
//   - mcp.tool.name: the tool name
//   - mcp.tool.args_size: byte size of the arguments JSON
//   - mcp.tool.response_size: byte size of the response (estimated)
//
// On error, the span status is set to Error with the error message recorded.
func Tracing(tracer trace.Tracer) middleware.Middleware {
	return func(next middleware.HandlerFunc) middleware.HandlerFunc {
		return func(ctx context.Context, toolName string, args map[string]any) (any, error) {
			ctx, span := tracer.Start(ctx, fmt.Sprintf("mcp.tool.%s", toolName),
				trace.WithSpanKind(trace.SpanKindServer),
			)
			defer span.End()

			// Record tool name
			span.SetAttributes(attribute.String("mcp.tool.name", toolName))

			// Record args size
			if args != nil {
				if argsJSON, err := json.Marshal(args); err == nil {
					span.SetAttributes(attribute.Int("mcp.tool.args_size", len(argsJSON)))
				}
			}

			// Call the handler
			result, err := next(ctx, toolName, args)

			// Record response size
			if result != nil {
				switch v := result.(type) {
				case string:
					span.SetAttributes(attribute.Int("mcp.tool.response_size", len(v)))
				default:
					if data, e := json.Marshal(v); e == nil {
						span.SetAttributes(attribute.Int("mcp.tool.response_size", len(data)))
					}
				}
			}

			// Record errors
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			} else {
				span.SetStatus(codes.Ok, "")
			}

			return result, err
		}
	}
}
