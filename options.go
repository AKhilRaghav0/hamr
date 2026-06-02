package hamr

import (
	"encoding/json"
	"fmt"
	"fsnotify"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"log/slog"

	"gopkg.in/yaml.v3"
	"github.com/BurntSushi/toml"
)

// serverConfig holds the configuration values accumulated by Option functions.
// The Server type (defined elsewhere) embeds or consumes this struct during
// construction.
type serverConfig struct {
	logger         *slog.Logger
	version        string
	transport      string
	description    string
	minimalSchemas bool

	// configFile is the path to the configuration file for hot-reload.
	// If non-empty, a file watcher will be started to reload the config on changes.
	configFile string
	// envPrefix is the prefix for environment variables for hot-reload.
	// If non-empty, the server will check for changes in environment variables
	// (though this is less common; we might not implement hot-reload for env).
	envPrefix string
	// watcher is the file watcher for hot-reload.
	watcher *fsnotify.Watcher
	// watcherDone is a channel that is closed when the watcher should stop.
	watcherDone chan struct{}
}

// fileConfig is the subset of serverConfig that can be set from configuration files
// and environment variables (excluding logger which must be set via functional option).
type fileConfig struct {
	Version        string
	Transport      string
	Description    string
	MinimalSchemas bool
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

// WithConfigFile loads configuration from the given file (YAML, JSON, or TOML)
// and applies it to the server config. Supported extensions: .json, .yaml, .yml, .toml.
// If the file cannot be read or parsed, the function panics.
// It also sets up hot-reload for the configuration file.
func WithConfigFile(path string) Option {
	return func(c *serverConfig) {
		data, err := os.ReadFile(path)
		if err != nil {
			panic(fmt.Errorf("failed to read config file %s: %w", path, err))
		}

		var f fileConfig
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".json":
			if err := json.Unmarshal(data, &f); err != nil {
				panic(fmt.Errorf("failed to parse JSON config file %s: %w", path, err))
			}
		case ".yaml", ".yml":
			if err := yaml.Unmarshal(data, &f); err != nil {
				panic(fmt.Errorf("failed to parse YAML config file %s: %w", path, err))
			}
		case ".toml":
			if err := toml.Unmarshal(data, &f); err != nil {
				panic(fmt.Errorf("failed to parse TOML config file %s: %w", path, err))
			}
		default:
			panic(fmt.Sprintf("unsupported config file extension %s for file %s", ext, path))
		}

		// Apply the file config to the server config.
		if f.Version != "" {
			c.version = f.Version
		}
		if f.Transport != "" {
			c.transport = f.Transport
		}
		if f.Description != "" {
			c.description = f.Description
		}
		if f.MinimalSchemas {
			c.minimalSchemas = f.MinimalSchemas
		}

		// Set the config file for hot-reload.
		c.configFile = path
	}
}

// WithEnvPrefix binds environment variables with the given prefix to the server config.
// The prefix should be in the form "HAMR_" or similar. The environment variable names
// are constructed by prefixing the field name in uppercase: e.g., "HAMR_VERSION"
// for the Version field. The function automatically converts the environment variable
// value to the appropriate type. If an environment variable is set but cannot be
// converted, the function panics.
// It also sets up hot-reload for environment variables (checking for changes).
func WithEnvPrefix(prefix string) Option {
	return func(c *serverConfig) {
		// Version
		if v := os.Getenv(prefix + "VERSION"); v != "" {
			c.version = v
		}
		// Transport
		if v := os.Getenv(prefix + "TRANSPORT"); v != "" {
			c.transport = v
		}
		// Description
		if v := os.Getenv(prefix + "DESCRIPTION"); v != "" {
			c.description = v
		}
		// MinimalSchemas
		if v := os.Getenv(prefix + "MINIMAL_SCHEMAS"); v != "" {
			b, err := strconv.ParseBool(v)
			if err != nil {
				panic(fmt.Errorf("invalid boolean value for %sMINIMAL_SCHEMAS: %w", prefix, err))
			}
			c.minimalSchemas = b
		}

		// Set the env prefix for hot-reload.
		c.envPrefix = prefix
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

// reloadConfig reloads the configuration from the file (if set) and environment
// variables (if set) into the given serverConfig.
// It returns an error if the file cannot be read or parsed, or if an environment
// variable has an invalid format.
// The caller must lock the server's mu before calling this function if concurrent
// access is possible.
func reloadConfig(cfg *serverConfig) error {
	// Reload from file if configFile is set.
	if cfg.configFile != "" {
		data, err := os.ReadFile(cfg.configFile)
		if err != nil {
			return fmt.Errorf("failed to read config file %s: %w", cfg.configFile, err)
		}

		var f fileConfig
		ext := strings.ToLower(filepath.Ext(cfg.configFile))
		switch ext {
		case ".json":
			if err := json.Unmarshal(data, &f); err != nil {
				return fmt.Errorf("failed to parse JSON config file %s: %w", cfg.configFile, err)
			}
		case ".yaml", ".yml":
			if err := yaml.Unmarshal(data, &f); err != nil {
				return fmt.Errorf("failed to parse YAML config file %s: %w", cfg.configFile, err)
			}
		case ".toml":
			if err := toml.Unmarshal(data, &f); err != nil {
				return fmt.Errorf("failed to parse TOML config file %s: %w", cfg.configFile, err)
			}
		default:
			return fmt.Errorf("unsupported config file extension %s for file %s", ext, cfg.configFile)
		}

		// Apply the file config to the server config.
		if f.Version != "" {
			cfg.version = f.Version
		}
		if f.Transport != "" {
			cfg.transport = f.Transport
		}
		if f.Description != "" {
			cfg.description = f.Description
		}
		if f.MinimalSchemas {
			cfg.minimalSchemas = f.MinimalSchemas
		}
	}

	// Reload from environment variables if envPrefix is set.
	if cfg.envPrefix != "" {
		// Version
		if v := os.Getenv(cfg.envPrefix + "VERSION"); v != "" {
			cfg.version = v
		}
		// Transport
		if v := os.Getenv(cfg.envPrefix + "TRANSPORT"); v != "" {
			cfg.transport = v
		}
		// Description
		if v := os.Getenv(cfg.envPrefix + "DESCRIPTION"); v != "" {
			cfg.description = v
		}
		// MinimalSchemas
		if v := os.Getenv(cfg.envPrefix + "MINIMAL_SCHEMAS"); v != "" {
			b, err := strconv.ParseBool(v)
			if err != nil {
				return fmt.Errorf("invalid boolean value for %sMINIMAL_SCHEMAS: %w", cfg.envPrefix, err)
			}
			cfg.minimalSchemas = b
		}
	}

	return nil
}