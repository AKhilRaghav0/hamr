// Package transport provides MCP transport implementations for mcpx servers.
//
// The package defines the Handler interface that all transports rely on, along
// with the JSON-RPC 2.0 wire types (JSONRPCRequest, JSONRPCResponse,
// JSONRPCNotification, RPCError) that are shared across transport layers.
//
// StdioTransport is the standard implementation: it reads newline-delimited
// JSON-RPC messages from an io.Reader (default: os.Stdin) and writes responses
// to an io.Writer (default: os.Stdout). This matches the MCP stdio transport
// specification used by Claude Desktop and most MCP clients.
package transport

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
)

// JSONRPCRequest represents an incoming JSON-RPC 2.0 request. Requests carry
// an ID so that the client can match the response. The ID may be a string or
// an integer per the JSON-RPC specification.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"` // can be string or int
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse represents an outgoing JSON-RPC 2.0 response. Exactly one
// of Result or Error will be non-nil in a well-formed response.
type JSONRPCResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *RPCError `json:"error,omitempty"`
}

// JSONRPCNotification is a one-way JSON-RPC 2.0 message that carries no ID
// and expects no response. MCP uses notifications for lifecycle events such as
// "notifications/initialized".
type JSONRPCNotification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// RPCError is the JSON-RPC 2.0 error object embedded in a JSONRPCResponse
// when a request cannot be fulfilled. Code should be one of the standard
// JSON-RPC error codes defined in this package.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Standard JSON-RPC 2.0 error codes as defined by the specification.
const (
	CodeParseError     = -32700 // Invalid JSON was received.
	CodeInvalidRequest = -32600 // The request object is not valid JSON-RPC.
	CodeMethodNotFound = -32601 // The method does not exist or is not available.
	CodeInvalidParams  = -32602 // Invalid method parameters.
	CodeInternalError  = -32603 // Internal JSON-RPC error.
)

// Handler is the interface implemented by the mcpx server core. Transport
// implementations call HandleRequest for each incoming JSON-RPC request and
// HandleNotification for incoming notifications (no response expected).
type Handler interface {
	HandleRequest(ctx context.Context, req *JSONRPCRequest) *JSONRPCResponse
	HandleNotification(ctx context.Context, notif *JSONRPCNotification)
}

// StdioTransport reads JSON-RPC messages from stdin and writes responses to stdout.
type StdioTransport struct {
	handler Handler
	reader  io.Reader
	writer  io.Writer
	mu      sync.Mutex // protects writer
}

// NewStdio creates a stdio transport using os.Stdin and os.Stdout.
func NewStdio(handler Handler) *StdioTransport {
	return &StdioTransport{
		handler: handler,
		reader:  os.Stdin,
		writer:  os.Stdout,
	}
}

// NewStdioWithIO creates a stdio transport with custom reader/writer (useful for testing).
func NewStdioWithIO(handler Handler, r io.Reader, w io.Writer) *StdioTransport {
	return &StdioTransport{
		handler: handler,
		reader:  r,
		writer:  w,
	}
}

// Run starts the stdio transport. Blocks until EOF or context cancellation.
func (t *StdioTransport) Run(ctx context.Context) error {
	scanner := bufio.NewScanner(t.reader)
	// MCP messages can be large
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		// Try to parse as request (has id) or notification (no id)
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(line, &raw); err != nil {
			t.sendError(nil, CodeParseError, "Parse error")
			continue
		}

		// Check if it has an "id" field — if so, it's a request
		if _, hasID := raw["id"]; hasID {
			var req JSONRPCRequest
			if err := json.Unmarshal(line, &req); err != nil {
				t.sendError(nil, CodeParseError, "Parse error")
				continue
			}
			resp := t.handler.HandleRequest(ctx, &req)
			if resp != nil {
				t.sendResponse(resp)
			}
		} else {
			var notif JSONRPCNotification
			if err := json.Unmarshal(line, &notif); err != nil {
				continue // notifications don't get error responses
			}
			t.handler.HandleNotification(ctx, &notif)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("stdin read error: %w", err)
	}
	return nil
}

func (t *StdioTransport) sendResponse(resp *JSONRPCResponse) {
	t.mu.Lock()
	defer t.mu.Unlock()

	data, err := json.Marshal(resp)
	if err != nil {
		return
	}
	data = append(data, '\n')
	t.writer.Write(data)
}

func (t *StdioTransport) sendError(id any, code int, message string) {
	t.sendResponse(&JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &RPCError{
			Code:    code,
			Message: message,
		},
	})
}

// SendNotification sends a JSON-RPC notification to the client.
func (t *StdioTransport) SendNotification(method string, params any) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	var paramsRaw json.RawMessage
	if params != nil {
		var err error
		paramsRaw, err = json.Marshal(params)
		if err != nil {
			return err
		}
	}

	notif := JSONRPCNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  paramsRaw,
	}
	data, err := json.Marshal(notif)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = t.writer.Write(data)
	return err
}
