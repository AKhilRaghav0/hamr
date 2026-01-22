package transport_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/AKhilRaghav0/hamr/transport"
)

// ---- helpers ----------------------------------------------------------------

// freePort returns a TCP port that is free on localhost at the time of the call.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// startSSE creates an SSETransport, starts it in the background, and returns
// its base URL. The transport is shut down when the test context is cancelled.
func startSSE(t *testing.T) (baseURL string, cancel context.CancelFunc) {
	t.Helper()
	port := freePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	baseURL = "http://" + addr

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tr := transport.NewSSE(&echoHandler{}, addr, logger)

	ctx, cancel := context.WithCancel(context.Background())

	ready := make(chan struct{})
	go func() {
		// Poll until the server is accepting connections, then signal ready.
		for i := 0; i < 50; i++ {
			conn, err := net.DialTimeout("tcp", addr, 20*time.Millisecond)
			if err == nil {
				conn.Close()
				close(ready)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		close(ready) // give up polling; tests will fail naturally
	}()

	go func() { tr.Run(ctx) }() //nolint:errcheck

	<-ready
	return baseURL, cancel
}

// readSSELines reads SSE lines from the response body until either the context
// is done or n event lines (lines starting with "data:" or "event:") have been
// collected.
func readSSELines(ctx context.Context, body io.Reader, n int) []string {
	var lines []string
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return lines
		default:
		}
		line := scanner.Text()
		if strings.HasPrefix(line, "event:") || strings.HasPrefix(line, "data:") {
			lines = append(lines, line)
			if len(lines) >= n {
				return lines
			}
		}
	}
	return lines
}

// ---- TestSSE_EndpointEvent --------------------------------------------------

// TestSSE_EndpointEvent connects to /sse and verifies that the first event is
// an "endpoint" event containing a sessionId query parameter.
func TestSSE_EndpointEvent(t *testing.T) {
	baseURL, cancel := startSSE(t)
	defer cancel()

	ctx, clientCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer clientCancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/sse", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /sse: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /sse status = %d, want 200", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}

	// Read first two lines: "event: endpoint" and "data: /message?sessionId=…"
	lines := readSSELines(ctx, resp.Body, 2)
	if len(lines) < 2 {
		t.Fatalf("expected 2 SSE lines, got %d: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "endpoint") {
		t.Errorf("first line should be endpoint event, got %q", lines[0])
	}
	if !strings.Contains(lines[1], "sessionId") {
		t.Errorf("data line should contain sessionId, got %q", lines[1])
	}
}

// ---- TestSSE_PostMessage ----------------------------------------------------

// TestSSE_PostMessage opens an SSE stream, extracts the session endpoint from
// the first event, POSTs a JSON-RPC request, and reads the response back over
// the SSE stream.
func TestSSE_PostMessage(t *testing.T) {
	baseURL, cancel := startSSE(t)
	defer cancel()

	// Connect to SSE stream.
	sseCtx, sseCancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer sseCancel()

	sseReq, _ := http.NewRequestWithContext(sseCtx, http.MethodGet, baseURL+"/sse", nil)
	sseResp, err := http.DefaultClient.Do(sseReq)
	if err != nil {
		t.Fatalf("GET /sse: %v", err)
	}
	defer sseResp.Body.Close()

	// Parse the endpoint event to extract the POST URL.
	lines := readSSELines(sseCtx, sseResp.Body, 2)
	if len(lines) < 2 {
		t.Fatalf("expected 2 SSE lines, got %d", len(lines))
	}
	dataLine := strings.TrimPrefix(lines[1], "data: ")
	dataLine = strings.TrimSpace(dataLine)
	postURL := baseURL + dataLine // e.g. http://…/message?sessionId=client-…

	// POST a JSON-RPC request.
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      99,
		"method":  "tools/list",
	})
	postResp, err := http.Post(postURL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", postURL, err)
	}
	defer postResp.Body.Close()

	if postResp.StatusCode != http.StatusAccepted {
		t.Errorf("POST status = %d, want 202", postResp.StatusCode)
	}

	// Read the response that arrives on the SSE stream.
	respLines := readSSELines(sseCtx, sseResp.Body, 2)
	var msgData string
	for _, l := range respLines {
		if strings.HasPrefix(l, "data:") {
			msgData = strings.TrimPrefix(l, "data:")
			msgData = strings.TrimSpace(msgData)
		}
	}
	if msgData == "" {
		t.Fatal("no data received on SSE stream after POST")
	}

	var rpcResp transport.JSONRPCResponse
	if err := json.Unmarshal([]byte(msgData), &rpcResp); err != nil {
		t.Fatalf("failed to unmarshal SSE response: %v", err)
	}
	if rpcResp.Error != nil {
		t.Errorf("unexpected RPC error: %v", rpcResp.Error.Message)
	}
	if fmt.Sprint(rpcResp.ID) != "99" {
		t.Errorf("response ID = %v, want 99", rpcResp.ID)
	}
}

// ---- TestSSE_InvalidSessionID -----------------------------------------------

func TestSSE_InvalidSessionID(t *testing.T) {
	baseURL, cancel := startSSE(t)
	defer cancel()

	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "ping"})
	resp, err := http.Post(baseURL+"/message?sessionId=does-not-exist",
		"application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /message: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for invalid sessionId", resp.StatusCode)
	}
}

// ---- TestSSE_MissingSessionID -----------------------------------------------

func TestSSE_MissingSessionID(t *testing.T) {
	baseURL, cancel := startSSE(t)
	defer cancel()

	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "ping"})
	resp, err := http.Post(baseURL+"/message",
		"application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /message: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 when sessionId is missing", resp.StatusCode)
	}
}

// ---- TestSSE_NonPostRejected ------------------------------------------------

func TestSSE_NonPostRejected(t *testing.T) {
	baseURL, cancel := startSSE(t)
	defer cancel()

	resp, err := http.Get(baseURL + "/message?sessionId=whatever")
	if err != nil {
		t.Fatalf("GET /message: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405 for non-POST on /message", resp.StatusCode)
	}
}

// ---- TestSSE_InvalidJSON ----------------------------------------------------

func TestSSE_InvalidJSON(t *testing.T) {
	baseURL, cancel := startSSE(t)
	defer cancel()

	// Connect first to get a real session.
	sseCtx, sseCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer sseCancel()

	sseReq, _ := http.NewRequestWithContext(sseCtx, http.MethodGet, baseURL+"/sse", nil)
	sseResp, err := http.DefaultClient.Do(sseReq)
	if err != nil {
		t.Fatalf("GET /sse: %v", err)
	}
	defer sseResp.Body.Close()

	lines := readSSELines(sseCtx, sseResp.Body, 2)
	if len(lines) < 2 {
		t.Fatalf("expected 2 SSE lines, got %d", len(lines))
	}
	dataLine := strings.TrimSpace(strings.TrimPrefix(lines[1], "data:"))
	postURL := baseURL + dataLine

	// POST invalid JSON.
	postResp, err := http.Post(postURL, "application/json",
		strings.NewReader("this is not json"))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer postResp.Body.Close()

	if postResp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for invalid JSON body", postResp.StatusCode)
	}
}

// ---- TestSSE_Notification ---------------------------------------------------

// TestSSE_Notification verifies that posting a notification (no "id" field)
// is accepted without an SSE response (notifications are fire-and-forget).
func TestSSE_Notification(t *testing.T) {
	baseURL, cancel := startSSE(t)
	defer cancel()

	sseCtx, sseCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer sseCancel()

	sseReq, _ := http.NewRequestWithContext(sseCtx, http.MethodGet, baseURL+"/sse", nil)
	sseResp, err := http.DefaultClient.Do(sseReq)
	if err != nil {
		t.Fatalf("GET /sse: %v", err)
	}
	defer sseResp.Body.Close()

	lines := readSSELines(sseCtx, sseResp.Body, 2)
	if len(lines) < 2 {
		t.Fatalf("expected 2 SSE lines, got %d", len(lines))
	}
	dataLine := strings.TrimSpace(strings.TrimPrefix(lines[1], "data:"))
	postURL := baseURL + dataLine

	// POST a notification (no id field).
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	})
	postResp, err := http.Post(postURL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST notification: %v", err)
	}
	defer postResp.Body.Close()

	if postResp.StatusCode != http.StatusAccepted {
		t.Errorf("notification POST status = %d, want 202", postResp.StatusCode)
	}
}
