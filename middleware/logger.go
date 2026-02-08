package middleware

import (
	"context"
	"log/slog"
	"time"
)

// logConfig holds the resolved configuration for the Logger middleware.
type logConfig struct {
	logger *slog.Logger
	level  slog.Level
}

// LogOption is a functional option for configuring the Logger middleware.
type LogOption func(*logConfig)

// WithLogLevel sets the slog level used for successful request log lines.
// Error lines are always logged at slog.LevelError regardless of this setting.
func WithLogLevel(level slog.Level) LogOption {
	return func(c *logConfig) {
		c.level = level
	}
}

// WithCustomLogger replaces the default logger (slog.Default()) with the
// provided *slog.Logger.
func WithCustomLogger(logger *slog.Logger) LogOption {
	return func(c *logConfig) {
		c.logger = logger
	}
}

// Logger returns a Middleware that logs each tool invocation using structured
// logging via log/slog. It records the tool name, elapsed duration, and
// whether the call succeeded or returned an error.
//
// By default it uses slog.Default() and logs at slog.LevelInfo.
func Logger(opts ...LogOption) Middleware {
	cfg := &logConfig{
		logger: slog.Default(),
		level:  slog.LevelInfo,
	}
	for _, o := range opts {
		o(cfg)
	}

	return func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, toolName string, args map[string]any) (any, error) {
			start := time.Now()

			result, err := next(ctx, toolName, args)

			duration := time.Since(start)

			if err != nil {
				cfg.logger.LogAttrs(ctx, slog.LevelError, "tool call failed",
					slog.String("tool", toolName),
					slog.Duration("duration", duration),
					slog.String("error", err.Error()),
				)
			} else {
				cfg.logger.LogAttrs(ctx, cfg.level, "tool call succeeded",
					slog.String("tool", toolName),
					slog.Duration("duration", duration),
				)
			}

			return result, err
		}
	}
}
