// Package hamrtest provides testing utilities for mcpx servers.
//
// The central type is Client, an in-process MCP client that sends JSON-RPC
// requests directly to a transport.Handler (obtained from Server.NewTestHandler)
// without starting any real network or stdio transport. This keeps tests fast,
// deterministic, and free of port/process management.
//
// Typical usage:
//
//	func TestMyTool(t *testing.T) {
//	    s := hamr.New("srv", "1.0")
//	    s.Tool("greet", "Greet a user", myGreetHandler)
//	    client := hamrtest.NewClient(t, s.NewTestHandler())
//	    result, err := client.CallTool("greet", map[string]any{"name": "Alice"})
//	    // assert on result and err ...
//	}
package hamrtest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/AKhilRaghav0/hamr/transport"
)

// Client is an in-process MCP test client. It dispatches JSON-RPC requests
// directly to a transport.Handler, bypassing all network and stdio machinery.
// Methods call t.Fatal on unexpected errors, keeping test code concise.
type Client struct {
	t       *testing.T
	handler transport.Handler
}

// NewClient creates a Client that sends requests to handler. Obtain a handler
// from a configured Server using Server.NewTestHandler.
func NewClient(t *testing.T, handler transport.Handler) *Client {
	t.Helper()
	return &Client{t: t, handler: handler}
}

// ToolResult holds the decoded result of a CallTool invocation. IsError is
// true when the server signalled a tool-level error rather than a successful
// response.
type ToolResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError"`
}

// ContentBlock represents a single content item within a ToolResult. The
// Type field determines which other fields are populated: "text" uses Text,
// "image" uses MimeType and Data (base64), "resource" uses URI, MimeType,
// and Text.
type ContentBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	Data     string `json:"data,omitempty"`
	URI      string `json:"uri,omitempty"`
}

// Text returns the concatenated text of all "text" content blocks in the
// result, joined with newlines when there are multiple blocks.
func (r *ToolResult) Text() string {
	var buf bytes.Buffer
	for _, c := range r.Content {
		if c.Type == "text" && c.Text != "" {
			if buf.Len() > 0 {
				buf.WriteString("\n")
			}
			buf.WriteString(c.Text)
		}
	}
	return buf.String()
}

// ToolDef describes a registered tool as returned by ListTools. InputSchema
// is the JSON Schema map generated from the tool's input struct.
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// Initialize sends the MCP initialize handshake and returns the server's info
// map (containing serverInfo, capabilities, etc.). It must be called before
// any tool calls when testing protocol correctness, but CallTool and
// ListTools work without it for convenience.
func (c *Client) Initialize() map[string]any {
	c.t.Helper()
	resp := c.request("initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "hamrtest",
			"version": "1.0.0",
		},
	})
	result, ok := resp.Result.(map[string]any)
	if !ok {
		c.t.Fatalf("initialize: unexpected result type: %T", resp.Result)
	}
	return result
}

// ListTools sends a tools/list request and returns all registered tool
// definitions. The method calls t.Fatal if the response cannot be parsed.
func (c *Client) ListTools() []ToolDef {
	c.t.Helper()
	resp := c.request("tools/list", nil)
	result, ok := resp.Result.(map[string]any)
	if !ok {
		c.t.Fatalf("list_tools: unexpected result type: %T", resp.Result)
	}

	toolsRaw, ok := result["tools"]
	if !ok {
		return nil
	}

	// Re-marshal and unmarshal to get proper types
	data, _ := json.Marshal(toolsRaw)
	var tools []ToolDef
	if err := json.Unmarshal(data, &tools); err != nil {
		c.t.Fatalf("list_tools: failed to parse tools: %v", err)
	}
	return tools
}

// CallTool invokes a registered tool by name with the provided argument map.
// It returns the decoded ToolResult on success, or a non-nil error when the
// server returns a JSON-RPC error response. A tool-level error (IsError == true
// in the result) is returned alongside a nil error from CallTool.
func (c *Client) CallTool(name string, args map[string]any) (*ToolResult, error) {
	c.t.Helper()

	argsJSON, _ := json.Marshal(args)
	resp := c.request("tools/call", map[string]any{
		"name":      name,
		"arguments": json.RawMessage(argsJSON),
	})

	if resp.Error != nil {
		return nil, fmt.Errorf("tool call error [%d]: %s", resp.Error.Code, resp.Error.Message)
	}

	data, _ := json.Marshal(resp.Result)
	var result ToolResult
	if err := json.Unmarshal(data, &result); err != nil {
		c.t.Fatalf("CallTool: failed to parse result: %v", err)
	}
	return &result, nil
}

// ListResources sends a resources/list request and returns the raw resource
// descriptors. Each map contains at least "uri", "name", and "description"
// keys as defined by the MCP specification.
func (c *Client) ListResources() []map[string]any {
	c.t.Helper()
	resp := c.request("resources/list", nil)
	result, ok := resp.Result.(map[string]any)
	if !ok {
		return nil
	}
	data, _ := json.Marshal(result["resources"])
	var resources []map[string]any
	json.Unmarshal(data, &resources)
	return resources
}

// ListPrompts sends a prompts/list request and returns the raw prompt
// descriptors. Each map contains at least "name" and "description" keys.
func (c *Client) ListPrompts() []map[string]any {
	c.t.Helper()
	resp := c.request("prompts/list", nil)
	result, ok := resp.Result.(map[string]any)
	if !ok {
		return nil
	}
	data, _ := json.Marshal(result["prompts"])
	var prompts []map[string]any
	json.Unmarshal(data, &prompts)
	return prompts
}

func (c *Client) request(method string, params any) *transport.JSONRPCResponse {
	c.t.Helper()

	var paramsRaw json.RawMessage
	if params != nil {
		var err error
		paramsRaw, err = json.Marshal(params)
		if err != nil {
			c.t.Fatalf("failed to marshal params: %v", err)
		}
	}

	req := &transport.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  method,
		Params:  paramsRaw,
	}

	return c.handler.HandleRequest(context.Background(), req)
}

// AssertToolExists is a test helper that calls t.Errorf if no tool with the
// given name is found in the server's tool list.
func AssertToolExists(t *testing.T, client *Client, name string) {
	t.Helper()
	tools := client.ListTools()
	for _, tool := range tools {
		if tool.Name == name {
			return
		}
	}
	t.Errorf("tool %q not found in registered tools", name)
}

// AssertToolCount is a test helper that calls t.Errorf if the number of
// registered tools does not equal n.
func AssertToolCount(t *testing.T, client *Client, n int) {
	t.Helper()
	tools := client.ListTools()
	if len(tools) != n {
		t.Errorf("expected %d tools, got %d", n, len(tools))
	}
}
