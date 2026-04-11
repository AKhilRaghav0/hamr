package hamr

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/AKhilRaghav0/hamr/transport"
)

// mcpHandler implements transport.Handler and bridges the MCP protocol to the Server.
type mcpHandler struct {
	server *Server
}

// MCP protocol constants.
const (
	mcpProtocolVersion = "2024-11-05"

	methodInitialize        = "initialize"
	methodListTools         = "tools/list"
	methodCallTool          = "tools/call"
	methodListPrompts       = "prompts/list"
	methodGetPrompt         = "prompts/get"
	methodListResources     = "resources/list"
	methodReadResource      = "resources/read"
	methodNotificationsInit = "notifications/initialized"
)

// HandleRequest processes incoming JSON-RPC requests.
func (h *mcpHandler) HandleRequest(ctx context.Context, req *transport.JSONRPCRequest) *transport.JSONRPCResponse {
	switch req.Method {
	case methodInitialize:
		return h.handleInitialize(req)
	case methodListTools:
		return h.handleListTools(req)
	case methodCallTool:
		return h.handleCallTool(ctx, req)
	case methodListPrompts:
		return h.handleListPrompts(req)
	case methodGetPrompt:
		return h.handleGetPrompt(ctx, req)
	case methodListResources:
		return h.handleListResources(req)
	case methodReadResource:
		return h.handleReadResource(ctx, req)
	default:
		return &transport.JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &transport.RPCError{
				Code:    transport.CodeMethodNotFound,
				Message: fmt.Sprintf("method not found: %s", req.Method),
			},
		}
	}
}

// HandleNotification processes incoming notifications (no response needed).
func (h *mcpHandler) HandleNotification(ctx context.Context, notif *transport.JSONRPCNotification) {
	switch notif.Method {
	case methodNotificationsInit:
		h.server.logger.Info("client initialized")
	default:
		h.server.logger.Debug("unknown notification", "method", notif.Method)
	}
}

func (h *mcpHandler) handleInitialize(req *transport.JSONRPCRequest) *transport.JSONRPCResponse {
	h.server.mu.RLock()
	hasTools := len(h.server.tools) > 0 || len(h.server.toolGroups) > 0
	hasPrompts := len(h.server.prompts) > 0
	hasResources := len(h.server.resources) > 0
	h.server.mu.RUnlock()

	capabilities := map[string]any{}
	if hasTools {
		capabilities["tools"] = map[string]any{}
	}
	if hasPrompts {
		capabilities["prompts"] = map[string]any{}
	}
	if hasResources {
		capabilities["resources"] = map[string]any{}
	}

	serverInfo := map[string]any{
		"name":    h.server.name,
		"version": h.server.version,
	}
	if h.server.config.description != "" {
		serverInfo["description"] = h.server.config.description
	}

	return &transport.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]any{
			"protocolVersion": mcpProtocolVersion,
			"capabilities":    capabilities,
			"serverInfo":      serverInfo,
		},
	}
}

func (h *mcpHandler) handleListTools(req *transport.JSONRPCRequest) *transport.JSONRPCResponse {
	h.server.mu.RLock()
	defer h.server.mu.RUnlock()

	tools := make([]map[string]any, 0, len(h.server.tools)+len(h.server.toolGroups))
	for _, td := range h.server.tools {
		schema := td.Schema
		if h.server.config.minimalSchemas {
			schema = minifySchema(schema)
		}
		tools = append(tools, map[string]any{
			"name":        td.Name,
			"description": td.Description,
			"inputSchema": schema,
		})
	}

	// Add a selector entry for each group so the AI can expand it lazily.
	for _, g := range h.server.toolGroups {
		tools = append(tools, map[string]any{
			"name":        "group__" + g.name,
			"description": g.description + " (call this to see available operations)",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
		})
	}

	return &transport.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]any{
			"tools": tools,
		},
	}
}

// minifySchema returns a copy of schema with verbose annotation fields removed.
// Kept fields: type, properties (recursively minified), required, items.
// Stripped fields: description, default, enum, minimum, maximum, pattern, format.
func minifySchema(schema map[string]any) map[string]any {
	if schema == nil {
		return nil
	}
	out := make(map[string]any, len(schema))

	if v, ok := schema["type"]; ok {
		out["type"] = v
	}

	if props, ok := schema["properties"].(map[string]any); ok {
		minified := make(map[string]any, len(props))
		for k, v := range props {
			if propSchema, ok := v.(map[string]any); ok {
				minified[k] = minifySchema(propSchema)
			} else {
				minified[k] = v
			}
		}
		out["properties"] = minified
	}

	if v, ok := schema["required"]; ok {
		out["required"] = v
	}

	// Recursively minify array items.
	if items, ok := schema["items"].(map[string]any); ok {
		out["items"] = minifySchema(items)
	}

	return out
}

func (h *mcpHandler) handleCallTool(ctx context.Context, req *transport.JSONRPCRequest) *transport.JSONRPCResponse {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &transport.JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &transport.RPCError{
				Code:    transport.CodeInvalidParams,
				Message: "invalid params for tools/call",
			},
		}
	}

	// Handle group selector calls: return a text listing of the group's tools.
	if strings.HasPrefix(params.Name, "group__") {
		groupName := strings.TrimPrefix(params.Name, "group__")
		h.server.mu.RLock()
		g, found := h.server.toolGroups[groupName]
		h.server.mu.RUnlock()

		if !found {
			return &transport.JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": fmt.Sprintf("unknown group: %s", groupName)},
					},
					"isError": true,
				},
			}
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Group %q — %s\n\nAvailable tools:\n", g.name, g.description))
		for _, td := range g.tools {
			sb.WriteString(fmt.Sprintf("\n- %s: %s\n", td.Name, td.Description))
			// List required parameters with their types.
			if props, ok := td.Schema["properties"].(map[string]any); ok {
				required := map[string]bool{}
				if req, ok := td.Schema["required"].([]any); ok {
					for _, r := range req {
						if s, ok := r.(string); ok {
							required[s] = true
						}
					}
				}
				for pName, pVal := range props {
					pSchema, _ := pVal.(map[string]any)
					pType, _ := pSchema["type"].(string)
					req := required[pName]
					reqStr := "optional"
					if req {
						reqStr = "required"
					}
					sb.WriteString(fmt.Sprintf("    %s (%s, %s)\n", pName, pType, reqStr))
				}
			}
		}

		return &transport.JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": sb.String()},
				},
				"isError": false,
			},
		}
	}

	result, err := h.server.invokeTool(ctx, params.Name, params.Arguments)
	if err != nil {
		h.server.logger.Error("tool call failed", "tool", params.Name, "error", err)
	}

	// Build MCP content response
	content := make([]map[string]any, 0, len(result.Content))
	for _, c := range result.Content {
		entry := map[string]any{"type": c.Type}
		if c.Text != "" {
			entry["text"] = c.Text
		}
		if c.MimeType != "" {
			entry["mimeType"] = c.MimeType
		}
		if c.Data != "" {
			entry["data"] = c.Data
		}
		if c.URI != "" {
			entry["uri"] = c.URI
		}
		content = append(content, entry)
	}

	return &transport.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]any{
			"content": content,
			"isError": result.IsError,
		},
	}
}

func (h *mcpHandler) handleListPrompts(req *transport.JSONRPCRequest) *transport.JSONRPCResponse {
	h.server.mu.RLock()
	defer h.server.mu.RUnlock()

	prompts := make([]map[string]any, 0, len(h.server.prompts))
	for _, pd := range h.server.prompts {
		prompt := map[string]any{
			"name":        pd.Name,
			"description": pd.Description,
		}
		if len(pd.arguments) > 0 {
			prompt["arguments"] = pd.arguments
		}
		prompts = append(prompts, prompt)
	}

	return &transport.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]any{
			"prompts": prompts,
		},
	}
}

func (h *mcpHandler) handleGetPrompt(ctx context.Context, req *transport.JSONRPCRequest) *transport.JSONRPCResponse {
	var params struct {
		Name      string            `json:"name"`
		Arguments map[string]string `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &transport.JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &transport.RPCError{
				Code:    transport.CodeInvalidParams,
				Message: "invalid params for prompts/get",
			},
		}
	}

	h.server.mu.RLock()
	pd, ok := h.server.prompts[params.Name]
	h.server.mu.RUnlock()

	if !ok {
		return &transport.JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &transport.RPCError{
				Code:    transport.CodeInvalidParams,
				Message: fmt.Sprintf("unknown prompt: %s", params.Name),
			},
		}
	}

	// Call the prompt handler — it returns (string, error)
	results := pd.handlerVal.Call([]reflect.Value{
		reflect.ValueOf(ctx),
		reflect.ValueOf(params.Arguments),
	})

	if !results[1].IsNil() {
		err := results[1].Interface().(error)
		return &transport.JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &transport.RPCError{
				Code:    transport.CodeInternalError,
				Message: err.Error(),
			},
		}
	}

	text := results[0].Interface().(string)
	return &transport.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]any{
			"description": pd.Description,
			"messages": []map[string]any{
				{
					"role": "user",
					"content": map[string]any{
						"type": "text",
						"text": text,
					},
				},
			},
		},
	}
}

func (h *mcpHandler) handleListResources(req *transport.JSONRPCRequest) *transport.JSONRPCResponse {
	h.server.mu.RLock()
	defer h.server.mu.RUnlock()

	resources := make([]map[string]any, 0, len(h.server.resources))
	for _, rd := range h.server.resources {
		res := map[string]any{
			"uri":         rd.URI,
			"name":        rd.Name,
			"description": rd.Description,
		}
		if rd.MimeType != "" {
			res["mimeType"] = rd.MimeType
		}
		resources = append(resources, res)
	}

	return &transport.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]any{
			"resources": resources,
		},
	}
}

func (h *mcpHandler) handleReadResource(ctx context.Context, req *transport.JSONRPCRequest) *transport.JSONRPCResponse {
	var params struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &transport.JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &transport.RPCError{
				Code:    transport.CodeInvalidParams,
				Message: "invalid params for resources/read",
			},
		}
	}

	h.server.mu.RLock()
	rd, ok := h.server.resources[params.URI]
	h.server.mu.RUnlock()

	if !ok {
		return &transport.JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &transport.RPCError{
				Code:    transport.CodeInvalidParams,
				Message: fmt.Sprintf("unknown resource: %s", params.URI),
			},
		}
	}

	// Call resource handler — func(ctx) (string, error)
	results := rd.handlerVal.Call([]reflect.Value{
		reflect.ValueOf(ctx),
	})

	if !results[1].IsNil() {
		err := results[1].Interface().(error)
		return &transport.JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &transport.RPCError{
				Code:    transport.CodeInternalError,
				Message: err.Error(),
			},
		}
	}

	text := results[0].Interface().(string)
	return &transport.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]any{
			"contents": []map[string]any{
				{
					"uri":  rd.URI,
					"text": text,
				},
			},
		},
	}
}
