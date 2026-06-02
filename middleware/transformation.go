package middleware

import "context"

// transformationConfig holds the resolved configuration for the Transformation middleware.
type transformationConfig struct {
	argsTransform   func(map[string]any) map[string]any
	resultTransform func(any) any
}

// TransformationOption is a functional option for configuring the Transformation middleware.
type TransformationOption func(*transformationConfig)

// WithArgsTransform sets a function to transform the arguments before passing them to the next handler.
func WithArgsTransform(transform func(map[string]any) map[string]any) TransformationOption {
	return func(c *transformationConfig) {
		c.argsTransform = transform
	}
}

// WithResultTransform sets a function to transform the result returned by the next handler.
func WithResultTransform(transform func(any) any) TransformationOption {
	return func(c *transformationConfig) {
		c.resultTransform = transform
	}
}

// Transformation returns a Middleware that applies transformation functions to the
// args and/or result. If no transformation functions are set, it acts as a pass-through.
func Transformation(opts ...TransformationOption) Middleware {
	cfg := &transformationConfig{}
	for _, o := range opts {
		o(cfg)
	}

	return func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, toolName string, args map[string]any) (any, error) {
			// Transform args if a function is set.
			if cfg.argsTransform != nil {
				args = cfg.argsTransform(args)
			}

			result, err := next(ctx, toolName, args)
			if err != nil {
				return result, err
			}

			// Transform result if a function is set.
			if cfg.resultTransform != nil {
				result = cfg.resultTransform(result)
			}

			return result, nil
		}
	}
}