package middleware

import (
	"context"
	"errors"
)

// TransformerFunc is a function that transforms a value.
// It is used to transform args or result.
type TransformerFunc func(in any) (any, error)

// TransformationConfig holds the configuration for the Transformation middleware.
type TransformationConfig struct {
	// ArgsTransformer, if set, transforms the args before calling the handler.
	ArgsTransformer TransformerFunc
	// ResultTransformer, if set, transforms the result (if no error) after calling the handler.
	ResultTransformer TransformerFunc
}

// TransformationOption is a functional option for configuring the Transformation middleware.
type TransformationOption func(*TransformationConfig)

// WithArgsTransformer sets the args transformer.
func WithArgsTransformer(tf TransformerFunc) TransformationOption {
	return func(c *TransformationConfig) {
		c.ArgsTransformer = tf
	}
}

// WithResultTransformer sets the result transformer.
func WithResultTransformer(tf TransformerFunc) TransformationOption {
	return func(c *TransformationConfig) {
		c.ResultTransformer = tf
	}
}

// Transformation returns a Middleware that transforms args and/or result.
func Transformation(opts ...TransformationOption) Middleware {
	cfg := &TransformationConfig{}
	for _, o := range opts {
		o(cfg)
	}

	return func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, toolName string, args map[string]any) (any, error) {
			// Transform args if a transformer is set.
			transformedArgs := args
			if cfg.ArgsTransformer != nil {
				var err error
				transformedArgs, err = cfg.ArgsTransformer(args)
				if err != nil {
					return nil, err
				}
			}

			// Call the next handler with transformed args.
			result, err := next(ctx, toolName, transformedArgs)
			if err != nil {
				return result, err
			}

			// Transform result if a transformer is set and there's no error.
			if cfg.ResultTransformer != nil {
				var err error
				result, err = cfg.ResultTransformer(result)
				if err != nil {
					return nil, err
				}
			}
			return result, err
		}
	}
}