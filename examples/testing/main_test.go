// Package main demonstrates how to write tests for mcpx tools using hamrtest.
//
// The test file lives alongside the server code (or in its own package) and uses
// hamrtest.NewClient to communicate with the server in-memory — no processes,
// no network, no test setup/teardown beyond standard testing.T.
package main

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/AKhilRaghav0/hamr"
	"github.com/AKhilRaghav0/hamr/hamrtest"
)

// ---- input types ------------------------------------------------------------

type SearchInput struct {
	Query      string `json:"query" desc:"search query" required:"true"`
	MaxResults int    `json:"max_results" desc:"maximum results to return" default:"10"`
	Format     string `json:"format" desc:"output format" enum:"json,text,markdown" default:"text"`
}

type CalculateInput struct {
	Expression string `json:"expression" desc:"math expression to evaluate" required:"true"`
}

type FormatInput struct {
	Text  string `json:"text" desc:"text to format" required:"true"`
	Style string `json:"style" desc:"formatting style" enum:"upper,lower,title" default:"lower"`
}

// ---- handlers ---------------------------------------------------------------

func handleSearch(_ context.Context, in SearchInput) (string, error) {
	return fmt.Sprintf("Results for %q (max=%d, format=%s):\n1. First result\n2. Second result",
		in.Query, in.MaxResults, in.Format), nil
}

func handleCalculate(_ context.Context, in CalculateInput) (string, error) {
	// Minimal stub — real implementation would parse and evaluate the expression.
	return fmt.Sprintf("evaluated: %s", in.Expression), nil
}

func handleFormat(_ context.Context, in FormatInput) (string, error) {
	switch in.Style {
	case "upper":
		return strings.ToUpper(in.Text), nil
	case "title":
		return strings.Title(in.Text), nil //nolint:staticcheck // acceptable in example
	default:
		return strings.ToLower(in.Text), nil
	}
}

// ---- server factory ---------------------------------------------------------

// newServer builds a server with all tools registered. Tests call this to get
// a fresh, isolated server instance per test (or share one via TestMain).
func newServer() *hamr.Server {
	s := hamr.New("test-demo-server", "1.0.0")
	s.Tool("search", "Search for information", handleSearch)
	s.Tool("calculate", "Evaluate a mathematical expression", handleCalculate)
	s.Tool("format", "Format text with a chosen style", handleFormat)
	return s
}

// ---- tests ------------------------------------------------------------------

// TestToolExists verifies that each registered tool appears in tools/list.
func TestToolExists(t *testing.T) {
	client := hamrtest.NewClient(t, newServer().NewTestHandler())

	hamrtest.AssertToolExists(t, client, "search")
	hamrtest.AssertToolExists(t, client, "calculate")
	hamrtest.AssertToolExists(t, client, "format")
}

// TestToolCount verifies the exact number of registered tools.
func TestToolCount(t *testing.T) {
	client := hamrtest.NewClient(t, newServer().NewTestHandler())
	hamrtest.AssertToolCount(t, client, 3)
}

// TestToolCall_ValidInput exercises a normal, successful tool call.
func TestToolCall_ValidInput(t *testing.T) {
	client := hamrtest.NewClient(t, newServer().NewTestHandler())

	result, err := client.CallTool("search", map[string]any{
		"query": "mcpx framework",
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result.IsError {
		t.Errorf("isError should be false, got text: %s", result.Text())
	}

	text := result.Text()
	if !strings.Contains(text, "mcpx framework") {
		t.Errorf("result text missing query string; got: %s", text)
	}
}

// TestToolCall_DefaultsApplied checks that omitted fields are filled from defaults.
func TestToolCall_DefaultsApplied(t *testing.T) {
	client := hamrtest.NewClient(t, newServer().NewTestHandler())

	// Only supply the required "query" field; expect max_results=10 and format=text.
	result, err := client.CallTool("search", map[string]any{
		"query": "defaults test",
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}

	text := result.Text()
	if !strings.Contains(text, "max=10") {
		t.Errorf("default max_results not applied; result: %s", text)
	}
	if !strings.Contains(text, "format=text") {
		t.Errorf("default format not applied; result: %s", text)
	}
}

// TestToolCall_MissingRequiredField verifies that omitting a required field
// surfaces as a validation error (isError=true) rather than a panic or crash.
func TestToolCall_MissingRequiredField(t *testing.T) {
	client := hamrtest.NewClient(t, newServer().NewTestHandler())

	// "query" is required — omit it entirely.
	result, err := client.CallTool("search", map[string]any{})
	if err != nil {
		// CallTool itself should not fail — validation errors come back as tool results.
		t.Fatalf("CallTool should not return RPC error for validation failures: %v", err)
	}
	if !result.IsError {
		t.Error("isError should be true when a required field is missing")
	}
	if result.Text() == "" {
		t.Error("validation error message should not be empty")
	}
}

// TestToolCall_WrongType passes a boolean where a string is expected.
func TestToolCall_WrongType(t *testing.T) {
	client := hamrtest.NewClient(t, newServer().NewTestHandler())

	result, err := client.CallTool("search", map[string]any{
		"query":       true, // boolean, not string
		"max_results": 5,
	})
	if err != nil {
		t.Fatalf("CallTool should not return RPC error: %v", err)
	}
	if !result.IsError {
		t.Error("isError should be true when field has wrong type")
	}
}

// TestToolCall_EnumViolation passes a value outside the declared enum.
func TestToolCall_EnumViolation(t *testing.T) {
	client := hamrtest.NewClient(t, newServer().NewTestHandler())

	result, err := client.CallTool("search", map[string]any{
		"query":  "test",
		"format": "xml", // not in enum: json, text, markdown
	})
	if err != nil {
		t.Fatalf("CallTool should not return RPC error: %v", err)
	}
	if !result.IsError {
		t.Error("isError should be true when enum constraint is violated")
	}
}

// TestToolCall_EnumValid verifies that a valid enum value is accepted.
func TestToolCall_EnumValid(t *testing.T) {
	client := hamrtest.NewClient(t, newServer().NewTestHandler())

	for _, format := range []string{"json", "text", "markdown"} {
		format := format
		t.Run(format, func(t *testing.T) {
			result, err := client.CallTool("search", map[string]any{
				"query":  "enum test",
				"format": format,
			})
			if err != nil {
				t.Fatalf("CallTool error: %v", err)
			}
			if result.IsError {
				t.Errorf("unexpected validation error for valid enum %q: %s", format, result.Text())
			}
		})
	}
}

// TestToolCall_StyleEnum verifies the "style" enum on the format tool.
func TestToolCall_StyleEnum(t *testing.T) {
	client := hamrtest.NewClient(t, newServer().NewTestHandler())

	result, err := client.CallTool("format", map[string]any{
		"text":  "Hello World",
		"style": "upper",
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Text())
	}
	if result.Text() != "HELLO WORLD" {
		t.Errorf("text = %q, want %q", result.Text(), "HELLO WORLD")
	}
}

// TestInitialize verifies the server handshake response.
func TestInitialize(t *testing.T) {
	client := hamrtest.NewClient(t, newServer().NewTestHandler())

	info := client.Initialize()
	if info["protocolVersion"] == "" {
		t.Error("protocolVersion missing from initialize response")
	}
	serverInfo, ok := info["serverInfo"].(map[string]any)
	if !ok {
		t.Fatal("serverInfo missing or wrong type")
	}
	if serverInfo["name"] != "test-demo-server" {
		t.Errorf("server name = %v, want %q", serverInfo["name"], "test-demo-server")
	}
}

// TestToolDef_SchemaPresent verifies that each tool exposes an inputSchema.
func TestToolDef_SchemaPresent(t *testing.T) {
	client := hamrtest.NewClient(t, newServer().NewTestHandler())
	tools := client.ListTools()

	for _, tool := range tools {
		if tool.InputSchema == nil {
			t.Errorf("tool %q has nil inputSchema", tool.Name)
		}
	}
}
