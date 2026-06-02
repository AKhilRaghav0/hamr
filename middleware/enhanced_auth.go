package middleware

import (
	"context"
	"errors"
)

// enhancedAuthConfig holds the resolved configuration for the EnhancedAuth middleware.
type enhancedAuthConfig struct {
	extractor func(context.Context) (string, bool)
	validator func(context.Context, string) (context.Context, error)
}

// EnhancedAuthOption is a functional option for configuring the EnhancedAuth middleware.
type EnhancedAuthOption func(*enhancedAuthConfig)

// WithAPIKeyExtractor sets the extractor to look for an API key in the context
// using the given key. The validator should validate the API key.
func WithAPIKeyExtractor(key string, validator func(context.Context, string) (context.Context, error)) EnhancedAuthOption {
	return func(c *enhancedAuthConfig) {
		c.extractor = func(ctx context.Context) (string, bool) {
			val := ctx.Value(key)
			if val == nil {
				return "", false
			}
			token, ok := val.(string)
			return token, ok
		}
		c.validator = validator
	}
}

// WithJWTExtractor sets the extractor to look for a JWT token in the context
// using the given key. The validator should validate the JWT (e.g., check signature, claims).
func WithJWTExtractor(key string, validator func(context.Context, string) (context.Context, error)) EnhancedAuthOption {
	return func(c *enhancedAuthConfig) {
		c.extractor = func(ctx context.Context) (string, bool) {
			val := ctx.Value(key)
			if val == nil {
				return "", false
			}
			token, ok := val.(string)
			return token, ok
		}
		c.validator = validator
	}
}

// WithExtractor sets a custom extractor function that returns the credential string
// and a boolean indicating if found. The validator validates the credential.
func WithExtractor(extractor func(context.Context) (string, bool), validator func(context.Context, string) (context.Context, error)) EnhancedAuthOption {
	return func(c *enhancedAuthConfig) {
		c.extractor = extractor
		c.validator = validator
	}
}

// EnhancedAuth returns a Middleware that extracts credentials from the context
// using the provided extractor and validates them with the validator. If no
// credential is found, the request is rejected. If validation fails, the request
// is also rejected. On success, the context returned by the validator is forwarded
// to the next handler.
func EnhancedAuth(opts ...EnhancedAuthOption) Middleware {
	cfg := &enhancedAuthConfig{}
	for _, o := range opts {
		o(cfg)
	}

	if cfg.extractor == nil || cfg.validator == nil {
		// Return a middleware that always returns an error if misconfigured.
		return func(next HandlerFunc) HandlerFunc {
			return func(ctx context.Context, toolName string, args map[string]any) (any, error) {
				return nil, errors.New("enhanced auth: misconfigured (extractor or validator not set)")
			}
		}
	}

	return func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, toolName string, args map[string]any) (any, error) {
			token, ok := cfg.extractor(ctx)
			if !ok {
				return nil, errors.New("enhanced auth: no credentials found in context")
			}

			enriched, err := cfg.validator(ctx, token)
			if err != nil {
				return nil, err
			}

			return next(enriched, toolName, args)
		}
	}
}