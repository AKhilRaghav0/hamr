package toolbox

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/AKhilRaghav0/hamr"
)

const (
	defaultHTTPTimeout = 30 * time.Second
	defaultMaxBodySize = 1 << 20 // 1 MiB
)

// httpConfig holds the configuration for HTTPTools.
type httpConfig struct {
	timeout     time.Duration
	maxBodySize int64
}

// HTTPOption is a functional option for HTTPTools.
type HTTPOption func(*httpConfig)

// WithTimeout sets the HTTP client timeout. Default is 30 seconds.
func WithTimeout(d time.Duration) HTTPOption {
	return func(c *httpConfig) {
		c.timeout = d
	}
}

// WithMaxBodySize sets the maximum number of bytes read from a response body.
// Responses larger than this are truncated. Default is 1 MiB.
func WithMaxBodySize(n int64) HTTPOption {
	return func(c *httpConfig) {
		c.maxBodySize = n
	}
}

// HTTPTools is a collection of HTTP request tools.
type HTTPTools struct {
	client *http.Client
	cfg    httpConfig
}

// HTTP returns an HTTPTools collection with the given options applied.
func HTTP(opts ...HTTPOption) *HTTPTools {
	cfg := httpConfig{
		timeout:     defaultHTTPTimeout,
		maxBodySize: defaultMaxBodySize,
	}
	for _, o := range opts {
		o(&cfg)
	}
	return &HTTPTools{
		client: &http.Client{Timeout: cfg.timeout},
		cfg:    cfg,
	}
}

// Tools implements hamr.ToolCollection.
func (h *HTTPTools) Tools() []hamr.ToolInfo {
	return []hamr.ToolInfo{
		{
			Name:        "http_get",
			Description: "Send an HTTP GET request to a URL and return the response body.",
			Handler:     h.httpGet,
		},
		{
			Name:        "http_post",
			Description: "Send an HTTP POST request to a URL with a body and return the response.",
			Handler:     h.httpPost,
		},
		{
			Name:        "fetch_url",
			Description: "Fetch a URL and return its text content together with the HTTP status code.",
			Handler:     h.fetchURL,
		},
	}
}

// ---- input structs ----

// HTTPGetInput is the input for the http_get tool.
type HTTPGetInput struct {
	URL     string            `json:"url" desc:"the URL to GET"`
	Headers map[string]string `json:"headers" desc:"optional request headers" optional:"true"`
}

// HTTPPostInput is the input for the http_post tool.
type HTTPPostInput struct {
	URL         string            `json:"url" desc:"the URL to POST to"`
	Body        string            `json:"body" desc:"the request body"`
	ContentType string            `json:"content_type" desc:"Content-Type header value; defaults to text/plain" optional:"true"`
	Headers     map[string]string `json:"headers" desc:"optional additional request headers" optional:"true"`
}

// FetchURLInput is the input for the fetch_url tool.
type FetchURLInput struct {
	URL string `json:"url" desc:"the URL to fetch"`
}

// ---- helpers ----

// readBody reads at most maxBodySize bytes from r and closes it.
func (h *HTTPTools) readBody(r io.ReadCloser) (string, error) {
	defer r.Close()
	limited := io.LimitReader(r, h.cfg.maxBodySize)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("reading response body: %w", err)
	}
	return string(data), nil
}

// doRequest executes req, reads the body and returns (status, body, error).
func (h *HTTPTools) doRequest(ctx context.Context, req *http.Request) (int, string, error) {
	req = req.WithContext(ctx)
	resp, err := h.client.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("request failed: %w", err)
	}
	body, err := h.readBody(resp.Body)
	if err != nil {
		return resp.StatusCode, "", err
	}
	return resp.StatusCode, body, nil
}

// ---- handlers ----

func (h *HTTPTools) httpGet(ctx context.Context, in HTTPGetInput) (string, error) {
	req, err := http.NewRequest(http.MethodGet, in.URL, nil)
	if err != nil {
		return "", fmt.Errorf("http_get: build request: %w", err)
	}
	for k, v := range in.Headers {
		req.Header.Set(k, v)
	}

	status, body, err := h.doRequest(ctx, req)
	if err != nil {
		return "", fmt.Errorf("http_get: %w", err)
	}
	return fmt.Sprintf("HTTP %d\n\n%s", status, body), nil
}

func (h *HTTPTools) httpPost(ctx context.Context, in HTTPPostInput) (string, error) {
	ct := in.ContentType
	if ct == "" {
		ct = "text/plain"
	}

	req, err := http.NewRequest(http.MethodPost, in.URL, strings.NewReader(in.Body))
	if err != nil {
		return "", fmt.Errorf("http_post: build request: %w", err)
	}
	req.Header.Set("Content-Type", ct)
	for k, v := range in.Headers {
		req.Header.Set(k, v)
	}

	status, body, err := h.doRequest(ctx, req)
	if err != nil {
		return "", fmt.Errorf("http_post: %w", err)
	}
	return fmt.Sprintf("HTTP %d\n\n%s", status, body), nil
}

func (h *HTTPTools) fetchURL(ctx context.Context, in FetchURLInput) (string, error) {
	req, err := http.NewRequest(http.MethodGet, in.URL, nil)
	if err != nil {
		return "", fmt.Errorf("fetch_url: build request: %w", err)
	}

	status, body, err := h.doRequest(ctx, req)
	if err != nil {
		return "", fmt.Errorf("fetch_url: %w", err)
	}
	return fmt.Sprintf("Status: %d\n\n%s", status, body), nil
}
