package middleware

import (
	"context"
	"fmt"
)

// TenantInfo holds per-tenant credentials and metadata.
// Users populate this however they want — from a database, config file, etc.
type TenantInfo struct {
	ID          string            // tenant identifier
	Credentials map[string]string // arbitrary key-value credentials (api keys, db urls, etc.)
	Metadata    map[string]any    // extra data the tool handler might need
}

// TenantResolver is called on every tool invocation to determine which tenant
// is making the request. It receives the context (which may contain session info
// from the transport layer) and returns tenant info.
//
// Return an error to reject the request (e.g. unknown tenant, expired session).
type TenantResolver func(ctx context.Context) (TenantInfo, error)

// tenantKey is the context key for tenant info.
type tenantKey struct{}

// TenantFrom extracts the TenantInfo from context. Returns empty TenantInfo and
// false if no tenant was set (i.e. middleware not in use).
func TenantFrom(ctx context.Context) (TenantInfo, bool) {
	info, ok := ctx.Value(tenantKey{}).(TenantInfo)
	return info, ok
}

// Tenant returns a middleware that resolves the current tenant on every tool call
// and injects TenantInfo into the context. Tool handlers access it via TenantFrom(ctx).
//
// If the resolver returns an error, the tool call is rejected immediately.
//
// Example:
//
//	s.Use(middleware.Tenant(func(ctx context.Context) (middleware.TenantInfo, error) {
//	    sessionID := ctx.Value("session_id").(string)
//	    tenant, err := db.LookupTenant(sessionID)
//	    if err != nil {
//	        return middleware.TenantInfo{}, fmt.Errorf("unknown tenant")
//	    }
//	    return middleware.TenantInfo{
//	        ID: tenant.ID,
//	        Credentials: map[string]string{
//	            "db_url":  tenant.DatabaseURL,
//	            "api_key": tenant.APIKey,
//	        },
//	    }, nil
//	}))
func Tenant(resolver TenantResolver) Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, toolName string, args map[string]any) (any, error) {
			info, err := resolver(ctx)
			if err != nil {
				return nil, fmt.Errorf("tenant resolution failed: %w", err)
			}
			ctx = context.WithValue(ctx, tenantKey{}, info)
			return next(ctx, toolName, args)
		}
	}
}

// TenantToolFilter returns a middleware that restricts which tools a tenant can access.
// The filter function receives the tenant info and tool name, returning true if allowed.
//
// Use this with Tenant middleware to implement per-tenant tool permissions:
//
//	s.Use(middleware.Tenant(resolver))
//	s.Use(middleware.TenantToolFilter(func(info middleware.TenantInfo, tool string) bool {
//	    // Check if this tenant's plan allows this tool
//	    return info.Metadata["plan"] == "pro" || tool != "expensive_tool"
//	}))
func TenantToolFilter(filter func(TenantInfo, string) bool) Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, toolName string, args map[string]any) (any, error) {
			info, ok := TenantFrom(ctx)
			if !ok {
				// No tenant in context, pass through (Tenant middleware not used)
				return next(ctx, toolName, args)
			}
			if !filter(info, toolName) {
				return nil, fmt.Errorf("tool %q is not available for tenant %q", toolName, info.ID)
			}
			return next(ctx, toolName, args)
		}
	}
}
