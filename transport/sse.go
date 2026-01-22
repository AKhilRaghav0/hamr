package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// SSETransport serves MCP over HTTP with Server-Sent Events.
type SSETransport struct {
	handler Handler
	addr    string
	logger  *slog.Logger
	clients map[string]*sseClient
	mu      sync.RWMutex
}

type sseClient struct {
	id      string
	msgChan chan []byte
	done    chan struct{}
}

// NewSSE creates an SSE transport.
func NewSSE(handler Handler, addr string, logger *slog.Logger) *SSETransport {
	return &SSETransport{
		handler: handler,
		addr:    addr,
		logger:  logger,
		clients: make(map[string]*sseClient),
	}
}

// Run starts the SSE HTTP server. Blocks until context cancellation.
func (t *SSETransport) Run(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/sse", t.handleSSE)
	mux.HandleFunc("/message", t.handleMessage)

	server := &http.Server{
		Addr:    t.addr,
		Handler: mux,
	}

	// Shutdown on context cancel
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
	}()

	t.logger.Info("SSE transport listening", "addr", t.addr)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		return fmt.Errorf("SSE server error: %w", err)
	}
	return nil
}

func (t *SSETransport) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	clientID := fmt.Sprintf("client-%d", time.Now().UnixNano())
	client := &sseClient{
		id:      clientID,
		msgChan: make(chan []byte, 64),
		done:    make(chan struct{}),
	}

	t.mu.Lock()
	t.clients[clientID] = client
	t.mu.Unlock()

	defer func() {
		t.mu.Lock()
		delete(t.clients, clientID)
		t.mu.Unlock()
		close(client.done)
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Send the endpoint event so the client knows where to POST
	fmt.Fprintf(w, "event: endpoint\ndata: /message?sessionId=%s\n\n", clientID)
	flusher.Flush()

	t.logger.Info("SSE client connected", "id", clientID)

	for {
		select {
		case msg := <-client.msgChan:
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", msg)
			flusher.Flush()
		case <-r.Context().Done():
			t.logger.Info("SSE client disconnected", "id", clientID)
			return
		}
	}
}

func (t *SSETransport) handleMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionID := r.URL.Query().Get("sessionId")
	if sessionID == "" {
		http.Error(w, "Missing sessionId", http.StatusBadRequest)
		return
	}

	t.mu.RLock()
	client, ok := t.clients[sessionID]
	t.mu.RUnlock()

	if !ok {
		http.Error(w, "Unknown session", http.StatusNotFound)
		return
	}

	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	fullJSON, _ := json.Marshal(raw)

	// Check if request or notification
	if _, hasID := raw["id"]; hasID {
		var req JSONRPCRequest
		if err := json.Unmarshal(fullJSON, &req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		resp := t.handler.HandleRequest(r.Context(), &req)
		if resp != nil {
			respJSON, _ := json.Marshal(resp)
			select {
			case client.msgChan <- respJSON:
			default:
				t.logger.Warn("SSE message buffer full", "client", sessionID)
			}
		}
	} else {
		var notif JSONRPCNotification
		if err := json.Unmarshal(fullJSON, &notif); err != nil {
			http.Error(w, "Invalid notification", http.StatusBadRequest)
			return
		}
		t.handler.HandleNotification(r.Context(), &notif)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(`{"status":"accepted"}`))
}
