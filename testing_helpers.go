package hamr

import "github.com/AKhilRaghav0/hamr/transport"

// NewTestHandler returns a transport.Handler backed by this Server. The
// handler processes JSON-RPC requests in-process without starting any
// network or stdio transport, making it suitable for use with hamrtest.Client
// in unit and integration tests.
//
// Example:
//
//	s := hamr.New("test-server", "1.0.0")
//	s.Tool("echo", "Echo text", myHandler)
//	client := hamrtest.NewClient(t, s.NewTestHandler())
//	result, err := client.CallTool("echo", map[string]any{"text": "hello"})
func (s *Server) NewTestHandler() transport.Handler {
	return &mcpHandler{server: s}
}
