package mcpx

import (
	"fmt"
	"strings"
)

// ToolError is returned when a tool handler encounters a failure.
// It carries the name of the tool, a human-readable message, and an optional
// underlying cause.
type ToolError struct {
	Tool    string
	Message string
	Cause   error
}

// Error implements the error interface.
func (e *ToolError) Error() string {
	if e.Cause != nil && e.Message != "" {
		return fmt.Sprintf("tool %q: %s: %v", e.Tool, e.Message, e.Cause)
	}
	if e.Cause != nil {
		return fmt.Sprintf("tool %q: %v", e.Tool, e.Cause)
	}
	return fmt.Sprintf("tool %q: %s", e.Tool, e.Message)
}

// Unwrap returns the underlying cause so errors.Is / errors.As can traverse
// the error chain.
func (e *ToolError) Unwrap() error {
	return e.Cause
}

// SchemaError is returned when automatic JSON Schema generation fails for a
// Go type. Type is the stringified type name that triggered the failure.
type SchemaError struct {
	Type    string
	Message string
}

// Error implements the error interface.
func (e *SchemaError) Error() string {
	return fmt.Sprintf("schema generation failed for type %q: %s", e.Type, e.Message)
}

// ValidationError is returned when incoming tool arguments fail validation.
// Errors contains one entry per violated constraint.
type ValidationError struct {
	Tool   string
	Errors []string
}

// Error implements the error interface. Multiple validation errors are joined
// with a semicolon so the message is still a single line.
func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation failed for tool %q: %s", e.Tool, strings.Join(e.Errors, "; "))
}

// RegistrationError is returned when a tool, resource, or prompt cannot be
// registered — for example when the name is empty or already taken.
type RegistrationError struct {
	Name    string
	Message string
}

// Error implements the error interface.
func (e *RegistrationError) Error() string {
	return fmt.Sprintf("registration failed for %q: %s", e.Name, e.Message)
}
