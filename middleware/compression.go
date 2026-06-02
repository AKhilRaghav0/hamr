package middleware

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
)

// compressionConfig holds the resolved configuration for the Compression middleware.
type compressionConfig struct {
	level int
}

// CompressionOption is a functional option for configuring the Compression middleware.
type CompressionOption func(*compressionConfig)

// WithCompressionLevel sets the gzip compression level (default is gzip.DefaultCompression).
func WithCompressionLevel(level int) CompressionOption {
	return func(c *compressionConfig) {
		if level >= gzip.HuffmanOnly && level <= gzip.BestCompression {
			c.level = level
		}
	}
}

// Compression returns a Middleware that compresses string and []byte results using gzip.
// It also decompresses incoming args if they are gzip-compressed (detected by magic bytes).
//
// Note: This middleware assumes that the args map values and result are either plain
// or gzip-compressed byte slices. It will attempt to decompress any []byte value in args
// that starts with the gzip magic number. For results, if the result is a string or []byte,
// it will compress it.
func Compression(opts ...CompressionOption) Middleware {
	cfg := &compressionConfig{
		level: gzip.DefaultCompression,
	}
	for _, o := range opts {
		o(cfg)
	}

	return func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, toolName string, args map[string]any) (any, error) {
			// Decompress any gzip-compressed values in args
			decompressedArgs := make(map[string]any, len(args))
			for k, v := range args {
				if b, ok := v.([]byte); ok && len(b) >= 2 && b[0] == 0x1f && b[1] == 0x8b {
					// Gzip magic number
					gr, err := gzip.NewReader(bytes.NewReader(b))
					if err != nil {
						return nil, err
					}
					defer gr.Close()
					decompressed, err := io.ReadAll(gr)
					if err != nil {
						return nil, err
					}
					decompressedArgs[k] = decompressed
				} else {
					decompressedArgs[k] = v
				}
			}

			result, err := next(ctx, toolName, decompressedArgs)
			if err != nil {
				return result, err
			}

			// Compress string and []byte results
			switch v := result.(type) {
			case string:
				var buf bytes.Buffer
				gw := gzip.NewWriter(&buf)
				gw.Level = cfg.level
				if _, err := gw.Write([]byte(v)); err != nil {
					return nil, err
				}
				if err := gw.Close(); err != nil {
					return nil, err
				}
				return buf.Bytes(), nil
			case []byte:
				var buf bytes.Buffer
				gw := gzip.NewWriter(&buf)
				gw.Level = cfg.level
				if _, err := gw.Write(v); err != nil {
					return nil, err
				}
				if err := gw.Close(); err != nil {
					return nil, err
				}
				return buf.Bytes(), nil
			default:
				// For other types, return as-is
				return result, nil
			}
		}
	}
}