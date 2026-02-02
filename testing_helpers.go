package mcpx

import "github.com/AKhilRaghav0/hamr/transport"

// NewTestHandler returns a transport.Handler backed by this Server. The
// handler processes JSON-RPC requests in-process without starting any
// network or stdio transport, making it suitable for use with mcpxtest.Client
// in unit and integration tests.
//
// Example:
//
//	s := mcpx.New("test-server", "1.0.0")
//	s.Tool("echo", "Echo text", myHandler)
//	client := mcpxtest.NewClient(t, s.NewTestHandler())
//	result, err := client.CallTool("echo", map[string]any{"text": "hello"})
func (s *Server) NewTestHandler() transport.Handler {
	return &mcpHandler{server: s}
}
