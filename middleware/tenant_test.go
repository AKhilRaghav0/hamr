package middleware_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/AKhilRaghav0/hamr/middleware"
)

// ---- Tenant -----------------------------------------------------------------

func TestTenant_ResolvesAndInjectsTenantInfo(t *testing.T) {
	t.Parallel()

	resolver := func(_ context.Context) (middleware.TenantInfo, error) {
		return middleware.TenantInfo{ID: "acme"}, nil
	}

	var capturedInfo middleware.TenantInfo
	inner := middleware.HandlerFunc(func(ctx context.Context, _ string, _ map[string]any) (any, error) {
		info, ok := middleware.TenantFrom(ctx)
		if !ok {
			return nil, errors.New("no tenant in context")
		}
		capturedInfo = info
		return "ok", nil
	})

	mw := middleware.Tenant(resolver)
	handler := mw(inner)

	_, err := handler(context.Background(), "tool", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedInfo.ID != "acme" {
		t.Errorf("expected tenant ID %q, got %q", "acme", capturedInfo.ID)
	}
}

func TestTenant_HandlerCanReadTenantInfo(t *testing.T) {
	t.Parallel()

	resolver := func(_ context.Context) (middleware.TenantInfo, error) {
		return middleware.TenantInfo{
			ID: "globex",
			Credentials: map[string]string{
				"api_key": "secret-123",
			},
			Metadata: map[string]any{
				"plan": "pro",
			},
		}, nil
	}

	var got middleware.TenantInfo
	inner := middleware.HandlerFunc(func(ctx context.Context, _ string, _ map[string]any) (any, error) {
		info, _ := middleware.TenantFrom(ctx)
		got = info
		return "ok", nil
	})

	mw := middleware.Tenant(resolver)
	handler := mw(inner)

	_, err := handler(context.Background(), "tool", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "globex" {
		t.Errorf("expected ID %q, got %q", "globex", got.ID)
	}
	if got.Metadata["plan"] != "pro" {
		t.Errorf("expected plan %q, got %v", "pro", got.Metadata["plan"])
	}
}

func TestTenant_ResolverErrorRejectsCall(t *testing.T) {
	t.Parallel()

	resolver := func(_ context.Context) (middleware.TenantInfo, error) {
		return middleware.TenantInfo{}, errors.New("unknown tenant")
	}

	mw := middleware.Tenant(resolver)
	handler := mw(func(_ context.Context, _ string, _ map[string]any) (any, error) {
		return "should not reach", nil
	})

	_, err := handler(context.Background(), "tool", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "tenant resolution failed") {
		t.Errorf("expected 'tenant resolution failed' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "unknown tenant") {
		t.Errorf("expected wrapped error 'unknown tenant', got: %v", err)
	}
}

func TestTenantFrom_ReturnsFalseWhenNotSet(t *testing.T) {
	t.Parallel()

	_, ok := middleware.TenantFrom(context.Background())
	if ok {
		t.Error("expected false from TenantFrom on a plain context")
	}
}

func TestTenant_CredentialsAccessibleInHandler(t *testing.T) {
	t.Parallel()

	resolver := func(_ context.Context) (middleware.TenantInfo, error) {
		return middleware.TenantInfo{
			ID: "initech",
			Credentials: map[string]string{
				"db_url":  "postgres://initech@localhost/db",
				"api_key": "tok-abc",
			},
		}, nil
	}

	var dbURL, apiKey string
	inner := middleware.HandlerFunc(func(ctx context.Context, _ string, _ map[string]any) (any, error) {
		info, _ := middleware.TenantFrom(ctx)
		dbURL = info.Credentials["db_url"]
		apiKey = info.Credentials["api_key"]
		return "ok", nil
	})

	mw := middleware.Tenant(resolver)
	handler := mw(inner)

	_, err := handler(context.Background(), "tool", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dbURL != "postgres://initech@localhost/db" {
		t.Errorf("unexpected db_url: %q", dbURL)
	}
	if apiKey != "tok-abc" {
		t.Errorf("unexpected api_key: %q", apiKey)
	}
}

// ---- TenantToolFilter -------------------------------------------------------

func TestTenantToolFilter_AllowsPermittedTools(t *testing.T) {
	t.Parallel()

	// Inject a tenant into context manually, as if Tenant middleware already ran.
	tenantCtx := func() context.Context {
		ctx := context.Background()
		mw := middleware.Tenant(func(_ context.Context) (middleware.TenantInfo, error) {
			return middleware.TenantInfo{ID: "tier1", Metadata: map[string]any{"plan": "pro"}}, nil
		})
		var captured context.Context
		mw(func(ctx context.Context, _ string, _ map[string]any) (any, error) {
			captured = ctx
			return nil, nil
		})(ctx, "tool", nil) //nolint:errcheck
		return captured
	}()

	filter := func(info middleware.TenantInfo, tool string) bool {
		return true // allow everything
	}

	mw := middleware.TenantToolFilter(filter)
	handler := mw(func(_ context.Context, _ string, _ map[string]any) (any, error) {
		return "allowed", nil
	})

	val, err := handler(tenantCtx, "any_tool", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "allowed" {
		t.Errorf("expected %q, got %v", "allowed", val)
	}
}

func TestTenantToolFilter_BlocksRestrictedTools(t *testing.T) {
	t.Parallel()

	// Build a context that already has a tenant injected.
	var tenantCtx context.Context
	middleware.Tenant(func(_ context.Context) (middleware.TenantInfo, error) {
		return middleware.TenantInfo{ID: "free-tier", Metadata: map[string]any{"plan": "free"}}, nil
	})(func(ctx context.Context, _ string, _ map[string]any) (any, error) {
		tenantCtx = ctx
		return nil, nil
	})(context.Background(), "tool", nil) //nolint:errcheck

	filter := func(info middleware.TenantInfo, tool string) bool {
		// Only pro tenants can use "expensive_tool"
		if tool == "expensive_tool" {
			return info.Metadata["plan"] == "pro"
		}
		return true
	}

	mw := middleware.TenantToolFilter(filter)
	handler := mw(func(_ context.Context, _ string, _ map[string]any) (any, error) {
		return "should not reach", nil
	})

	_, err := handler(tenantCtx, "expensive_tool", nil)
	if err == nil {
		t.Fatal("expected error for restricted tool, got nil")
	}
	if !strings.Contains(err.Error(), "expensive_tool") {
		t.Errorf("expected tool name in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "free-tier") {
		t.Errorf("expected tenant ID in error, got: %v", err)
	}
}

func TestTenantToolFilter_PassesThroughWhenNoTenantInContext(t *testing.T) {
	t.Parallel()

	filter := func(_ middleware.TenantInfo, _ string) bool {
		return false // would block if tenant was present
	}

	mw := middleware.TenantToolFilter(filter)
	handler := mw(func(_ context.Context, _ string, _ map[string]any) (any, error) {
		return "passed through", nil
	})

	// Plain context — no tenant injected.
	val, err := handler(context.Background(), "any_tool", nil)
	if err != nil {
		t.Fatalf("expected pass-through, got error: %v", err)
	}
	if val != "passed through" {
		t.Errorf("expected %q, got %v", "passed through", val)
	}
}

func TestTenant_ComposedWithTenantToolFilter(t *testing.T) {
	t.Parallel()

	resolver := func(ctx context.Context) (middleware.TenantInfo, error) {
		id, _ := ctx.Value("tid").(string)
		if id == "" {
			return middleware.TenantInfo{}, errors.New("no session")
		}
		plans := map[string]string{"t1": "pro", "t2": "free"}
		plan, ok := plans[id]
		if !ok {
			return middleware.TenantInfo{}, fmt.Errorf("unknown tenant %q", id)
		}
		return middleware.TenantInfo{
			ID:       id,
			Metadata: map[string]any{"plan": plan},
		}, nil
	}

	filter := func(info middleware.TenantInfo, tool string) bool {
		if tool == "pro_tool" {
			return info.Metadata["plan"] == "pro"
		}
		return true
	}

	chain := middleware.Chain(
		middleware.Tenant(resolver),
		middleware.TenantToolFilter(filter),
	)
	handler := chain(func(_ context.Context, _ string, _ map[string]any) (any, error) {
		return "ok", nil
	})

	// Pro tenant can use pro_tool.
	ctxPro := context.WithValue(context.Background(), "tid", "t1")
	_, err := handler(ctxPro, "pro_tool", nil)
	if err != nil {
		t.Errorf("pro tenant should be allowed: %v", err)
	}

	// Free tenant cannot use pro_tool.
	ctxFree := context.WithValue(context.Background(), "tid", "t2")
	_, err = handler(ctxFree, "pro_tool", nil)
	if err == nil {
		t.Error("free tenant should be blocked from pro_tool")
	}

	// Free tenant can use basic_tool.
	_, err = handler(ctxFree, "basic_tool", nil)
	if err != nil {
		t.Errorf("free tenant should access basic_tool: %v", err)
	}

	// Missing session rejects regardless of tool.
	_, err = handler(context.Background(), "basic_tool", nil)
	if err == nil {
		t.Error("missing session should be rejected")
	}
}

func TestTenant_ConcurrentCallsDontRace(t *testing.T) {
	t.Parallel()

	// Each goroutine supplies its own tenant ID via context. Resolver reads it and
	// returns distinct TenantInfo values. The test is run with -race.
	resolver := func(ctx context.Context) (middleware.TenantInfo, error) {
		id, _ := ctx.Value("tid").(string)
		return middleware.TenantInfo{
			ID:          id,
			Credentials: map[string]string{"key": "val-" + id},
		}, nil
	}

	mw := middleware.Tenant(resolver)
	handler := mw(func(ctx context.Context, _ string, _ map[string]any) (any, error) {
		info, _ := middleware.TenantFrom(ctx)
		return info.ID, nil
	})

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			id := fmt.Sprintf("tenant-%d", i)
			ctx := context.WithValue(context.Background(), "tid", id)
			val, err := handler(ctx, "tool", nil)
			if err != nil {
				t.Errorf("goroutine %d: unexpected error: %v", i, err)
				return
			}
			if val != id {
				t.Errorf("goroutine %d: expected %q, got %v", i, id, val)
			}
		}()
	}

	wg.Wait()
}
