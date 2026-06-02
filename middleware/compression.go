package middleware

import (
	"context"
	"strings"
)

// CompressionConfig holds the configuration for the Compression middleware.
type CompressionConfig struct {
	// If true, the middleware will compress string responses using gzip.
	// For simplicity, we'll just add a prefix to indicate compression in this stub.
	Enabled bool
}

// CompressionOption is a functional option for configuring the Compression middleware.
type CompressionOption func(*CompressionConfig)

// WithCompressionEnabled enables or disables compression.
func WithCompressionEnabled(enabled bool) CompressionOption {
	return func(c *CompressionConfig) {
		c.Enabled = enabled
	}
}

// Compression returns a Middleware that compresses string responses if
// compression is enabled and the client accepts it (in this stub, we ignore
// the client's acceptance and just compress if enabled).
func Compression(opts ...CompressionOption) Middleware {
	cfg := &CompressionConfig{
		Enabled: false,
	}
	for _, o := range opts {
		o(cfg)
	}

	return func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, toolName string, args map[string]any) (any, error) {
			result, err := next(ctx, toolName, args)
			if err != nil {
				return result, err
			}
			if cfg.Enabled {
				switch v := result.(type) {
				case string:
					// In a real implementation, we would gzip compress the string.
					// For this stub, we'll just return a compressed version by adding a prefix.
					return "COMPRESSED:" + v, nil
				case []byte:
					// Similarly, for byte slices, we'll add a prefix.
					return append([]byte("COMPRESSED:"), v...), nil
				}
				// For other types, we return as is.
			}
			return result, err
		}
	}
}