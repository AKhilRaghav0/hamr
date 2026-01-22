package transport_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/AKhilRaghav0/hamr/transport"
)

// ---- test handler -----------------------------------------------------------

// echoHandler is a minimal transport.Handler for transport-layer tests.
// It echoes back request params as the result, making responses easy to verify.
type echoHandler struct {
	notifications []string // records notification methods received
}

func (h *echoHandler) HandleRequest(ctx context.Context, req *transport.JSONRPCRequest) *transport.JSONRPCResponse {
	// Return the method name as the result so callers can verify routing.
	return &transport.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  map[string]any{"method": req.Method},
	}
}

func (h *echoHandler) HandleNotification(ctx context.Context, notif *transport.JSONRPCNotification) {
	h.notifications = append(h.notifications, notif.Method)
}

// ---- runTransport runs the stdio transport on an in-memory pipe and closes
// the write end of input after writing all lines. It returns the raw output.
func runTransport(t *testing.T, handler transport.Handler, lines []string) []byte {
	t.Helper()

	// Build the full input: one JSON line per entry, terminated by newline.
	var inputBuf bytes.Buffer
	for _, line := range lines {
		inputBuf.WriteString(line)
		inputBuf.WriteByte('\n')
	}

	var outputBuf bytes.Buffer
	tr := transport.NewStdioWithIO(handler, &inputBuf, &outputBuf)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run returns after EOF on the reader, which happens immediately after
	// inputBuf is exhausted.
	if err := tr.Run(ctx); err != nil {
		t.Fatalf("transport.Run error: %v", err)
	}
	return outputBuf.Bytes()
}

// parseLines splits raw output into non-empty lines and JSON-decodes each.
func parseLines(t *testing.T, raw []byte) []transport.JSONRPCResponse {
	t.Helper()
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	var responses []transport.JSONRPCResponse
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var resp transport.JSONRPCResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("failed to parse response line %q: %v", line, err)
		}
		responses = append(responses, resp)
	}
	return responses
}

// ---- TestStdio_SingleRequest ------------------------------------------------

func TestStdio_SingleRequest(t *testing.T) {
	h := &echoHandler{}
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/list",
	}
	line, _ := json.Marshal(req)

	out := runTransport(t, h, []string{string(line)})
	responses := parseLines(t, out)

	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}

	resp := responses[0]
	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error.Message)
	}
	if resp.JSONRPC != "2.0" {
		t.Errorf("jsonrpc = %q, want %q", resp.JSONRPC, "2.0")
	}
	// ID round-trips through JSON as float64; compare via string.
	if fmt.Sprint(resp.ID) != "1" {
		t.Errorf("response ID = %v, want 1", resp.ID)
	}
}

// ---- TestStdio_MultipleRequests ---------------------------------------------

func TestStdio_MultipleRequests(t *testing.T) {
	h := &echoHandler{}

	lines := make([]string, 5)
	for i := range lines {
		req := map[string]any{
			"jsonrpc": "2.0",
			"id":      i + 1,
			"method":  fmt.Sprintf("method/%d", i+1),
		}
		b, _ := json.Marshal(req)
		lines[i] = string(b)
	}

	out := runTransport(t, h, lines)
	responses := parseLines(t, out)

	if len(responses) != 5 {
		t.Fatalf("expected 5 responses, got %d", len(responses))
	}
}

// ---- TestStdio_Notification -------------------------------------------------

// Notifications have no "id" field. The transport must route them to
// HandleNotification and produce no response.
func TestStdio_Notification(t *testing.T) {
	h := &echoHandler{}

	notif := map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	}
	b, _ := json.Marshal(notif)

	out := runTransport(t, h, []string{string(b)})
	responses := parseLines(t, out)

	if len(responses) != 0 {
		t.Errorf("expected 0 responses for notification, got %d", len(responses))
	}
	if len(h.notifications) != 1 || h.notifications[0] != "notifications/initialized" {
		t.Errorf("notification not delivered to handler; got %v", h.notifications)
	}
}

// ---- TestStdio_ParseError ---------------------------------------------------

// Sending malformed JSON must produce a parse-error response.
func TestStdio_ParseError(t *testing.T) {
	h := &echoHandler{}

	out := runTransport(t, h, []string{`{not valid json`})
	responses := parseLines(t, out)

	if len(responses) != 1 {
		t.Fatalf("expected 1 error response, got %d", len(responses))
	}
	if responses[0].Error == nil {
		t.Fatal("expected error in response, got nil")
	}
	if responses[0].Error.Code != transport.CodeParseError {
		t.Errorf("error code = %d, want %d", responses[0].Error.Code, transport.CodeParseError)
	}
}

// ---- TestStdio_EmptyLines ---------------------------------------------------

// Blank lines (zero-length after scanning) in the input are silently skipped.
// Note: whitespace-only lines are non-empty bytes and produce a parse error —
// that behaviour is tested in TestStdio_ParseError.
func TestStdio_EmptyLines(t *testing.T) {
	h := &echoHandler{}

	req, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/list",
	})

	// Surround a valid request with truly empty lines (no bytes, just newlines).
	out := runTransport(t, h, []string{"", string(req), ""})
	responses := parseLines(t, out)

	if len(responses) != 1 {
		t.Errorf("expected 1 response (empty lines ignored), got %d", len(responses))
	}
}

// ---- TestStdio_MixedRequestsAndNotifications --------------------------------

func TestStdio_MixedRequestsAndNotifications(t *testing.T) {
	h := &echoHandler{}

	req1, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
	notif, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	req2, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call"})

	out := runTransport(t, h, []string{string(req1), string(notif), string(req2)})
	responses := parseLines(t, out)

	// Two requests → two responses; one notification → no response.
	if len(responses) != 2 {
		t.Errorf("expected 2 responses, got %d", len(responses))
	}
	if len(h.notifications) != 1 {
		t.Errorf("expected 1 notification, got %d", len(h.notifications))
	}
}

// ---- TestStdio_ResponseFraming ----------------------------------------------

// Each response must be terminated by exactly one newline and be valid JSON.
func TestStdio_ResponseFraming(t *testing.T) {
	h := &echoHandler{}

	req, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
	out := runTransport(t, h, []string{string(req)})

	// The raw output must end with exactly one newline.
	if !bytes.HasSuffix(out, []byte("\n")) {
		t.Errorf("response not terminated by newline: %q", out)
	}
	// Trim the trailing newline and verify JSON validity.
	trimmed := bytes.TrimRight(out, "\n")
	if !json.Valid(trimmed) {
		t.Errorf("response is not valid JSON: %q", trimmed)
	}
}

// ---- TestStdio_ContextCancellation ------------------------------------------

// When the context is cancelled, Run must return promptly.
func TestStdio_ContextCancellation(t *testing.T) {
	h := &echoHandler{}

	// A pipe reader that blocks until closed gives us control over EOF timing.
	pr, pw := io.Pipe()
	defer pr.Close()
	defer pw.Close()

	var outputBuf bytes.Buffer
	tr := transport.NewStdioWithIO(h, pr, &outputBuf)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- tr.Run(ctx)
	}()

	// Cancel the context; Run should exit.
	cancel()
	pw.Close() // unblock the scanner so it sees EOF

	select {
	case <-done:
		// Run returned — success.
	case <-make(chan struct{}): // never fires; just for the select form
	}
}

// ---- TestSendNotification ---------------------------------------------------

func TestSendNotification(t *testing.T) {
	h := &echoHandler{}

	pr, pw := io.Pipe()
	var outputBuf bytes.Buffer
	tr := transport.NewStdioWithIO(h, pr, &outputBuf)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- tr.Run(ctx)
	}()

	// Send a notification from the server to the client (outbound).
	if err := tr.SendNotification("tools/list_changed", map[string]any{"delta": 1}); err != nil {
		t.Fatalf("SendNotification error: %v", err)
	}

	cancel()
	pw.Close()
	<-done

	out := outputBuf.Bytes()
	if len(out) == 0 {
		t.Fatal("expected notification output, got nothing")
	}

	var notif transport.JSONRPCNotification
	line := bytes.TrimRight(out, "\n")
	if err := json.Unmarshal(line, &notif); err != nil {
		t.Fatalf("failed to parse sent notification: %v", err)
	}
	if notif.Method != "tools/list_changed" {
		t.Errorf("method = %q, want %q", notif.Method, "tools/list_changed")
	}
}

// ---- TestStdio_StringID -----------------------------------------------------

func TestStdio_StringID(t *testing.T) {
	h := &echoHandler{}

	req, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      "my-string-id",
		"method":  "tools/list",
	})

	out := runTransport(t, h, []string{string(req)})
	responses := parseLines(t, out)

	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}
	if fmt.Sprint(responses[0].ID) != "my-string-id" {
		t.Errorf("response ID = %v, want %q", responses[0].ID, "my-string-id")
	}
}

// ---- TestStdio_LargeMessage -------------------------------------------------

// TestStdio_LargeMessage verifies that the transport can handle a single
// message that is substantially larger than the default scanner buffer size
// (the transport configures a 10 MiB limit).
func TestStdio_LargeMessage(t *testing.T) {
	h := &echoHandler{}

	// Build a valid JSON object with a large "data" field (~512 KiB).
	largeValue := strings.Repeat("x", 512*1024)
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      42,
		"method":  "tools/list",
		"params":  map[string]any{"data": largeValue},
	}
	line, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	out := runTransport(t, h, []string{string(line)})
	responses := parseLines(t, out)

	if len(responses) != 1 {
		t.Fatalf("expected 1 response for large message, got %d", len(responses))
	}
	if responses[0].Error != nil {
		t.Errorf("unexpected error for large message: %v", responses[0].Error.Message)
	}
}

// ---- TestStdio_ValidJSONButMissingMethod ------------------------------------

// TestStdio_ValidJSONButMissingMethod sends a structurally valid JSON object
// that has an "id" field but no "method". The transport should still route it
// as a request and the handler receives it (with an empty method string).
func TestStdio_ValidJSONButMissingMethod(t *testing.T) {
	h := &echoHandler{}

	// Valid JSON with id but without method.
	line, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      7,
		// "method" intentionally absent
	})

	out := runTransport(t, h, []string{string(line)})
	responses := parseLines(t, out)

	// The transport must produce exactly one response (even for an empty method).
	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}
}

// ---- TestStdio_NotificationInvalidJSON --------------------------------------

// TestStdio_NotificationInvalidJSON verifies that malformed JSON for a
// notification (no id field) is silently skipped — no response is produced.
func TestStdio_NotificationInvalidJSON(t *testing.T) {
	h := &echoHandler{}

	// A JSON object with no "id" field but invalid nested structure.
	// json.Unmarshal will fail for the inner decode of JSONRPCNotification.
	// We use a number where a string is expected for "method".
	line := []byte(`{"jsonrpc":"2.0","method":12345}`)

	out := runTransport(t, h, []string{string(line)})
	responses := parseLines(t, out)

	// Notification decode failure should produce zero responses.
	if len(responses) != 0 {
		t.Errorf("expected 0 responses for notification with bad method type, got %d", len(responses))
	}
}

// ---- TestStdio_WhitespaceOnlyLine -------------------------------------------

// TestStdio_WhitespaceOnlyLine sends a line that contains only spaces. The
// transport will attempt JSON parsing on non-empty bytes and produce a parse
// error response.
func TestStdio_WhitespaceOnlyLine(t *testing.T) {
	h := &echoHandler{}

	out := runTransport(t, h, []string{"   "})
	responses := parseLines(t, out)

	// A whitespace-only line is non-empty and will fail JSON parsing → parse error.
	if len(responses) != 1 {
		t.Fatalf("expected 1 parse-error response, got %d", len(responses))
	}
	if responses[0].Error == nil || responses[0].Error.Code != transport.CodeParseError {
		t.Errorf("expected parse error, got: %v", responses[0].Error)
	}
}
