package hamr

// Comprehensive QA tests for:
//   - Tool Groups (registration, expansion, direct call, panics, edge cases)
//   - Minimal Schemas (stripping, preservation of required fields, recursive)
//   - EstimateSchemaTokens (standalone, group selectors, not grouped tools)
//   - handleInitialize capabilities when server has ONLY groups (regression for bug #1)
//   - Integration test (WithMinimalSchemas + groups + CostTracker + truncation)

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/AKhilRaghav0/hamr/middleware"
	"github.com/AKhilRaghav0/hamr/transport"
)

// ---- shared types for new feature tests -------------------------------------

type k8sInput struct {
	Namespace string `json:"namespace" desc:"Kubernetes namespace" default:"default"`
	Label     string `json:"label" desc:"label selector" required:"true"`
}

type richSchemaInput struct {
	Name    string  `json:"name" desc:"item name" required:"true"`
	Count   int     `json:"count" desc:"item count" default:"1" min:"1" max:"100"`
	Mode    string  `json:"mode" desc:"mode" enum:"fast,slow,medium"`
	Pattern string  `json:"pattern" desc:"regex" pattern:"^[a-z]+$"`
	Score   float64 `json:"score" desc:"relevance score" default:"0.5"`
}

// ---- Tool Group: registration and list -------------------------------------

// TestToolGroup_ThreeToolsListShowsOneSelector verifies that a group with 3
// tools results in exactly one "group__<name>" entry in tools/list.
func TestToolGroup_ThreeToolsListShowsOneSelector(t *testing.T) {
	t.Parallel()

	s := New("k8s-server", "1.0.0")
	s.ToolGroup("kubernetes", "Kubernetes operations", func(g *Group) {
		g.Tool("get_pods", "List pods in namespace", func(_ context.Context, in k8sInput) (string, error) {
			return "pods in " + in.Namespace, nil
		})
		g.Tool("get_services", "List services", func(_ context.Context, in k8sInput) (string, error) {
			return "services in " + in.Namespace, nil
		})
		g.Tool("get_deployments", "List deployments", func(_ context.Context, in k8sInput) (string, error) {
			return "deployments in " + in.Namespace, nil
		})
	})

	h := s.NewTestHandler()
	resp := h.HandleRequest(context.Background(), &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      200,
		Method:  "tools/list",
	})
	if resp.Error != nil {
		t.Fatalf("tools/list error: %v", resp.Error.Message)
	}

	tools := extractTools(t, resp)

	// Only the group selector should appear — no individual sub-tools.
	if len(tools) != 1 {
		t.Errorf("expected 1 entry (group selector), got %d", len(tools))
	}

	names := toolNames(tools)
	if !names["group__kubernetes"] {
		t.Error("group selector 'group__kubernetes' should be in tools/list")
	}
	if names["get_pods"] || names["get_services"] || names["get_deployments"] {
		t.Error("individual group tools must NOT appear in tools/list")
	}
}

// TestToolGroup_SelectorReturnsSubToolListing verifies that calling
// group__kubernetes returns a readable text listing of sub-tools with params.
func TestToolGroup_SelectorReturnsSubToolListing(t *testing.T) {
	t.Parallel()

	s := New("k8s-server", "1.0.0")
	s.ToolGroup("kubernetes", "Kubernetes operations", func(g *Group) {
		g.Tool("get_pods", "List pods", func(_ context.Context, in k8sInput) (string, error) {
			return "ok", nil
		})
	})
	h := s.NewTestHandler()

	resp := h.HandleRequest(context.Background(), &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      201,
		Method:  "tools/call",
		Params:  mustMarshal(map[string]any{"name": "group__kubernetes", "arguments": map[string]any{}}),
	})
	if resp.Error != nil {
		t.Fatalf("unexpected RPC error: %v", resp.Error.Message)
	}

	result := resp.Result.(map[string]any)
	if isErr, _ := result["isError"].(bool); isErr {
		t.Error("isError should be false for group expansion")
	}

	text := extractFirstContentText(t, result)
	if !strings.Contains(text, "get_pods") {
		t.Errorf("group expansion should mention 'get_pods', got:\n%s", text)
	}
	// Should include parameter info.
	if !strings.Contains(text, "namespace") && !strings.Contains(text, "label") {
		t.Errorf("group expansion should mention parameters (namespace or label), got:\n%s", text)
	}
}

// TestToolGroup_DirectCallByRealName verifies that a grouped tool can be called
// directly by its real name (invokeTool fallback path).
func TestToolGroup_DirectCallByRealName(t *testing.T) {
	t.Parallel()

	s := New("k8s-server", "1.0.0")
	s.ToolGroup("kubernetes", "Kubernetes operations", func(g *Group) {
		g.Tool("get_pods", "List pods", func(_ context.Context, in k8sInput) (string, error) {
			return "pods:" + in.Namespace, nil
		})
	})
	h := s.NewTestHandler()

	resp := h.HandleRequest(context.Background(), &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      202,
		Method:  "tools/call",
		Params: mustMarshal(map[string]any{
			"name":      "get_pods",
			"arguments": map[string]any{"label": "app=nginx"},
		}),
	})
	if resp.Error != nil {
		t.Fatalf("unexpected RPC error: %v", resp.Error.Message)
	}
	result := resp.Result.(map[string]any)
	if isErr, _ := result["isError"].(bool); isErr {
		t.Error("isError should be false when calling grouped tool directly")
	}
	text := extractFirstContentText(t, result)
	// Default for namespace="default" should apply.
	if text != "pods:default" {
		t.Errorf("text = %q, want %q", text, "pods:default")
	}
}

// TestToolGroup_StandaloneAndGroupSameNamePanics verifies that a standalone
// tool and a group with the same name causes a panic.
func TestToolGroup_StandaloneAndGroupSameNamePanics(t *testing.T) {
	t.Parallel()

	s := New("panic-server", "1.0.0")
	s.Tool("ops", "Standalone ops", func(_ context.Context, in echoInput) (string, error) {
		return in.Message, nil
	})

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when group name conflicts with standalone tool name")
		}
	}()

	s.ToolGroup("ops", "Should panic", func(g *Group) {})
}

// TestToolGroup_TwoGroupsSameNamePanics verifies registering two groups with
// the same name panics.
func TestToolGroup_TwoGroupsSameNamePanics(t *testing.T) {
	t.Parallel()

	s := New("panic-server", "1.0.0")
	s.ToolGroup("workers", "First workers group", func(g *Group) {})

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for duplicate group name")
		}
	}()

	s.ToolGroup("workers", "Duplicate group", func(g *Group) {})
}

// TestToolGroup_EmptyGroup verifies that an empty group (no tools) does not
// crash and the selector can still be expanded.
func TestToolGroup_EmptyGroup(t *testing.T) {
	t.Parallel()

	s := New("empty-group-server", "1.0.0")
	s.ToolGroup("empty", "An empty group", func(g *Group) {
		// No tools registered.
	})
	h := s.NewTestHandler()

	// tools/list should still return the group selector.
	listResp := h.HandleRequest(context.Background(), &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      203,
		Method:  "tools/list",
	})
	tools := extractTools(t, listResp)
	names := toolNames(tools)
	if !names["group__empty"] {
		t.Error("empty group should still appear as a selector in tools/list")
	}

	// Expanding it should not crash.
	callResp := h.HandleRequest(context.Background(), &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      204,
		Method:  "tools/call",
		Params:  mustMarshal(map[string]any{"name": "group__empty", "arguments": map[string]any{}}),
	})
	if callResp.Error != nil {
		t.Fatalf("expanding empty group should not return RPC error: %v", callResp.Error.Message)
	}
	result := callResp.Result.(map[string]any)
	if isErr, _ := result["isError"].(bool); isErr {
		t.Error("isError should be false for empty group expansion")
	}
}

// TestToolGroup_PerToolMiddlewareRuns verifies that per-tool middleware
// registered on a grouped tool still executes.
func TestToolGroup_PerToolMiddlewareRuns(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var fired bool

	trackMW := func(next middleware.HandlerFunc) middleware.HandlerFunc {
		return func(ctx context.Context, name string, args map[string]any) (any, error) {
			mu.Lock()
			fired = true
			mu.Unlock()
			return next(ctx, name, args)
		}
	}

	s := New("mw-group-server", "1.0.0")
	s.ToolGroup("ops", "Operations", func(g *Group) {
		g.Tool("tracked_op", "A tracked operation",
			func(_ context.Context, in echoInput) (string, error) {
				return in.Message, nil
			},
			trackMW,
		)
	})
	h := s.NewTestHandler()

	h.HandleRequest(context.Background(), &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      205,
		Method:  "tools/call",
		Params:  mustMarshal(map[string]any{"name": "tracked_op", "arguments": map[string]any{"message": "hello"}}),
	})

	mu.Lock()
	defer mu.Unlock()
	if !fired {
		t.Error("per-tool middleware should run for grouped tools")
	}
}

// TestToolGroup_ValidationErrorForGroupedTool verifies that validation runs
// for grouped tools invoked directly.
func TestToolGroup_ValidationErrorForGroupedTool(t *testing.T) {
	t.Parallel()

	s := New("group-validate-server", "1.0.0")
	s.ToolGroup("ops", "Operations", func(g *Group) {
		g.Tool("strict_op", "Requires label",
			func(_ context.Context, in k8sInput) (string, error) {
				return in.Namespace, nil
			},
		)
	})
	h := s.NewTestHandler()

	// "label" is required — omitting it should cause validation failure.
	resp := h.HandleRequest(context.Background(), &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      206,
		Method:  "tools/call",
		Params: mustMarshal(map[string]any{
			"name":      "strict_op",
			"arguments": map[string]any{}, // no label
		}),
	})
	if resp.Error != nil {
		t.Fatalf("unexpected RPC error: %v", resp.Error.Message)
	}
	result := resp.Result.(map[string]any)
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("isError should be true when required field is missing in grouped tool call")
	}
}

// TestToolGroup_DefaultsApplyForGroupedTool verifies that default field values
// are applied when calling a grouped tool with missing optional fields.
func TestToolGroup_DefaultsApplyForGroupedTool(t *testing.T) {
	t.Parallel()

	s := New("group-defaults-server", "1.0.0")
	s.ToolGroup("ops", "Operations", func(g *Group) {
		g.Tool("ns_op", "Returns namespace",
			func(_ context.Context, in k8sInput) (string, error) {
				return in.Namespace, nil
			},
		)
	})
	h := s.NewTestHandler()

	resp := h.HandleRequest(context.Background(), &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      207,
		Method:  "tools/call",
		Params: mustMarshal(map[string]any{
			"name":      "ns_op",
			"arguments": map[string]any{"label": "app=nginx"}, // namespace omitted → default "default"
		}),
	})
	if resp.Error != nil {
		t.Fatalf("unexpected RPC error: %v", resp.Error.Message)
	}
	result := resp.Result.(map[string]any)
	if isErr, _ := result["isError"].(bool); isErr {
		t.Error("isError should be false when defaults cover missing fields")
	}
	text := extractFirstContentText(t, result)
	if text != "default" {
		t.Errorf("expected default namespace 'default', got %q", text)
	}
}

// TestToolGroup_CallNonexistentGroup verifies that group__nonexistent returns
// isError=true with a descriptive message.
func TestToolGroup_CallNonexistentGroup(t *testing.T) {
	t.Parallel()

	s := New("group-server", "1.0.0")
	h := s.NewTestHandler()

	resp := h.HandleRequest(context.Background(), &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      208,
		Method:  "tools/call",
		Params:  mustMarshal(map[string]any{"name": "group__noexist", "arguments": map[string]any{}}),
	})
	if resp.Error != nil {
		t.Fatalf("unexpected RPC error: %v", resp.Error.Message)
	}
	result := resp.Result.(map[string]any)
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("isError should be true for unknown group")
	}
	text := extractFirstContentText(t, result)
	if !strings.Contains(text, "noexist") {
		t.Errorf("error message should mention the group name, got: %s", text)
	}
}

// ---- Regression: Bug #1 - handleInitialize capabilities with groups only ----

// TestInitialize_CapabilitiesWithGroupsOnly is a regression test for the bug
// where a server that has ONLY tool groups (no standalone tools) would not
// advertise the "tools" capability in the initialize response.
func TestInitialize_CapabilitiesWithGroupsOnly(t *testing.T) {
	t.Parallel()

	s := New("groups-only-server", "1.0.0")
	// Register a group but NO standalone tools.
	s.ToolGroup("ops", "Operations", func(g *Group) {
		g.Tool("do_thing", "Does a thing", func(_ context.Context, in echoInput) (string, error) {
			return in.Message, nil
		})
	})
	h := s.NewTestHandler()

	resp := h.HandleRequest(context.Background(), &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      300,
		Method:  "initialize",
		Params: mustMarshal(map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "test", "version": "1.0"},
		}),
	})
	if resp.Error != nil {
		t.Fatalf("initialize error: %v", resp.Error.Message)
	}
	result := resp.Result.(map[string]any)
	caps := result["capabilities"].(map[string]any)
	if _, has := caps["tools"]; !has {
		t.Error("capabilities must include 'tools' when the server has tool groups (regression: Bug #1)")
	}
}

// ---- Minimal Schemas ---------------------------------------------------------

// TestMinimalSchemas_StripsAllAnnotationFields verifies that description,
// default, enum, minimum, maximum, pattern, and format are stripped.
func TestMinimalSchemas_StripsAllAnnotationFields(t *testing.T) {
	t.Parallel()

	s := New("minimal-server", "1.0.0", WithMinimalSchemas())
	s.Tool("rich", "Rich schema tool", func(_ context.Context, in richSchemaInput) (string, error) {
		return "ok", nil
	})
	h := s.NewTestHandler()

	resp := h.HandleRequest(context.Background(), &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      400,
		Method:  "tools/list",
	})
	if resp.Error != nil {
		t.Fatalf("tools/list error: %v", resp.Error.Message)
	}

	tools := extractTools(t, resp)
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}

	schemaData, _ := json.Marshal(tools[0]["inputSchema"])
	var schema map[string]any
	json.Unmarshal(schemaData, &schema)

	propsData, _ := json.Marshal(schema["properties"])
	var props map[string]any
	json.Unmarshal(propsData, &props)

	stripped := []string{"description", "default", "enum", "minimum", "maximum", "pattern", "format"}
	for propName, propRaw := range props {
		propData, _ := json.Marshal(propRaw)
		var propSchema map[string]any
		json.Unmarshal(propData, &propSchema)
		for _, field := range stripped {
			if _, found := propSchema[field]; found {
				t.Errorf("property %q should not have field %q in minimal schema", propName, field)
			}
		}
	}
}

// TestMinimalSchemas_KeepsTypePropertiesRequired verifies that type, properties,
// and required fields are preserved in minimal schemas.
func TestMinimalSchemas_KeepsTypePropertiesRequired(t *testing.T) {
	t.Parallel()

	s := New("minimal-server", "1.0.0", WithMinimalSchemas())
	s.Tool("rich", "Rich schema tool", func(_ context.Context, in richSchemaInput) (string, error) {
		return "ok", nil
	})
	h := s.NewTestHandler()

	resp := h.HandleRequest(context.Background(), &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      401,
		Method:  "tools/list",
	})

	tools := extractTools(t, resp)
	schemaData, _ := json.Marshal(tools[0]["inputSchema"])
	var schema map[string]any
	json.Unmarshal(schemaData, &schema)

	if schema["type"] != "object" {
		t.Errorf("schema type should be 'object', got %v", schema["type"])
	}
	if _, ok := schema["properties"]; !ok {
		t.Error("schema should preserve 'properties'")
	}
	if _, ok := schema["required"]; !ok {
		t.Error("schema should preserve 'required'")
	}
}

// TestMinimalSchemas_NestedObjectRecursivelyMinified verifies that nested
// object schemas inside properties are also minified.
type outerInput struct {
	Name    string      `json:"name" desc:"outer name" required:"true"`
	Options innerOption `json:"options" desc:"nested options"`
}

type innerOption struct {
	Timeout int    `json:"timeout" desc:"timeout in ms" default:"5000" min:"0" max:"60000"`
	Mode    string `json:"mode" desc:"mode" enum:"sync,async"`
}

func TestMinimalSchemas_NestedObjectRecursivelyMinified(t *testing.T) {
	t.Parallel()

	s := New("minimal-server", "1.0.0", WithMinimalSchemas())
	s.Tool("nested", "Tool with nested input", func(_ context.Context, in outerInput) (string, error) {
		return "ok", nil
	})
	h := s.NewTestHandler()

	resp := h.HandleRequest(context.Background(), &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      402,
		Method:  "tools/list",
	})

	tools := extractTools(t, resp)
	schemaData, _ := json.Marshal(tools[0]["inputSchema"])
	var schema map[string]any
	json.Unmarshal(schemaData, &schema)

	propsData, _ := json.Marshal(schema["properties"])
	var props map[string]any
	json.Unmarshal(propsData, &props)

	// Inspect the nested "options" property.
	optData, _ := json.Marshal(props["options"])
	var optSchema map[string]any
	json.Unmarshal(optData, &optSchema)

	// options should have type=object and its own properties.
	if optSchema["type"] != "object" {
		t.Errorf("nested options schema type = %v, want object", optSchema["type"])
	}

	// The inner properties of "options" should also be stripped.
	innerPropsData, _ := json.Marshal(optSchema["properties"])
	var innerProps map[string]any
	json.Unmarshal(innerPropsData, &innerProps)

	stripped := []string{"description", "default", "minimum", "maximum", "enum"}
	for innerPropName, innerPropRaw := range innerProps {
		innerPropData, _ := json.Marshal(innerPropRaw)
		var innerPropSchema map[string]any
		json.Unmarshal(innerPropData, &innerPropSchema)
		for _, field := range stripped {
			if _, found := innerPropSchema[field]; found {
				t.Errorf("nested property %q should not have field %q in minimal schema", innerPropName, field)
			}
		}
	}
}

// TestMinimalSchemas_FullSchemaReturnedWithoutOption verifies that without
// WithMinimalSchemas, all schema annotation fields are present (no regression).
func TestMinimalSchemas_FullSchemaReturnedWithoutOption(t *testing.T) {
	t.Parallel()

	s := New("full-schema-server", "1.0.0") // no WithMinimalSchemas
	s.Tool("rich", "Rich schema tool", func(_ context.Context, in richSchemaInput) (string, error) {
		return "ok", nil
	})
	h := s.NewTestHandler()

	resp := h.HandleRequest(context.Background(), &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      403,
		Method:  "tools/list",
	})

	tools := extractTools(t, resp)
	schemaData, _ := json.Marshal(tools[0]["inputSchema"])
	var schema map[string]any
	json.Unmarshal(schemaData, &schema)

	propsData, _ := json.Marshal(schema["properties"])
	var props map[string]any
	json.Unmarshal(propsData, &props)

	// "name" property should have description.
	nameData, _ := json.Marshal(props["name"])
	var nameProp map[string]any
	json.Unmarshal(nameData, &nameProp)
	if _, has := nameProp["description"]; !has {
		t.Error("full schema should include 'description' on properties")
	}

	// "count" should have default, minimum, maximum.
	countData, _ := json.Marshal(props["count"])
	var countProp map[string]any
	json.Unmarshal(countData, &countProp)
	for _, field := range []string{"default", "minimum", "maximum"} {
		if _, has := countProp[field]; !has {
			t.Errorf("full schema 'count' should have field %q", field)
		}
	}
}

// TestMinimalSchemas_ValidationUsesFullSchema verifies that validation still
// uses the full schema (enum, min/max constraints still enforced) even when
// WithMinimalSchemas is active.
func TestMinimalSchemas_ValidationUsesFullSchema(t *testing.T) {
	t.Parallel()

	s := New("minimal-validate-server", "1.0.0", WithMinimalSchemas())
	s.Tool("rich", "Rich schema tool", func(_ context.Context, in richSchemaInput) (string, error) {
		return "ok", nil
	})
	h := s.NewTestHandler()

	// Pass mode not in enum.
	resp := h.HandleRequest(context.Background(), &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      404,
		Method:  "tools/call",
		Params: mustMarshal(map[string]any{
			"name": "rich",
			"arguments": map[string]any{
				"name": "test",
				"mode": "turbo", // not in enum fast,slow,medium
			},
		}),
	})
	if resp.Error != nil {
		t.Fatalf("unexpected RPC error: %v", resp.Error.Message)
	}
	result := resp.Result.(map[string]any)
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("validation should still fail with enum violation when minimal schemas is on")
	}
}

// TestMinimalSchemas_MinifySchemaNilInput verifies that minifySchema handles
// nil input without panicking.
func TestMinimalSchemas_MinifySchemaDirectNil(t *testing.T) {
	t.Parallel()

	// This is an internal function test — we verify it does not panic by calling
	// through the handler path with a schema that has nil sub-schemas would be
	// unusual, so instead we call minifySchema directly via a white-box test.
	result := minifySchema(nil)
	if result != nil {
		t.Errorf("minifySchema(nil) = %v, want nil", result)
	}
}

// TestMinimalSchemas_MinifySchemaEmptyProperties verifies that an empty
// properties map is handled gracefully.
func TestMinimalSchemas_MinifySchemaEmptyProperties(t *testing.T) {
	t.Parallel()

	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
	result := minifySchema(schema)
	if result["type"] != "object" {
		t.Errorf("type should be preserved, got %v", result["type"])
	}
	props, ok := result["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties should be present")
	}
	if len(props) != 0 {
		t.Errorf("empty properties should produce empty output, got %v", props)
	}
}

// ---- EstimateSchemaTokens ---------------------------------------------------

// TestEstimateSchemaTokens_StandaloneTools verifies estimates are returned for
// all standalone tools.
func TestEstimateSchemaTokens_StandaloneTools(t *testing.T) {
	t.Parallel()

	s := New("token-server", "1.0.0")
	s.Tool("tool_a", "Tool A", func(_ context.Context, in echoInput) (string, error) { return "", nil })
	s.Tool("tool_b", "Tool B", func(_ context.Context, in echoInput) (string, error) { return "", nil })

	estimates := s.EstimateSchemaTokens()
	for _, name := range []string{"tool_a", "tool_b"} {
		if _, ok := estimates[name]; !ok {
			t.Errorf("EstimateSchemaTokens missing entry for standalone tool %q", name)
		}
	}
}

// TestEstimateSchemaTokens_GroupSelectors verifies estimates are returned for
// group selectors using the group__<name> key.
func TestEstimateSchemaTokens_GroupSelectors(t *testing.T) {
	t.Parallel()

	s := New("token-server", "1.0.0")
	s.ToolGroup("cluster", "Cluster tools", func(g *Group) {
		g.Tool("scale", "Scale workload", func(_ context.Context, in echoInput) (string, error) { return "", nil })
	})

	estimates := s.EstimateSchemaTokens()
	if _, ok := estimates["group__cluster"]; !ok {
		t.Error("EstimateSchemaTokens should contain entry for 'group__cluster'")
	}
	if _, ok := estimates["scale"]; ok {
		t.Error("EstimateSchemaTokens should NOT contain entry for individual grouped tool 'scale'")
	}
}

// TestEstimateSchemaTokens_PositiveValues verifies all estimates are positive.
func TestEstimateSchemaTokens_PositiveValues(t *testing.T) {
	t.Parallel()

	s := New("token-server", "1.0.0")
	s.Tool("ping", "Ping tool", func(_ context.Context, in echoInput) (string, error) { return "", nil })
	s.ToolGroup("ops", "Operations", func(g *Group) {
		g.Tool("op1", "Op 1", func(_ context.Context, in echoInput) (string, error) { return "", nil })
	})

	for name, tokens := range s.EstimateSchemaTokens() {
		if tokens <= 0 {
			t.Errorf("EstimateSchemaTokens[%q] = %d, want > 0", name, tokens)
		}
	}
}

// ---- Regression: Bug #2 - MaxResponseTokens(0) ------------------------------

// TestMaxResponseTokens_ZeroDisablesTruncation is a regression test for the bug
// where MaxResponseTokens(0) would truncate every non-empty string response to
// just the truncation message (because maxChars=0 and len(s)>0 is always true).
func TestMaxResponseTokens_ZeroDisablesTruncation(t *testing.T) {
	t.Parallel()

	mw := middleware.MaxResponseTokens(0)
	handler := mw(func(_ context.Context, _ string, _ map[string]any) (any, error) {
		return "hello world", nil
	})

	val, err := handler(context.Background(), "tool", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s, ok := val.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", val)
	}
	if s != "hello world" {
		t.Errorf("MaxResponseTokens(0) should be a no-op, got %q", s)
	}
}

// TestMaxResponseTokens_VerySmallLimit verifies that a very small maxTokens
// value (10 tokens = 40 chars) still truncates correctly.
func TestMaxResponseTokens_VerySmallLimit(t *testing.T) {
	t.Parallel()

	const maxTokens = 10 // 40 chars
	long := strings.Repeat("a", 200)

	mw := middleware.MaxResponseTokens(maxTokens)
	handler := mw(func(_ context.Context, _ string, _ map[string]any) (any, error) {
		return long, nil
	})

	val, err := handler(context.Background(), "tool", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s, ok := val.(string)
	if !ok {
		t.Fatalf("expected string, got %T", val)
	}
	if len(s) >= len(long) {
		t.Error("response should have been truncated")
	}
	if !strings.Contains(s, "truncated") {
		t.Errorf("truncation marker missing in: %s", s)
	}
}

// ---- CostTracker: concurrent calls ------------------------------------------

// TestCostTracker_ConcurrentCallsDoNotRace verifies that multiple concurrent
// calls through CostTracker do not trigger the race detector.
func TestCostTracker_ConcurrentCallsDoNotRace(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var allStats []middleware.CostStats

	mw := middleware.CostTracker(func(s middleware.CostStats) {
		mu.Lock()
		allStats = append(allStats, s)
		mu.Unlock()
	})

	handler := mw(func(_ context.Context, _ string, _ map[string]any) (any, error) {
		return "result", nil
	})

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			handler(context.Background(), "tool", map[string]any{"k": "v"})
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if len(allStats) != n {
		t.Errorf("expected %d CostStats, got %d", n, len(allStats))
	}
}

// TestCostTracker_MultipleSequentialCallsAccumulate verifies that the callback
// fires for each call independently and stats are consistent.
func TestCostTracker_MultipleSequentialCallsAccumulate(t *testing.T) {
	t.Parallel()

	var calls []middleware.CostStats
	mw := middleware.CostTracker(func(s middleware.CostStats) {
		calls = append(calls, s)
	})

	handler := mw(func(_ context.Context, name string, _ map[string]any) (any, error) {
		return "result-" + name, nil
	})

	for i, toolName := range []string{"alpha", "beta", "gamma"} {
		handler(context.Background(), toolName, map[string]any{"i": i})
	}

	if len(calls) != 3 {
		t.Fatalf("expected 3 CostStats, got %d", len(calls))
	}
	for i, name := range []string{"alpha", "beta", "gamma"} {
		if calls[i].ToolName != name {
			t.Errorf("call[%d].ToolName = %q, want %q", i, calls[i].ToolName, name)
		}
		if calls[i].Duration <= 0 {
			t.Errorf("call[%d].Duration should be positive", i)
		}
		if calls[i].TotalTokens != calls[i].RequestTokens+calls[i].ResponseTokens {
			t.Errorf("call[%d]: TotalTokens != RequestTokens+ResponseTokens", i)
		}
	}
}

// ---- Integration test -------------------------------------------------------

type bigInput struct {
	Query string `json:"query" desc:"search query" required:"true"`
}

type integInput struct {
	Name string `json:"name" desc:"name" required:"true"`
}

// TestIntegration_TokenOptimizationFeatures is a comprehensive integration test
// covering: WithMinimalSchemas, tool groups, CostTracker, and response truncation.
func TestIntegration_TokenOptimizationFeatures(t *testing.T) {
	t.Parallel()

	var costMu sync.Mutex
	var allCosts []middleware.CostStats

	// Server with minimal schemas and all middleware.
	s := New("integ-server", "1.0.0", WithMinimalSchemas())
	s.Use(
		middleware.CostTracker(func(c middleware.CostStats) {
			costMu.Lock()
			allCosts = append(allCosts, c)
			costMu.Unlock()
		}),
		middleware.MaxResponseTokens(20), // 80 chars limit
	)

	// 2 standalone tools.
	s.Tool("greet", "Greet a person", func(_ context.Context, in integInput) (string, error) {
		return "Hello, " + in.Name + "!", nil
	})
	s.Tool("bigdata", "Returns a large response", func(_ context.Context, in integInput) (string, error) {
		// Return ~400 chars to exceed 80-char limit.
		return strings.Repeat("data-"+in.Name+"-", 40), nil
	})

	// 1 group with 3 tools.
	s.ToolGroup("cluster", "Cluster management", func(g *Group) {
		g.Tool("list_nodes", "List cluster nodes", func(_ context.Context, in integInput) (string, error) {
			return "nodes: " + in.Name, nil
		})
		g.Tool("list_pods", "List cluster pods", func(_ context.Context, in integInput) (string, error) {
			return "pods: " + in.Name, nil
		})
		g.Tool("list_svc", "List cluster services", func(_ context.Context, in integInput) (string, error) {
			return "svc: " + in.Name, nil
		})
	})

	h := s.NewTestHandler()

	// 1. tools/list must show 2 standalone tools + 1 group selector = 3 entries.
	listResp := h.HandleRequest(context.Background(), &transport.JSONRPCRequest{
		JSONRPC: "2.0", ID: 500, Method: "tools/list",
	})
	if listResp.Error != nil {
		t.Fatalf("tools/list error: %v", listResp.Error.Message)
	}
	tools := extractTools(t, listResp)
	if len(tools) != 3 {
		t.Errorf("expected 3 entries (2 standalone + 1 group selector), got %d", len(tools))
	}
	names := toolNames(tools)
	if !names["greet"] || !names["bigdata"] || !names["group__cluster"] {
		t.Errorf("unexpected tools/list names: %v", names)
	}

	// 2. schemas in the list should be minimal.
	for _, tool := range tools {
		if tool["name"] == "group__cluster" {
			continue // group selector has its own minimal schema
		}
		schemaData, _ := json.Marshal(tool["inputSchema"])
		var schema map[string]any
		json.Unmarshal(schemaData, &schema)
		propsData, _ := json.Marshal(schema["properties"])
		var props map[string]any
		json.Unmarshal(propsData, &props)
		for propName, propRaw := range props {
			propData, _ := json.Marshal(propRaw)
			var propSchema map[string]any
			json.Unmarshal(propData, &propSchema)
			if _, found := propSchema["description"]; found {
				t.Errorf("minimal schema: property %q of tool %q should not have 'description'", propName, tool["name"])
			}
		}
	}

	// 3. Call group selector — verify sub-tool listing.
	selResp := h.HandleRequest(context.Background(), &transport.JSONRPCRequest{
		JSONRPC: "2.0", ID: 501, Method: "tools/call",
		Params: mustMarshal(map[string]any{"name": "group__cluster", "arguments": map[string]any{}}),
	})
	if selResp.Error != nil {
		t.Fatalf("group selector error: %v", selResp.Error.Message)
	}
	selResult := selResp.Result.(map[string]any)
	if isErr, _ := selResult["isError"].(bool); isErr {
		t.Error("group selector isError should be false")
	}
	selText := extractFirstContentText(t, selResult)
	for _, subTool := range []string{"list_nodes", "list_pods", "list_svc"} {
		if !strings.Contains(selText, subTool) {
			t.Errorf("group expansion should mention %q; got:\n%s", subTool, selText)
		}
	}

	// 4. Call a grouped tool with valid args.
	nodeResp := h.HandleRequest(context.Background(), &transport.JSONRPCRequest{
		JSONRPC: "2.0", ID: 502, Method: "tools/call",
		Params: mustMarshal(map[string]any{"name": "list_nodes", "arguments": map[string]any{"name": "prod"}}),
	})
	if nodeResp.Error != nil {
		t.Fatalf("list_nodes error: %v", nodeResp.Error.Message)
	}
	nodeResult := nodeResp.Result.(map[string]any)
	if isErr, _ := nodeResult["isError"].(bool); isErr {
		t.Error("list_nodes isError should be false")
	}
	nodeText := extractFirstContentText(t, nodeResult)
	if nodeText != "nodes: prod" {
		t.Errorf("list_nodes text = %q, want %q", nodeText, "nodes: prod")
	}

	// 5. Call a grouped tool with invalid args (missing required "name").
	badResp := h.HandleRequest(context.Background(), &transport.JSONRPCRequest{
		JSONRPC: "2.0", ID: 503, Method: "tools/call",
		Params: mustMarshal(map[string]any{"name": "list_nodes", "arguments": map[string]any{}}),
	})
	badResult := badResp.Result.(map[string]any)
	if isErr, _ := badResult["isError"].(bool); !isErr {
		t.Error("list_nodes with missing required arg should have isError=true")
	}

	// 6. Call standalone tool that returns large response — verify truncation.
	bigResp := h.HandleRequest(context.Background(), &transport.JSONRPCRequest{
		JSONRPC: "2.0", ID: 504, Method: "tools/call",
		Params: mustMarshal(map[string]any{"name": "bigdata", "arguments": map[string]any{"name": "x"}}),
	})
	bigResult := bigResp.Result.(map[string]any)
	if isErr, _ := bigResult["isError"].(bool); isErr {
		t.Error("bigdata isError should be false")
	}
	bigText := extractFirstContentText(t, bigResult)
	if !strings.Contains(bigText, "truncated") {
		t.Errorf("large response should be truncated; got:\n%s", bigText)
	}

	// 7. Verify cost tracker captured all calls.
	// We made: group selector (no middleware runs for it), list_nodes (valid),
	// list_nodes (invalid — validation fails before handler but CostTracker wraps
	// invokeTool which validates before calling handler, so cost callback won't
	// fire for validation failures — that is expected behaviour).
	// greet was never called. bigdata was called.
	// Calls that go through the middleware chain: list_nodes (valid) + bigdata = 2.
	costMu.Lock()
	capturedCosts := len(allCosts)
	costMu.Unlock()

	// At minimum, bigdata and list_nodes(valid) must have fired the callback.
	if capturedCosts < 2 {
		t.Errorf("expected at least 2 cost tracker entries, got %d", capturedCosts)
	}
	for i, c := range allCosts {
		if c.Duration <= 0 {
			t.Errorf("allCosts[%d].Duration should be positive, got %v", i, c.Duration)
		}
		if c.TotalTokens != c.RequestTokens+c.ResponseTokens {
			t.Errorf("allCosts[%d]: TotalTokens mismatch", i)
		}
	}
}

// ---- helpers for new feature tests ------------------------------------------

// extractTools decodes the tools slice from a tools/list response.
func extractTools(t *testing.T, resp *transport.JSONRPCResponse) []map[string]any {
	t.Helper()
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("result is not a map, got %T", resp.Result)
	}
	data, _ := json.Marshal(result["tools"])
	var tools []map[string]any
	if err := json.Unmarshal(data, &tools); err != nil {
		t.Fatalf("failed to decode tools: %v", err)
	}
	return tools
}

// toolNames builds a name→bool map from a tools slice for membership checks.
func toolNames(tools []map[string]any) map[string]bool {
	names := make(map[string]bool, len(tools))
	for _, tool := range tools {
		if name, ok := tool["name"].(string); ok {
			names[name] = true
		}
	}
	return names
}

// extractFirstContentText pulls the text from the first content block of a tool
// call result.
func extractFirstContentText(t *testing.T, result map[string]any) string {
	t.Helper()
	data, _ := json.Marshal(result["content"])
	var content []map[string]any
	if err := json.Unmarshal(data, &content); err != nil {
		t.Fatalf("failed to decode content: %v", err)
	}
	if len(content) == 0 {
		t.Fatal("content slice is empty")
	}
	text, _ := content[0]["text"].(string)
	return text
}
