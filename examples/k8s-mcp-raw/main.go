//go:build ignore

// k8s-mcp-raw is the SAME k8s MCP server, written WITHOUT mcpx.
// Using the official github.com/modelcontextprotocol/go-sdk directly.
//
// Compare this with ../k8s-mcp/main.go to see the difference.
//
// This file is ~480 lines. The mcpx version is ~157 lines.
// And this only implements 3 of the 6 tools to keep it "short".
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ============================================================
// JSON-RPC types — you have to define all of these yourself
// ============================================================

type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *RPCError `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ============================================================
// MCP protocol types — all manual
// ============================================================

type InitializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ServerInfo      ServerInfo     `json:"serverInfo"`
}

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type ToolsListResult struct {
	Tools []ToolDefinition `json:"tools"`
}

type ToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type ToolCallResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ============================================================
// JSON Schema definitions — ALL written by hand, per tool
// ============================================================

var getPodsSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"namespace": map[string]any{
			"type":        "string",
			"description": "kubernetes namespace",
			"default":     "default",
		},
		"selector": map[string]any{
			"type":        "string",
			"description": "label selector like app=nginx",
		},
	},
	"required": []string{"namespace"},
}

var getLogsSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"pod": map[string]any{
			"type":        "string",
			"description": "pod name",
		},
		"namespace": map[string]any{
			"type":        "string",
			"description": "kubernetes namespace",
			"default":     "default",
		},
		"lines": map[string]any{
			"type":        "integer",
			"description": "number of log lines",
			"default":     50,
		},
		"container": map[string]any{
			"type":        "string",
			"description": "container name (for multi-container pods)",
		},
	},
	"required": []string{"pod", "namespace", "lines"},
}

var getResourcesSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"resource": map[string]any{
			"type":        "string",
			"description": "resource type: pods, deployments, services, ingresses, configmaps, etc.",
		},
		"namespace": map[string]any{
			"type":        "string",
			"description": "namespace, or 'all' for all namespaces",
			"default":     "default",
		},
	},
	"required": []string{"resource", "namespace"},
}

// For 6 tools you'd need 6 of these schema blocks.
// Each one is ~20 lines. That's ~120 lines JUST for schemas.

// ============================================================
// Tool registry — manual dispatch table
// ============================================================

var tools = []ToolDefinition{
	{
		Name:        "get_pods",
		Description: "List pods in a namespace (with optional label selector)",
		InputSchema: getPodsSchema,
	},
	{
		Name:        "get_logs",
		Description: "Get logs from a pod",
		InputSchema: getLogsSchema,
	},
	{
		Name:        "get_resources",
		Description: "List any resource type (deployments, services, etc.)",
		InputSchema: getResourcesSchema,
	},
	// ... you'd add describe, get_events, top here too
	// each needs its own schema block above
}

// ============================================================
// Request handler — giant switch statement, no middleware
// ============================================================

func handleRequest(req *JSONRPCRequest) *JSONRPCResponse {
	switch req.Method {
	case "initialize":
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: InitializeResult{
				ProtocolVersion: "2024-11-05",
				Capabilities:    map[string]any{"tools": map[string]any{}},
				ServerInfo: ServerInfo{
					Name:    "k8s-mcp",
					Version: "1.0.0",
				},
			},
		}

	case "tools/list":
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  ToolsListResult{Tools: tools},
		}

	case "tools/call":
		return handleToolCall(req)

	case "notifications/initialized":
		return nil // notifications get no response

	default:
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &RPCError{
				Code:    -32601,
				Message: fmt.Sprintf("method not found: %s", req.Method),
			},
		}
	}
}

func handleToolCall(req *JSONRPCRequest) *JSONRPCResponse {
	var params ToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &RPCError{Code: -32602, Message: "invalid params"},
		}
	}

	var result string
	var callErr error

	// Manual dispatch — no type safety, no validation, no middleware
	switch params.Name {
	case "get_pods":
		// Manually parse arguments — no auto-deserialization
		var args struct {
			Namespace string `json:"namespace"`
			Selector  string `json:"selector"`
		}
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			return errorResponse(req.ID, "invalid arguments: "+err.Error())
		}
		// Manually apply defaults — no auto-defaults
		if args.Namespace == "" {
			args.Namespace = "default"
		}
		// Manually validate — no auto-validation
		// (skipped here for brevity, but in production you'd check types, required fields, etc.)

		cmdArgs := []string{"get", "pods", "-n", args.Namespace, "-o", "wide"}
		if args.Selector != "" {
			cmdArgs = append(cmdArgs, "-l", args.Selector)
		}
		result, callErr = runKubectl(cmdArgs...)

	case "get_logs":
		var args struct {
			Pod       string `json:"pod"`
			Namespace string `json:"namespace"`
			Lines     int    `json:"lines"`
			Container string `json:"container"`
		}
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			return errorResponse(req.ID, "invalid arguments: "+err.Error())
		}
		if args.Namespace == "" {
			args.Namespace = "default"
		}
		if args.Lines == 0 {
			args.Lines = 50
		}
		if args.Pod == "" {
			return errorResponse(req.ID, "pod name is required")
		}

		cmdArgs := []string{"logs", args.Pod, "-n", args.Namespace, "--tail", fmt.Sprintf("%d", args.Lines)}
		if args.Container != "" {
			cmdArgs = append(cmdArgs, "-c", args.Container)
		}
		result, callErr = runKubectl(cmdArgs...)

	case "get_resources":
		var args struct {
			Resource  string `json:"resource"`
			Namespace string `json:"namespace"`
		}
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			return errorResponse(req.ID, "invalid arguments: "+err.Error())
		}
		if args.Namespace == "" {
			args.Namespace = "default"
		}
		if args.Resource == "" {
			return errorResponse(req.ID, "resource type is required")
		}

		cmdArgs := []string{"get", args.Resource}
		if args.Namespace == "all" {
			cmdArgs = append(cmdArgs, "--all-namespaces")
		} else {
			cmdArgs = append(cmdArgs, "-n", args.Namespace)
		}
		cmdArgs = append(cmdArgs, "-o", "wide")
		result, callErr = runKubectl(cmdArgs...)

	default:
		return errorResponse(req.ID, fmt.Sprintf("unknown tool: %s", params.Name))
	}

	if callErr != nil {
		return &JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: ToolCallResult{
				Content: []ContentBlock{{Type: "text", Text: callErr.Error()}},
				IsError: true,
			},
		}
	}

	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: ToolCallResult{
			Content: []ContentBlock{{Type: "text", Text: result}},
			IsError: false,
		},
	}
}

func errorResponse(id any, msg string) *JSONRPCResponse {
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: ToolCallResult{
			Content: []ContentBlock{{Type: "text", Text: msg}},
			IsError: true,
		},
	}
}

// ============================================================
// kubectl helper — same as mcpx version
// ============================================================

func runKubectl(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/usr/local/bin/kubectl", args...)
	if kc := os.Getenv("KUBECONFIG"); kc != "" {
		cmd.Env = append(os.Environ(), "KUBECONFIG="+kc)
	}
	out, err := cmd.CombinedOutput()
	result := strings.TrimSpace(string(out))
	if err != nil {
		if result != "" {
			return "", fmt.Errorf("kubectl %s: %s", strings.Join(args, " "), result)
		}
		return "", fmt.Errorf("kubectl %s: %w", strings.Join(args, " "), err)
	}
	if result == "" {
		return "no output", nil
	}
	return result, nil
}

// ============================================================
// stdio transport — you write this yourself too
// ============================================================

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			resp := JSONRPCResponse{
				JSONRPC: "2.0",
				Error:   &RPCError{Code: -32700, Message: "parse error"},
			}
			data, _ := json.Marshal(resp)
			fmt.Fprintln(os.Stdout, string(data))
			continue
		}

		resp := handleRequest(&req)
		if resp == nil {
			continue // notification
		}

		data, err := json.Marshal(resp)
		if err != nil {
			log.Printf("marshal error: %v", err)
			continue
		}
		fmt.Fprintln(os.Stdout, string(data))
	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
}

// Total: ~480 lines for 3 tools (not even all 6!)
//
// What's MISSING vs the mcpx version:
// - No input validation (types, required, enum, min/max)
// - No default value injection
// - No middleware (logging, recovery, rate limiting, timeout)
// - No SSE transport
// - No TUI dashboard
// - No panic recovery (one bad tool = server crash)
// - No test utilities
// - Schemas are hand-written and can drift from actual code
// - Adding a new tool = ~80 lines (schema + struct + case + validation)
//
// With mcpx, adding a new tool = ~15 lines (struct + handler + one s.Tool() call)
