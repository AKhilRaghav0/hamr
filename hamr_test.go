package hamr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/AKhilRaghav0/hamr/middleware"
	"github.com/AKhilRaghav0/hamr/transport"
)

// ---- shared test server setup -----------------------------------------------

type echoInput struct {
	Message string `json:"message" desc:"message to echo" required:"true"`
}

type addInput struct {
	A float64 `json:"a" desc:"first operand" required:"true"`
	B float64 `json:"b" desc:"second operand" required:"true"`
}

type greetInput struct {
	Name     string `json:"name" desc:"person to greet" required:"true"`
	Greeting string `json:"greeting" desc:"greeting word" default:"Hello" enum:"Hello,Hi,Hey"`
}

type defaultsInput struct {
	Count  int    `json:"count" desc:"item count" default:"5"`
	Format string `json:"format" desc:"output format" default:"text"`
}

// newTestServer builds a server with a small set of tools suited for integration
// testing. All registrations happen in this one place so individual tests don't
// repeat setup code.
func newTestServer() *Server {
	s := New("integration-test-server", "0.1.0")

	s.Tool("echo", "Echo the input message back to the caller", func(_ context.Context, in echoInput) (string, error) {
		return in.Message, nil
	})

	s.Tool("add", "Add two numbers", func(_ context.Context, in addInput) (string, error) {
		return fmt.Sprintf("%g", in.A+in.B), nil
	})

	s.Tool("greet", "Greet a person", func(_ context.Context, in greetInput) (string, error) {
		return fmt.Sprintf("%s, %s!", in.Greeting, in.Name), nil
	})

	s.Tool("defaults_demo", "Demonstrate default field values", func(_ context.Context, in defaultsInput) (string, error) {
		return fmt.Sprintf("count=%d format=%s", in.Count, in.Format), nil
	})

	s.Tool("fail_tool", "Always returns an error", func(_ context.Context, in echoInput) (string, error) {
		return "", errors.New("intentional failure")
	})

	return s
}

// callRaw sends a raw JSON-RPC request string and returns the response.
func callRaw(h transport.Handler, raw string) *transport.JSONRPCResponse {
	var req transport.JSONRPCRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		panic("callRaw: bad JSON: " + err.Error())
	}
	return h.HandleRequest(context.Background(), &req)
}

// mustMarshal encodes v to JSON, panicking on error (test helper).
func mustMarshal(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// ---- TestInitialize ---------------------------------------------------------

func TestInitialize(t *testing.T) {
	h := newTestServer().NewTestHandler()

	req := &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params: mustMarshal(map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "test", "version": "1.0"},
		}),
	}

	resp := h.HandleRequest(context.Background(), req)

	if resp.Error != nil {
		t.Fatalf("initialize returned error: %v", resp.Error.Message)
	}
	if resp.Result == nil {
		t.Fatal("initialize returned nil result")
	}

	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("result is not a map, got %T", resp.Result)
	}

	// Verify protocol version.
	if got := result["protocolVersion"]; got != mcpProtocolVersion {
		t.Errorf("protocolVersion = %q, want %q", got, mcpProtocolVersion)
	}

	// Verify server info.
	serverInfo, ok := result["serverInfo"].(map[string]any)
	if !ok {
		t.Fatal("serverInfo missing or wrong type")
	}
	if name := serverInfo["name"]; name != "integration-test-server" {
		t.Errorf("server name = %q, want %q", name, "integration-test-server")
	}
	if version := serverInfo["version"]; version != "0.1.0" {
		t.Errorf("server version = %q, want %q", version, "0.1.0")
	}

	// Verify capabilities include tools (we registered several).
	caps, ok := result["capabilities"].(map[string]any)
	if !ok {
		t.Fatal("capabilities missing or wrong type")
	}
	if _, hasCaps := caps["tools"]; !hasCaps {
		t.Error("capabilities should contain 'tools' key")
	}
}

// ---- TestListTools ----------------------------------------------------------

func TestListTools(t *testing.T) {
	s := newTestServer()
	h := s.NewTestHandler()

	req := &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/list",
	}

	resp := h.HandleRequest(context.Background(), req)

	if resp.Error != nil {
		t.Fatalf("tools/list error: %v", resp.Error.Message)
	}

	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("result is not a map, got %T", resp.Result)
	}

	toolsRaw, ok := result["tools"]
	if !ok {
		t.Fatal("result missing 'tools' key")
	}

	// Re-encode and decode into a typed slice.
	data, _ := json.Marshal(toolsRaw)
	var tools []map[string]any
	if err := json.Unmarshal(data, &tools); err != nil {
		t.Fatalf("failed to parse tools: %v", err)
	}

	// We registered 5 tools in newTestServer.
	if len(tools) != 5 {
		t.Errorf("expected 5 tools, got %d", len(tools))
	}

	// Build name set for membership checks.
	names := make(map[string]bool, len(tools))
	for _, tool := range tools {
		name, _ := tool["name"].(string)
		names[name] = true

		// Every tool must have name, description, and inputSchema.
		if name == "" {
			t.Error("tool missing name")
		}
		if _, ok := tool["description"]; !ok {
			t.Errorf("tool %q missing description", name)
		}
		if _, ok := tool["inputSchema"]; !ok {
			t.Errorf("tool %q missing inputSchema", name)
		}
	}

	expected := []string{"echo", "add", "greet", "defaults_demo", "fail_tool"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("expected tool %q not found in list", name)
		}
	}
}

// ---- TestCallTool -----------------------------------------------------------

func TestCallTool(t *testing.T) {
	h := newTestServer().NewTestHandler()

	req := &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "tools/call",
		Params: mustMarshal(map[string]any{
			"name":      "echo",
			"arguments": map[string]any{"message": "hello world"},
		}),
	}

	resp := h.HandleRequest(context.Background(), req)

	if resp.Error != nil {
		t.Fatalf("tools/call error: %v", resp.Error.Message)
	}

	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("result is not a map, got %T", resp.Result)
	}

	// isError must be false for a successful call.
	if isErr, _ := result["isError"].(bool); isErr {
		t.Error("isError should be false for a successful tool call")
	}

	// Extract text content.
	data, _ := json.Marshal(result["content"])
	var content []map[string]any
	if err := json.Unmarshal(data, &content); err != nil {
		t.Fatalf("failed to parse content: %v", err)
	}
	if len(content) == 0 {
		t.Fatal("content is empty")
	}
	if text, _ := content[0]["text"].(string); text != "hello world" {
		t.Errorf("text = %q, want %q", text, "hello world")
	}
}

// TestCallTool_AddNumbers verifies numeric tool invocation and result formatting.
func TestCallTool_AddNumbers(t *testing.T) {
	h := newTestServer().NewTestHandler()

	req := &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      4,
		Method:  "tools/call",
		Params: mustMarshal(map[string]any{
			"name":      "add",
			"arguments": map[string]any{"a": 3, "b": 4},
		}),
	}

	resp := h.HandleRequest(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("tools/call error: %v", resp.Error.Message)
	}

	result := resp.Result.(map[string]any)
	data, _ := json.Marshal(result["content"])
	var content []map[string]any
	json.Unmarshal(data, &content)

	if text, _ := content[0]["text"].(string); text != "7" {
		t.Errorf("add(3,4) = %q, want %q", text, "7")
	}
}

// ---- TestCallToolValidation -------------------------------------------------

func TestCallToolValidation(t *testing.T) {
	h := newTestServer().NewTestHandler()

	// Call "echo" without the required "message" field.
	req := &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      5,
		Method:  "tools/call",
		Params: mustMarshal(map[string]any{
			"name":      "echo",
			"arguments": map[string]any{},
		}),
	}

	resp := h.HandleRequest(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("unexpected RPC error: %v", resp.Error.Message)
	}

	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("result is not a map")
	}

	// A validation error surfaces as a tool result with isError=true.
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("isError should be true when required field is missing")
	}

	data, _ := json.Marshal(result["content"])
	var content []map[string]any
	json.Unmarshal(data, &content)
	if len(content) == 0 {
		t.Fatal("error content is empty")
	}
	text, _ := content[0]["text"].(string)
	if text == "" {
		t.Error("error message should not be empty")
	}
}

// TestCallToolValidation_WrongType passes a string where an integer is expected.
func TestCallToolValidation_WrongType(t *testing.T) {
	h := newTestServer().NewTestHandler()

	req := &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      6,
		Method:  "tools/call",
		Params: mustMarshal(map[string]any{
			"name":      "add",
			"arguments": map[string]any{"a": "not-a-number", "b": 4},
		}),
	}

	resp := h.HandleRequest(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("unexpected RPC error: %v", resp.Error.Message)
	}

	result := resp.Result.(map[string]any)
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("isError should be true when field has wrong type")
	}
}

// TestCallToolValidation_Enum passes a value not in the allowed enum.
func TestCallToolValidation_Enum(t *testing.T) {
	h := newTestServer().NewTestHandler()

	req := &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      7,
		Method:  "tools/call",
		Params: mustMarshal(map[string]any{
			"name": "greet",
			"arguments": map[string]any{
				"name":     "Alice",
				"greeting": "Howdy", // not in enum Hello,Hi,Hey
			},
		}),
	}

	resp := h.HandleRequest(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("unexpected RPC error: %v", resp.Error.Message)
	}

	result := resp.Result.(map[string]any)
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("isError should be true when enum constraint is violated")
	}
}

// ---- TestCallToolMissing ----------------------------------------------------

func TestCallToolMissing(t *testing.T) {
	h := newTestServer().NewTestHandler()

	req := &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      8,
		Method:  "tools/call",
		Params: mustMarshal(map[string]any{
			"name":      "nonexistent_tool",
			"arguments": map[string]any{},
		}),
	}

	resp := h.HandleRequest(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("unexpected RPC error: %v", resp.Error.Message)
	}

	result := resp.Result.(map[string]any)
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("isError should be true for an unknown tool")
	}

	data, _ := json.Marshal(result["content"])
	var content []map[string]any
	json.Unmarshal(data, &content)
	text, _ := content[0]["text"].(string)
	if text == "" {
		t.Error("error message should not be empty for unknown tool")
	}
}

// ---- TestCallToolWithDefaults -----------------------------------------------

func TestCallToolWithDefaults(t *testing.T) {
	h := newTestServer().NewTestHandler()

	// Call "defaults_demo" with no arguments; the schema supplies defaults.
	req := &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      9,
		Method:  "tools/call",
		Params: mustMarshal(map[string]any{
			"name":      "defaults_demo",
			"arguments": map[string]any{},
		}),
	}

	resp := h.HandleRequest(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("tools/call error: %v", resp.Error.Message)
	}

	result := resp.Result.(map[string]any)
	if isErr, _ := result["isError"].(bool); isErr {
		t.Error("isError should be false when defaults satisfy requirements")
	}

	data, _ := json.Marshal(result["content"])
	var content []map[string]any
	json.Unmarshal(data, &content)
	text, _ := content[0]["text"].(string)

	// Both count=5 and format=text must be present (from defaults).
	if text != "count=5 format=text" {
		t.Errorf("text = %q, want %q", text, "count=5 format=text")
	}
}

// ---- TestCallToolWithDefaults_Override --------------------------------------

// TestCallToolWithDefaults_Override checks that explicit args take precedence
// over schema defaults.
func TestCallToolWithDefaults_Override(t *testing.T) {
	h := newTestServer().NewTestHandler()

	req := &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      10,
		Method:  "tools/call",
		Params: mustMarshal(map[string]any{
			"name":      "defaults_demo",
			"arguments": map[string]any{"count": 99, "format": "json"},
		}),
	}

	resp := h.HandleRequest(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("tools/call error: %v", resp.Error.Message)
	}

	result := resp.Result.(map[string]any)
	data, _ := json.Marshal(result["content"])
	var content []map[string]any
	json.Unmarshal(data, &content)
	text, _ := content[0]["text"].(string)

	if text != "count=99 format=json" {
		t.Errorf("text = %q, want %q", text, "count=99 format=json")
	}
}

// ---- TestMiddlewareExecution ------------------------------------------------

func TestMiddlewareExecution(t *testing.T) {
	var mu sync.Mutex
	var order []string

	appendOrder := func(label string) {
		mu.Lock()
		order = append(order, label)
		mu.Unlock()
	}

	makeMiddleware := func(name string) middleware.Middleware {
		return func(next middleware.HandlerFunc) middleware.HandlerFunc {
			return func(ctx context.Context, toolName string, args map[string]any) (any, error) {
				appendOrder(name + ":before")
				result, err := next(ctx, toolName, args)
				appendOrder(name + ":after")
				return result, err
			}
		}
	}

	s := New("mw-test-server", "1.0.0")
	s.Use(makeMiddleware("global-A"), makeMiddleware("global-B"))

	s.Tool("tracked", "A tool with both global and per-tool middleware",
		func(_ context.Context, in echoInput) (string, error) {
			appendOrder("handler")
			return in.Message, nil
		},
		makeMiddleware("tool-C"),
	)

	h := s.NewTestHandler()

	req := &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      11,
		Method:  "tools/call",
		Params: mustMarshal(map[string]any{
			"name":      "tracked",
			"arguments": map[string]any{"message": "mw test"},
		}),
	}

	h.HandleRequest(context.Background(), req)

	// Global middleware wraps tool-specific middleware; global runs outermost.
	// Chain order: global-A → global-B → tool-C → handler
	want := []string{
		"global-A:before",
		"global-B:before",
		"tool-C:before",
		"handler",
		"tool-C:after",
		"global-B:after",
		"global-A:after",
	}

	mu.Lock()
	got := order
	mu.Unlock()

	if len(got) != len(want) {
		t.Fatalf("middleware execution order len=%d want=%d\ngot:  %v\nwant: %v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("step %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

// ---- TestMethodNotFound -----------------------------------------------------

func TestMethodNotFound(t *testing.T) {
	h := newTestServer().NewTestHandler()

	req := &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      12,
		Method:  "unknown/method",
	}

	resp := h.HandleRequest(context.Background(), req)
	if resp.Error == nil {
		t.Fatal("expected error for unknown method, got nil")
	}
	if resp.Error.Code != transport.CodeMethodNotFound {
		t.Errorf("error code = %d, want %d", resp.Error.Code, transport.CodeMethodNotFound)
	}
}

// ---- TestCallTool_ToolHandlerError ------------------------------------------

// TestCallTool_ToolHandlerError verifies that a handler returning an error is
// surfaced as isError=true in the tool result (not an RPC-level error).
func TestCallTool_ToolHandlerError(t *testing.T) {
	h := newTestServer().NewTestHandler()

	req := &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      13,
		Method:  "tools/call",
		Params: mustMarshal(map[string]any{
			"name":      "fail_tool",
			"arguments": map[string]any{"message": "trigger failure"},
		}),
	}

	resp := h.HandleRequest(context.Background(), req)
	// The RPC call itself should not fail.
	if resp.Error != nil {
		t.Fatalf("unexpected RPC error: %v", resp.Error.Message)
	}

	result := resp.Result.(map[string]any)
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Error("isError should be true when handler returns an error")
	}
}

// ---- TestListTools_EmptyServer ----------------------------------------------

func TestListTools_EmptyServer(t *testing.T) {
	s := New("empty-server", "1.0.0")
	h := s.NewTestHandler()

	req := &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      14,
		Method:  "tools/list",
	}

	resp := h.HandleRequest(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("tools/list error: %v", resp.Error.Message)
	}

	result := resp.Result.(map[string]any)
	data, _ := json.Marshal(result["tools"])
	var tools []map[string]any
	json.Unmarshal(data, &tools)

	if len(tools) != 0 {
		t.Errorf("expected 0 tools on empty server, got %d", len(tools))
	}
}

// ---- TestNotification -------------------------------------------------------

func TestNotification_InitializedDoesNotPanic(t *testing.T) {
	h := newTestServer().NewTestHandler()

	// notifications/initialized must be handled without panicking.
	notif := &transport.JSONRPCNotification{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	// The handler returns nothing; we just verify it does not panic.
	h.HandleNotification(context.Background(), notif)
}

func TestNotification_UnknownIsIgnored(t *testing.T) {
	h := newTestServer().NewTestHandler()

	notif := &transport.JSONRPCNotification{
		JSONRPC: "2.0",
		Method:  "notifications/some_unknown_event",
	}
	h.HandleNotification(context.Background(), notif)
}

// ---- TestIDPreserved --------------------------------------------------------

// TestIDPreserved verifies that the response ID mirrors the request ID for
// both numeric and string forms.
func TestIDPreserved(t *testing.T) {
	h := newTestServer().NewTestHandler()

	for _, id := range []any{42, "req-abc-123"} {
		id := id
		t.Run(fmt.Sprintf("%v", id), func(t *testing.T) {
			req := &transport.JSONRPCRequest{
				JSONRPC: "2.0",
				ID:      id,
				Method:  "tools/list",
			}
			resp := h.HandleRequest(context.Background(), req)
			if resp.ID != id {
				t.Errorf("response ID = %v, want %v", resp.ID, id)
			}
		})
	}
}

// ---- TestWithDescription ----------------------------------------------------

// TestWithDescription verifies that WithDescription sets the description field
// in the server-info block of the initialize response.
func TestWithDescription(t *testing.T) {
	s := New("desc-server", "1.0.0", WithDescription("A test server with a description"))
	h := s.NewTestHandler()

	req := &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      20,
		Method:  "initialize",
		Params: mustMarshal(map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "test", "version": "1.0"},
		}),
	}

	resp := h.HandleRequest(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("initialize returned error: %v", resp.Error.Message)
	}

	result := resp.Result.(map[string]any)
	serverInfo, ok := result["serverInfo"].(map[string]any)
	if !ok {
		t.Fatal("serverInfo missing or wrong type")
	}
	if desc, _ := serverInfo["description"].(string); desc != "A test server with a description" {
		t.Errorf("description = %q, want %q", desc, "A test server with a description")
	}
}

// TestWithDescription_Empty verifies that when no description is set, the
// serverInfo block does not contain a description key.
func TestWithDescription_Empty(t *testing.T) {
	s := New("no-desc-server", "1.0.0")
	h := s.NewTestHandler()

	req := &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      21,
		Method:  "initialize",
		Params: mustMarshal(map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "test", "version": "1.0"},
		}),
	}

	resp := h.HandleRequest(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("initialize error: %v", resp.Error.Message)
	}

	result := resp.Result.(map[string]any)
	serverInfo := result["serverInfo"].(map[string]any)
	if _, has := serverInfo["description"]; has {
		t.Error("description should not be present when not set")
	}
}

// ---- TestResource registration and listing ----------------------------------

func TestResource_ListAndRead(t *testing.T) {
	s := New("resource-server", "1.0.0")
	s.Resource("file://config.json", "config", "Server configuration", func(ctx context.Context) (string, error) {
		return `{"env":"test"}`, nil
	})
	h := s.NewTestHandler()

	// List resources.
	listReq := &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      30,
		Method:  "resources/list",
	}
	listResp := h.HandleRequest(context.Background(), listReq)
	if listResp.Error != nil {
		t.Fatalf("resources/list error: %v", listResp.Error.Message)
	}

	listResult := listResp.Result.(map[string]any)
	data, _ := json.Marshal(listResult["resources"])
	var resources []map[string]any
	json.Unmarshal(data, &resources)
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
	if resources[0]["uri"] != "file://config.json" {
		t.Errorf("unexpected URI: %v", resources[0]["uri"])
	}
	if resources[0]["name"] != "config" {
		t.Errorf("unexpected name: %v", resources[0]["name"])
	}

	// Read resource.
	readReq := &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      31,
		Method:  "resources/read",
		Params:  mustMarshal(map[string]any{"uri": "file://config.json"}),
	}
	readResp := h.HandleRequest(context.Background(), readReq)
	if readResp.Error != nil {
		t.Fatalf("resources/read error: %v", readResp.Error.Message)
	}

	readResult := readResp.Result.(map[string]any)
	contentsData, _ := json.Marshal(readResult["contents"])
	var contents []map[string]any
	json.Unmarshal(contentsData, &contents)
	if len(contents) == 0 {
		t.Fatal("expected at least one content block")
	}
	if text, _ := contents[0]["text"].(string); text != `{"env":"test"}` {
		t.Errorf("resource text = %q, want %q", text, `{"env":"test"}`)
	}
}

// TestResource_ReadUnknown verifies that reading a non-existent resource returns an error.
func TestResource_ReadUnknown(t *testing.T) {
	s := New("resource-server", "1.0.0")
	h := s.NewTestHandler()

	req := &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      32,
		Method:  "resources/read",
		Params:  mustMarshal(map[string]any{"uri": "file://nonexistent"}),
	}
	resp := h.HandleRequest(context.Background(), req)
	if resp.Error == nil {
		t.Fatal("expected error for unknown resource, got nil")
	}
	if resp.Error.Code != transport.CodeInvalidParams {
		t.Errorf("error code = %d, want CodeInvalidParams", resp.Error.Code)
	}
}

// TestResource_ReadInvalidParams verifies that malformed params return an error.
func TestResource_ReadInvalidParams(t *testing.T) {
	s := New("resource-server", "1.0.0")
	h := s.NewTestHandler()

	req := &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      33,
		Method:  "resources/read",
		Params:  json.RawMessage(`not-json`),
	}
	resp := h.HandleRequest(context.Background(), req)
	if resp.Error == nil {
		t.Fatal("expected error for invalid params")
	}
}

// TestResource_HandlerError verifies that a resource handler returning an error
// is surfaced as an RPC error.
func TestResource_HandlerError(t *testing.T) {
	s := New("resource-server", "1.0.0")
	s.Resource("file://fail", "fail", "Always fails", func(ctx context.Context) (string, error) {
		return "", errors.New("resource handler failure")
	})
	h := s.NewTestHandler()

	req := &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      34,
		Method:  "resources/read",
		Params:  mustMarshal(map[string]any{"uri": "file://fail"}),
	}
	resp := h.HandleRequest(context.Background(), req)
	if resp.Error == nil {
		t.Fatal("expected RPC error from failing resource handler")
	}
	if resp.Error.Code != transport.CodeInternalError {
		t.Errorf("error code = %d, want CodeInternalError", resp.Error.Code)
	}
}

// TestResource_DuplicatePanics verifies that registering a resource under the
// same URI twice panics.
func TestResource_DuplicatePanics(t *testing.T) {
	s := New("resource-server", "1.0.0")
	s.Resource("file://dup", "dup", "First registration", func(ctx context.Context) (string, error) {
		return "ok", nil
	})

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for duplicate resource registration")
		}
	}()

	s.Resource("file://dup", "dup2", "Second registration", func(ctx context.Context) (string, error) {
		return "ok", nil
	})
}

// TestResource_InitializeCapabilities verifies that the capabilities block
// contains "resources" when a resource is registered.
func TestResource_InitializeCapabilities(t *testing.T) {
	s := New("resource-server", "1.0.0")
	s.Resource("file://x", "x", "x", func(ctx context.Context) (string, error) { return "", nil })
	h := s.NewTestHandler()

	req := &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      35,
		Method:  "initialize",
		Params: mustMarshal(map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "t", "version": "1"},
		}),
	}
	resp := h.HandleRequest(context.Background(), req)
	result := resp.Result.(map[string]any)
	caps := result["capabilities"].(map[string]any)
	if _, has := caps["resources"]; !has {
		t.Error("capabilities should contain 'resources' when resources are registered")
	}
}

// ---- TestPrompt registration, listing, and getting --------------------------

func TestPrompt_ListAndGet(t *testing.T) {
	s := New("prompt-server", "1.0.0")
	s.Prompt("summarize", "Summarize text", func(ctx context.Context, args map[string]string) (string, error) {
		text := args["text"]
		return fmt.Sprintf("Please summarize: %s", text), nil
	})
	h := s.NewTestHandler()

	// List prompts.
	listReq := &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      40,
		Method:  "prompts/list",
	}
	listResp := h.HandleRequest(context.Background(), listReq)
	if listResp.Error != nil {
		t.Fatalf("prompts/list error: %v", listResp.Error.Message)
	}

	listResult := listResp.Result.(map[string]any)
	data, _ := json.Marshal(listResult["prompts"])
	var prompts []map[string]any
	json.Unmarshal(data, &prompts)
	if len(prompts) != 1 {
		t.Fatalf("expected 1 prompt, got %d", len(prompts))
	}
	if prompts[0]["name"] != "summarize" {
		t.Errorf("unexpected prompt name: %v", prompts[0]["name"])
	}

	// Get prompt.
	getReq := &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      41,
		Method:  "prompts/get",
		Params:  mustMarshal(map[string]any{"name": "summarize", "arguments": map[string]string{"text": "hello world"}}),
	}
	getResp := h.HandleRequest(context.Background(), getReq)
	if getResp.Error != nil {
		t.Fatalf("prompts/get error: %v", getResp.Error.Message)
	}

	getResult := getResp.Result.(map[string]any)
	msgsData, _ := json.Marshal(getResult["messages"])
	var messages []map[string]any
	json.Unmarshal(msgsData, &messages)
	if len(messages) == 0 {
		t.Fatal("expected at least one message in prompt result")
	}
	content := messages[0]["content"].(map[string]any)
	if text, _ := content["text"].(string); text != "Please summarize: hello world" {
		t.Errorf("prompt text = %q, want %q", text, "Please summarize: hello world")
	}
}

// TestPrompt_GetUnknown verifies that getting a non-existent prompt returns an error.
func TestPrompt_GetUnknown(t *testing.T) {
	s := New("prompt-server", "1.0.0")
	h := s.NewTestHandler()

	req := &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      42,
		Method:  "prompts/get",
		Params:  mustMarshal(map[string]any{"name": "nonexistent", "arguments": map[string]string{}}),
	}
	resp := h.HandleRequest(context.Background(), req)
	if resp.Error == nil {
		t.Fatal("expected error for unknown prompt")
	}
	if resp.Error.Code != transport.CodeInvalidParams {
		t.Errorf("error code = %d, want CodeInvalidParams", resp.Error.Code)
	}
}

// TestPrompt_GetInvalidParams verifies that malformed params return an error.
func TestPrompt_GetInvalidParams(t *testing.T) {
	s := New("prompt-server", "1.0.0")
	h := s.NewTestHandler()

	req := &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      43,
		Method:  "prompts/get",
		Params:  json.RawMessage(`not-json`),
	}
	resp := h.HandleRequest(context.Background(), req)
	if resp.Error == nil {
		t.Fatal("expected error for invalid params")
	}
}

// TestPrompt_HandlerError verifies that a prompt handler returning an error
// is surfaced as an RPC error.
func TestPrompt_HandlerError(t *testing.T) {
	s := New("prompt-server", "1.0.0")
	s.Prompt("fail_prompt", "Always fails", func(ctx context.Context, args map[string]string) (string, error) {
		return "", errors.New("prompt handler failure")
	})
	h := s.NewTestHandler()

	req := &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      44,
		Method:  "prompts/get",
		Params:  mustMarshal(map[string]any{"name": "fail_prompt", "arguments": map[string]string{}}),
	}
	resp := h.HandleRequest(context.Background(), req)
	if resp.Error == nil {
		t.Fatal("expected error from failing prompt handler")
	}
	if resp.Error.Code != transport.CodeInternalError {
		t.Errorf("error code = %d, want CodeInternalError", resp.Error.Code)
	}
}

// TestPrompt_DuplicatePanics verifies that registering a prompt with the same name twice panics.
func TestPrompt_DuplicatePanics(t *testing.T) {
	s := New("prompt-server", "1.0.0")
	s.Prompt("dup", "First", func(ctx context.Context, args map[string]string) (string, error) {
		return "ok", nil
	})

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for duplicate prompt registration")
		}
	}()

	s.Prompt("dup", "Second", func(ctx context.Context, args map[string]string) (string, error) {
		return "ok", nil
	})
}

// TestPrompt_InitializeCapabilities verifies that capabilities contains
// "prompts" when a prompt is registered.
func TestPrompt_InitializeCapabilities(t *testing.T) {
	s := New("prompt-server", "1.0.0")
	s.Prompt("p", "p", func(ctx context.Context, args map[string]string) (string, error) { return "", nil })
	h := s.NewTestHandler()

	req := &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      45,
		Method:  "initialize",
		Params: mustMarshal(map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "t", "version": "1"},
		}),
	}
	resp := h.HandleRequest(context.Background(), req)
	result := resp.Result.(map[string]any)
	caps := result["capabilities"].(map[string]any)
	if _, has := caps["prompts"]; !has {
		t.Error("capabilities should contain 'prompts' when prompts are registered")
	}
}

// ---- TestDuplicateTool_Panics -----------------------------------------------

func TestDuplicateTool_Panics(t *testing.T) {
	s := New("dup-tool-server", "1.0.0")
	s.Tool("mytool", "First registration", func(_ context.Context, in echoInput) (string, error) {
		return in.Message, nil
	})

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for duplicate tool registration")
		}
	}()

	s.Tool("mytool", "Second registration", func(_ context.Context, in echoInput) (string, error) {
		return in.Message, nil
	})
}

// ---- TestInvalidHandler_Panics ----------------------------------------------

func TestInvalidHandler_NotAFunc_Panics(t *testing.T) {
	s := New("panic-server", "1.0.0")
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for non-function handler")
		}
	}()
	s.Tool("bad", "A bad tool", "not a function")
}

func TestInvalidHandler_WrongParamCount_Panics(t *testing.T) {
	s := New("panic-server", "1.0.0")
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for handler with wrong parameter count")
		}
	}()
	// Single param (missing context) should panic.
	s.Tool("bad", "A bad tool", func(in echoInput) (string, error) {
		return "", nil
	})
}

func TestInvalidHandler_NonStructInput_Panics(t *testing.T) {
	s := New("panic-server", "1.0.0")
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for non-struct input type")
		}
	}()
	s.Tool("bad", "A bad tool", func(_ context.Context, in string) (string, error) {
		return "", nil
	})
}

func TestInvalidHandler_WrongReturnCount_Panics(t *testing.T) {
	s := New("panic-server", "1.0.0")
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for handler with wrong return count")
		}
	}()
	s.Tool("bad", "A bad tool", func(_ context.Context, in echoInput) string {
		return ""
	})
}

func TestInvalidHandler_BadReturnType_Panics(t *testing.T) {
	s := New("panic-server", "1.0.0")
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for handler with unsupported first return type")
		}
	}()
	// int is not string, []Content, or Result.
	s.Tool("bad", "A bad tool", func(_ context.Context, in echoInput) (int, error) {
		return 0, nil
	})
}

func TestInvalidHandler_EmptyName_Panics(t *testing.T) {
	s := New("panic-server", "1.0.0")
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for empty tool name")
		}
	}()
	s.Tool("", "A bad tool", func(_ context.Context, in echoInput) (string, error) {
		return "", nil
	})
}

func TestInvalidHandler_EmptyDescription_Panics(t *testing.T) {
	s := New("panic-server", "1.0.0")
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for empty tool description")
		}
	}()
	s.Tool("bad", "", func(_ context.Context, in echoInput) (string, error) {
		return "", nil
	})
}

// ---- TestReturnStyles -------------------------------------------------------

// contentReturnInput is the input type for content-returning tools.
type contentReturnInput struct {
	Label string `json:"label" desc:"label for the content"`
}

// TestCallTool_ContentReturn verifies that a handler returning []Content
// is correctly marshalled.
func TestCallTool_ContentReturn(t *testing.T) {
	s := New("content-server", "1.0.0")
	s.Tool("multi_content", "Returns multiple content blocks", func(_ context.Context, in contentReturnInput) ([]Content, error) {
		return []Content{
			TextContent("block one: " + in.Label),
			TextContent("block two"),
		}, nil
	})
	h := s.NewTestHandler()

	req := &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      50,
		Method:  "tools/call",
		Params:  mustMarshal(map[string]any{"name": "multi_content", "arguments": map[string]any{"label": "test"}}),
	}
	resp := h.HandleRequest(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("tools/call error: %v", resp.Error.Message)
	}

	result := resp.Result.(map[string]any)
	if isErr, _ := result["isError"].(bool); isErr {
		t.Error("isError should be false")
	}

	data, _ := json.Marshal(result["content"])
	var content []map[string]any
	json.Unmarshal(data, &content)
	if len(content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(content))
	}
	if text, _ := content[0]["text"].(string); text != "block one: test" {
		t.Errorf("content[0] = %q, want %q", text, "block one: test")
	}
}

// TestCallTool_ResultReturn verifies that a handler returning Result is
// correctly marshalled.
func TestCallTool_ResultReturn(t *testing.T) {
	s := New("result-server", "1.0.0")
	s.Tool("result_tool", "Returns a Result directly", func(_ context.Context, in contentReturnInput) (Result, error) {
		return NewResult(TextContent("result: " + in.Label)), nil
	})
	h := s.NewTestHandler()

	req := &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      51,
		Method:  "tools/call",
		Params:  mustMarshal(map[string]any{"name": "result_tool", "arguments": map[string]any{"label": "hello"}}),
	}
	resp := h.HandleRequest(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("tools/call error: %v", resp.Error.Message)
	}

	result := resp.Result.(map[string]any)
	if isErr, _ := result["isError"].(bool); isErr {
		t.Error("isError should be false")
	}

	data, _ := json.Marshal(result["content"])
	var content []map[string]any
	json.Unmarshal(data, &content)
	if len(content) == 0 {
		t.Fatal("expected content block")
	}
	if text, _ := content[0]["text"].(string); text != "result: hello" {
		t.Errorf("text = %q, want %q", text, "result: hello")
	}
}

// ---- TestAddTools (ToolCollection) ------------------------------------------

// simpleCollection is a minimal ToolCollection for testing AddTools.
type simpleCollection struct {
	tools []ToolInfo
}

func (sc *simpleCollection) Tools() []ToolInfo { return sc.tools }

func TestAddTools_ToolCollection(t *testing.T) {
	s := New("addtools-server", "1.0.0")

	coll := &simpleCollection{
		tools: []ToolInfo{
			{
				Name:        "coll_echo",
				Description: "Echo from collection",
				Handler: func(_ context.Context, in echoInput) (string, error) {
					return "coll:" + in.Message, nil
				},
			},
		},
	}
	s.AddTools(coll)

	h := s.NewTestHandler()
	req := &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      60,
		Method:  "tools/call",
		Params:  mustMarshal(map[string]any{"name": "coll_echo", "arguments": map[string]any{"message": "world"}}),
	}
	resp := h.HandleRequest(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("tools/call error: %v", resp.Error.Message)
	}
	result := resp.Result.(map[string]any)
	if isErr, _ := result["isError"].(bool); isErr {
		t.Error("isError should be false")
	}
	data, _ := json.Marshal(result["content"])
	var content []map[string]any
	json.Unmarshal(data, &content)
	if text, _ := content[0]["text"].(string); text != "coll:world" {
		t.Errorf("text = %q, want %q", text, "coll:world")
	}
}

// ---- TestListTools method ----------------------------------------------------

func TestListToolsMethod(t *testing.T) {
	s := newTestServer()
	tools := s.ListTools()
	if len(tools) != 5 {
		t.Errorf("ListTools() returned %d tools, want 5", len(tools))
	}
	names := make(map[string]bool)
	for _, td := range tools {
		names[td.Name] = true
	}
	for _, want := range []string{"echo", "add", "greet", "defaults_demo", "fail_tool"} {
		if !names[want] {
			t.Errorf("tool %q missing from ListTools() result", want)
		}
	}
}

// ---- TestCallTool_NilArgs ---------------------------------------------------

// TestCallTool_NilArgs verifies that passing nil rawArgs (no arguments field)
// is treated the same as an empty args map.
func TestCallTool_NilArgs(t *testing.T) {
	h := newTestServer().NewTestHandler()

	// Construct a tools/call request that omits the "arguments" field entirely.
	req := &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      70,
		Method:  "tools/call",
		// Omit "arguments" — the params only contains "name".
		Params: mustMarshal(map[string]any{
			"name": "defaults_demo",
			// arguments deliberately absent
		}),
	}

	resp := h.HandleRequest(context.Background(), req)
	if resp.Error != nil {
		t.Fatalf("unexpected RPC error: %v", resp.Error.Message)
	}
	result := resp.Result.(map[string]any)
	// defaults_demo should succeed because it has defaults for all fields.
	if isErr, _ := result["isError"].(bool); isErr {
		t.Error("isError should be false when args are nil and defaults cover all fields")
	}
}

// ---- TestOnCall callback -----------------------------------------------------

func TestOnCall_Callback(t *testing.T) {
	var mu sync.Mutex
	var called bool
	var capturedTool string

	s := New("oncall-server", "1.0.0")
	s.Tool("ping", "Ping tool", func(_ context.Context, in echoInput) (string, error) {
		return "pong", nil
	})
	s.OnCall(func(tool string, args map[string]any, duration time.Duration, err error) {
		mu.Lock()
		called = true
		capturedTool = tool
		mu.Unlock()
	})

	h := s.NewTestHandler()
	req := &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      80,
		Method:  "tools/call",
		Params:  mustMarshal(map[string]any{"name": "ping", "arguments": map[string]any{"message": "test"}}),
	}
	h.HandleRequest(context.Background(), req)

	mu.Lock()
	defer mu.Unlock()
	if !called {
		t.Error("OnCall callback was not invoked")
	}
	if capturedTool != "ping" {
		t.Errorf("OnCall got tool = %q, want %q", capturedTool, "ping")
	}
}

// ---- TestCallTool_InvalidCallParams -----------------------------------------

func TestCallTool_InvalidCallParams(t *testing.T) {
	h := newTestServer().NewTestHandler()

	req := &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      90,
		Method:  "tools/call",
		Params:  json.RawMessage(`not-valid-json`),
	}
	resp := h.HandleRequest(context.Background(), req)
	if resp.Error == nil {
		t.Fatal("expected error for invalid tools/call params")
	}
	if resp.Error.Code != transport.CodeInvalidParams {
		t.Errorf("error code = %d, want CodeInvalidParams", resp.Error.Code)
	}
}
