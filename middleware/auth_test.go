package middleware

import (
	"context"
	"errors"
	"testing"
)

func TestAuth_Basic(t *testing.T) {
	t.Parallel()

	// Validator that accepts any token and returns the same context.
	validator := func(ctx context.Context, token string) (context.Context, error) {
		return ctx, nil
	}

	mw := Auth(validator)
	handler := func(ctx context.Context, tool string, args map[string]any) (any, error) {
		return "success", nil
	}
	chained := mw(handler)

	// Call with a token in context.
	ctx := context.WithValue(context.Background(), AuthTokenKey, "my-token")
	result, err := chained(ctx, "test", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result != "success" {
		t.Errorf("Expected 'success', got %v", result)
	}
}

func TestAuth_MissingToken(t *testing.T) {
	t.Parallel()

	validator := func(ctx context.Context, token string) (context.Context, error) {
		return ctx, nil
	}

	mw := Auth(validator)
	handler := func(ctx context.Context, tool string, args map[string]any) (any, error) {
		return "success", nil
	}
	chained := mw(handler)

	// Call without token in context.
	ctx := context.Background()
	result, err := chained(ctx, "test", nil)
	if err == nil {
		t.Fatalf("Expected error for missing token")
	}
	if result != nil {
		t.Fatalf("Expected nil result on error")
	}
	if err.Error() != "auth: no token found in context" {
		t.Errorf("Expected missing token error, got %v", err)
	}
}

func TestAuth_InvalidToken(t *testing.T) {
	t.Parallel()

	validator := func(ctx context.Context, token string) (context.Context, error) {
		return nil, errors.New("invalid token")
	}

	mw := Auth(validator)
	handler := func(ctx context.Context, tool string, args map[string]any) (any, error) {
		return "success", nil
	}
	chained := mw(handler)

	ctx := context.WithValue(context.Background(), AuthTokenKey, "bad-token")
	result, err := chained(ctx, "test", nil)
	if err == nil {
		t.Fatalf("Expected error from validator")
	}
	if result != nil {
		t.Fatalf("Expected nil result on error")
	}
	if err.Error() != "auth: validation failed: invalid token" {
		t.Errorf("Expected wrapped error, got %v", err)
	}
}

func TestAPIKeyAuth_Success(t *testing.T) {
	t.Parallel()

	expectedKey := "secret123"
	mw := APIKeyAuth(expectedKey)
	handler := func(ctx context.Context, tool string, args map[string]any) (any, error) {
		return "success", nil
	}
	chained := mw(handler)

	ctx := context.WithValue(context.Background(), APIKeyKey, expectedKey)
	result, err := chained(ctx, "test", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result != "success" {
		t.Errorf("Expected 'success', got %v", result)
	}
}

func TestAPIKeyAuth_MissingKey(t *testing.T) {
	t.Parallel()

	mw := APIKeyAuth("secret")
	handler := func(ctx context.Context, tool string, args map[string]any) (any, error) {
		return "success", nil
	}
	chained := mw(handler)

	ctx := context.Background()
	result, err := chained(ctx, "test", nil)
	if err == nil {
		t.Fatalf("Expected error for missing API key")
	}
	if result != nil {
		t.Fatalf("Expected nil result on error")
	}
	if err.Error() != "auth: no API key found in context" {
		t.Errorf("Expected missing API key error, got %v", err)
	}
}

func TestAPIKeyAuth_InvalidKey(t *testing.T) {
	t.Parallel()

	mw := APIKeyAuth("secret")
	handler := func(ctx context.Context, tool string, args map[string]any) (any, error) {
		return "success", nil
	}
	chained := mw(handler)

	ctx := context.WithValue(context.Background(), APIKeyKey, "wrong-key")
	result, err := chained(ctx, "test", nil)
	if err == nil {
		t.Fatalf("Expected error for invalid API key")
	}
	if result != nil {
		t.Fatalf("Expected nil result on error")
	}
	if err.Error() != "auth: invalid API key" {
		t.Errorf("Expected invalid API key error, got %v", err)
	}
}

func TestJWTAuth_Success(t *testing.T) {
	t.Parallel()

	secret := "my-secret"
	mw := JWTAuth(secret)
	handler := func(ctx context.Context, tool string, args map[string]any) (any, error) {
		return "success", nil
	}
	chained := mw(handler)

	// Token format: Bearer <secret>
	token := "Bearer " + secret
	ctx := context.WithValue(context.Background(), AuthTokenKey, token)
	result, err := chained(ctx, "test", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result != "success" {
		t.Errorf("Expected 'success', got %v", result)
	}
}

func TestJWTAuth_MissingToken(t *testing.T) {
	t.Parallel()

	mw := JWTAuth("secret")
	handler := func(ctx context.Context, tool string, args map[string]any) (any, error) {
		return "success", nil
	}
	chained := mw(handler)

	ctx := context.Background()
	result, err := chained(ctx, "test", nil)
	if err == nil {
		t.Fatalf("Expected error for missing token")
	}
	if result != nil {
		t.Fatalf("Expected nil result on error")
	}
	if err.Error() != "auth: no token found in context" {
		t.Errorf("Expected missing token error, got %v", err)
	}
}

func TestJWTAuth_InvalidFormat(t *testing.T) {
	t.Parallel()

	mw := JWTAuth("secret")
	handler := func(ctx context.Context, tool string, args map[string]any) (any, error) {
		return "success", nil
	}
	chained := mw(handler)

	// Token without Bearer prefix
	ctx := context.WithValue(context.Background(), AuthTokenKey, "just-a-token")
	result, err := chained(ctx, "test", nil)
	if err == nil {
		t.Fatalf("Expected error for invalid token format")
	}
	if result != nil {
		t.Fatalf("Expected nil result on error")
	}
	if err.Error() != "auth: invalid token format" {
		t.Errorf("Expected invalid token format error, got %v", err)
	}
}

func TestJWTAuth_InvalidToken(t *testing.T) {
	t.Parallel()

	mw := JWTAuth("secret")
	handler := func(ctx context.Context, tool string, args map[string]any) (any, error) {
		return "success", nil
	}
	chained := mw(handler)

	// Token with Bearer prefix but wrong secret
	ctx := context.WithValue(context.Background(), AuthTokenKey, "Bearer wrong-secret")
	result, err := chained(ctx, "test", nil)
	if err == nil {
		t.Fatalf("Expected error for invalid token")
	}
	if result != nil {
		t.Fatalf("Expected nil result on error")
	}
	if err.Error() != "auth: invalid token" {
		t.Errorf("Expected invalid token error, got %v", err)
	}
}