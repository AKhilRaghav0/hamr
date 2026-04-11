package hamr

import "log/slog"

// serverConfig holds the configuration values accumulated by Option functions.
// The Server type (defined elsewhere) embeds or consumes this struct during
// construction.
type serverConfig struct {
	logger         *slog.Logger
	version        string
	transport      string
	description    string
	minimalSchemas bool
}

// Option is a functional option that configures a Server.
// Pass Option values to the Server constructor to customise behaviour.
type Option func(*serverConfig)

// WithLogger sets a custom structured logger on the server.
// When not provided the server uses slog.Default().
func WithLogger(logger *slog.Logger) Option {
	return func(c *serverConfig) {
		c.logger = logger
	}
}

// WithVersion sets the version string advertised in the MCP server-info
// handshake (e.g. "1.2.3").
func WithVersion(version string) Option {
	return func(c *serverConfig) {
		c.version = version
	}
}

// WithTransport sets the transport the server listens on.
// Accepted values are "stdio" (default) and "sse".
func WithTransport(transport string) Option {
	return func(c *serverConfig) {
		c.transport = transport
	}
}

// WithDescription sets a human-readable description of the server that is
// included in the MCP server-info response.
func WithDescription(desc string) Option {
	return func(c *serverConfig) {
		c.description = desc
	}
}

// WithMinimalSchemas strips verbose schema fields (description, default, enum,
// minimum, maximum, pattern, format) from the tools/list response to reduce
// token usage when the AI reads the tool catalogue. Validation still uses the
// full schema; only the wire representation is trimmed.
func WithMinimalSchemas() Option {
	return func(c *serverConfig) {
		c.minimalSchemas = true
	}
}

// defaultConfig returns a serverConfig populated with sensible defaults.
func defaultConfig() serverConfig {
	return serverConfig{
		logger:    slog.Default(),
		version:   "0.0.1",
		transport: "stdio",
	}
}

// applyOptions applies all provided options to a config, starting from the
// defaults.
func applyOptions(opts []Option) serverConfig {
	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}
	return cfg
}
