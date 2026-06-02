package middleware

import (
	"context"
	"errors"
	"fmt"
)

// contextKey is an unexported type used for context keys in this package to
// avoid collisions with keys defined in other packages.
type contextKey string

// AuthTokenKey is the context key under which the auth token is stored.
// Callers that set a token before invoking a tool should use this key:
//
//	ctx = context.WithValue(ctx, middleware.AuthTokenKey, "Bearer <token>")
const AuthTokenKey contextKey = "auth_token"

// APIKeyKey is the context key under which the API key is stored.
const APIKeyKey contextKey = "api_key"

// AuthFunc is called by the Auth middleware to validate the token extracted
// from the context. It may return an enriched context (e.g. carrying the
// resolved user identity) which the Auth middleware forwards downstream.
type AuthFunc func(ctx context.Context, token string) (context.Context, error)

// Auth returns a Middleware that extracts the auth token stored at
// AuthTokenKey in the request context and passes it to validator. If no token
// is present in the context, the request is rejected with an error. If
// validator returns an error the request is also rejected. On success the
// context returned by validator is forwarded to the next handler.
func Auth(validator AuthFunc) Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, toolName string, args map[string]any) (any, error) {
			raw := ctx.Value(AuthTokenKey)
			if raw == nil {
				return nil, errors.New("auth: no token found in context")
			}

			token, ok := raw.(string)
			if !ok || token == "" {
				return nil, fmt.Errorf("auth: token in context has unexpected type %T", raw)
			}

			enriched, err := validator(ctx, token)
			if err != nil {
				return nil, fmt.Errorf("auth: validation failed: %w", err)
			}

			return next(enriched, toolName, args)
		}
	}
}

// APIKeyAuth returns a Middleware that checks for an API key stored at
// APIKeyKey in the request context and compares it to the expected key.
// If the key is missing or incorrect, the request is rejected.
func APIKeyAuth(expectedKey string) Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, toolName string, args map[string]any) (any, error) {
			raw := ctx.Value(APIKeyKey)
			if raw == nil {
				return nil, errors.New("auth: no API key found in context")
			}

			key, ok := raw.(string)
			if !ok || key == "" {
				return nil, errors.New("auth: API key in context has unexpected type")
			}

			if key != expectedKey {
				return nil, errors.New("auth: invalid API key")
			}

			return next(ctx, toolName, args)
		}
	}
}

// JWTAuth returns a Middleware that validates a JWT token.
// This is a stub implementation for demonstration purposes.
// In a real implementation, you would use a JWT library to verify the token's signature and claims.
func JWTAuth(secret string) Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, toolName string, args map[string]any) (any, error) {
			raw := ctx.Value(AuthTokenKey)
			if raw == nil {
				return nil, errors.New("auth: no token found in context")
			}

			token, ok := raw.(string)
			if !ok || token == "" {
				return nil, errors.New("auth: token in context has unexpected type")
			}

			// Stub: check if the token starts with "Bearer " and then compare the rest to the secret.
			// This is not real JWT validation, just a stub for testing.
			if len(token) < 7 || token[:7] != "Bearer " {
				return nil, errors.New("auth: invalid token format")
			}
			tokenString := token[7:]
			if tokenString != secret {
				return nil, errors.New("auth: invalid token")
			}

			// In a real implementation, you would decode and validate the JWT here.
			// For this stub, we just return the same context.
			return next(ctx, toolName, args)
		}
	}
}