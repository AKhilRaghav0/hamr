// Example multi-tenant demonstrates running one MCP server that serves multiple
// tenants, each with different credentials and tool permissions.
//
// Run with: go run ./examples/multi-tenant
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/AKhilRaghav0/hamr"
	"github.com/AKhilRaghav0/hamr/middleware"
)

// tenantDB is a stand-in for a real database or config store.
var tenantDB = map[string]middleware.TenantInfo{
	"sess-acme": {
		ID: "acme",
		Credentials: map[string]string{
			"greeting": "Hello from Acme Corp",
			"api_key":  "acme-secret-key",
		},
		Metadata: map[string]any{"plan": "pro"},
	},
	"sess-initech": {
		ID: "initech",
		Credentials: map[string]string{
			"greeting": "Greetings from Initech",
			"api_key":  "initech-key",
		},
		Metadata: map[string]any{"plan": "free"},
	},
}

// sessionKey is the context key callers set before invoking a tool.
// In a real server this comes from the transport layer (HTTP header, MCP session, etc.).
type sessionKey struct{}

// resolveTenant looks up the tenant for the current session.
func resolveTenant(ctx context.Context) (middleware.TenantInfo, error) {
	sid, _ := ctx.Value(sessionKey{}).(string)
	info, ok := tenantDB[sid]
	if !ok {
		return middleware.TenantInfo{}, fmt.Errorf("unknown session %q", sid)
	}
	return info, nil
}

// ---- input structs ----------------------------------------------------------

// GreetInput is the input for the greet tool.
type GreetInput struct {
	Name string `json:"name" desc:"name to greet" required:"true"`
}

// ReportInput is the input for the report tool (pro-only).
type ReportInput struct {
	Topic string `json:"topic" desc:"report topic" default:"usage"`
}

// ---- handlers ---------------------------------------------------------------

// handleGreet personalises a greeting using the caller's tenant credentials.
func handleGreet(ctx context.Context, in GreetInput) (string, error) {
	info, _ := middleware.TenantFrom(ctx)
	greeting := info.Credentials["greeting"]
	return fmt.Sprintf("%s — nice to meet you, %s! (tenant: %s)", greeting, in.Name, info.ID), nil
}

// handleReport is restricted to pro-plan tenants via TenantToolFilter.
func handleReport(ctx context.Context, in ReportInput) (string, error) {
	info, _ := middleware.TenantFrom(ctx)
	return fmt.Sprintf("[%s] Report on %q generated for tenant %s", info.Metadata["plan"], in.Topic, info.ID), nil
}

// ---- main -------------------------------------------------------------------

func main() {
	s := hamr.New("multi-tenant-demo", "1.0.0")

	// Resolve tenant from the session stored in context, then restrict tools
	// based on the tenant's plan.
	s.Use(
		middleware.Tenant(resolveTenant),
		middleware.TenantToolFilter(func(info middleware.TenantInfo, tool string) bool {
			if tool == "report" {
				return info.Metadata["plan"] == "pro"
			}
			return true
		}),
	)

	s.Tool("greet", "Greet a user with a personalised message.", handleGreet)
	s.Tool("report", "Generate a report (pro plan only).", handleReport)

	if err := s.Run(); err != nil {
		log.Fatal(err)
	}
}
