// Package mcpx is a high-level Go framework for building MCP (Model Context Protocol) servers.
// It eliminates boilerplate by auto-generating JSON schemas from Go types, handling transport,
// validation, middleware, and providing a clean developer experience.
//
// Example:
//
//	type SearchInput struct {
//	    Query      string `json:"query" desc:"search query"`
//	    MaxResults int    `json:"max_results" desc:"max results" default:"10"`
//	}
//
//	func main() {
//	    s := hamr.New("my-server", "1.0.0")
//	    s.Tool("search", "Search the web", func(ctx context.Context, input SearchInput) (string, error) {
//	        return "results", nil
//	    })
//	    s.Run()
//	}
package hamr

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"sync"
	"time"

	"github.com/AKhilRaghav0/hamr/middleware"
	"github.com/AKhilRaghav0/hamr/schema"
	"github.com/AKhilRaghav0/hamr/validate"
)

// Server is the central type in hamr. It holds all registered tools, resources,
// and prompts, manages global and per-tool middleware, and owns the transport
// lifecycle.
//
// Create a Server with New, register handlers with Tool / Resource / Prompt,
// then call Run (stdio) or RunSSE (HTTP) to start serving MCP clients.
type Server struct {
	name        string
	version     string
	config      serverConfig
	tools       map[string]*toolDef
	resources   map[string]*resourceDef
	prompts     map[string]*promptDef
	toolGroups  map[string]*Group
	middlewares []middleware.Middleware
	mu          sync.RWMutex
	logger      *slog.Logger
	onCall      func(tool string, args map[string]any, duration time.Duration, err error) // optional callback
}

// Group is a named collection of related tools that are presented to AI clients
// as a single opaque selector entry in tools/list. The AI calls the group
// selector to receive a text listing of the grouped tools, then calls individual
// tools by their real names. This reduces the upfront token cost when a server
// has many tools.
type Group struct {
	name        string
	description string
	tools       map[string]*toolDef
	server      *Server
}

// toolDef holds the definition and handler for a registered tool.
type toolDef struct {
	Name            string
	Description     string
	Schema          map[string]any
	Handler         any           // the original user function
	inputType       reflect.Type  // the input struct type
	handlerVal      reflect.Value // reflected handler
	returnStyle     returnStyle
	toolMiddlewares []middleware.Middleware
}

// resourceDef holds the definition and handler for a registered resource.
type resourceDef struct {
	URI         string
	Name        string
	Description string
	MimeType    string
	Handler     any
	inputType   reflect.Type
	handlerVal  reflect.Value
}

// promptDef holds the definition and handler for a registered prompt.
type promptDef struct {
	Name        string
	Description string
	Handler     any
	inputType   reflect.Type
	handlerVal  reflect.Value
	arguments   []promptArgument
}

type promptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// returnStyle describes what the handler returns.
type returnStyle int

const (
	returnString  returnStyle = iota // func(ctx, T) (string, error)
	returnContent                    // func(ctx, T) ([]Content, error)
	returnResult                     // func(ctx, T) (Result, error)
)

// New creates a new mcpx Server with the given name and version, applying any
// provided Option values. If no logger option is supplied, the server writes
// structured log output to stderr using slog.
//
// Example:
//
//	s := hamr.New("my-server", "1.0.0",
//	    hamr.WithDescription("Does useful things"),
//	    hamr.WithLogger(myLogger),
//	)
func New(name, version string, opts ...Option) *Server {
	cfg := serverConfig{
		transport: "stdio",
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	logger := cfg.logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}))
	}

	return &Server{
		name:       name,
		version:    version,
		config:     cfg,
		tools:      make(map[string]*toolDef),
		resources:  make(map[string]*resourceDef),
		prompts:    make(map[string]*promptDef),
		toolGroups: make(map[string]*Group),
		logger:     logger,
	}
}

// Tool registers a tool with the server. The handler must be a function with
// one of these signatures, where T is any struct:
//
//	func(context.Context, T) (string, error)
//	func(context.Context, T) ([]Content, error)
//	func(context.Context, T) (Result, error)
//
// The JSON Schema for the tool's input is derived automatically from T's
// exported fields and their struct tags (see the schema package for supported
// tags). Optional per-tool middleware can be supplied; it runs inside the
// global middleware applied via Use.
//
// Tool panics if the handler signature is invalid, the name is empty, the
// description is empty, or the name has already been registered.
func (s *Server) Tool(name, description string, handler any, mws ...middleware.Middleware) *Server {
	td := s.buildToolDef(name, description, handler)
	td.toolMiddlewares = mws

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.tools[name]; exists {
		panic(fmt.Sprintf("mcpx: tool %q already registered", name))
	}
	s.tools[name] = td
	return s
}

// ToolGroup creates a named group of related tools. fn is called immediately
// with a *Group so that tools can be registered on it via Group.Tool. The group
// appears in tools/list as a single selector entry whose name is "group__<name>".
// Calling that selector returns a text listing of the tools inside the group
// without exposing full schemas upfront.
//
// ToolGroup panics if name conflicts with any standalone tool name or with
// another group name.
func (s *Server) ToolGroup(name, description string, fn func(g *Group)) *Server {
	g := &Group{
		name:        name,
		description: description,
		tools:       make(map[string]*toolDef),
		server:      s,
	}
	fn(g)

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.tools[name]; exists {
		panic(fmt.Sprintf("hamr: group name %q conflicts with an existing tool", name))
	}
	if _, exists := s.toolGroups[name]; exists {
		panic(fmt.Sprintf("hamr: tool group %q already registered", name))
	}
	s.toolGroups[name] = g
	return s
}

// Tool registers a tool inside this Group. It accepts the same handler
// signatures as Server.Tool and the same optional per-tool middleware. Tool
// panics if the handler is invalid, the name is empty or already taken
// (anywhere on the server — standalone tools, other groups, or within this
// group), or if the description is empty.
func (g *Group) Tool(name, description string, handler any, mws ...middleware.Middleware) {
	td := g.server.buildToolDef(name, description, handler)
	td.toolMiddlewares = mws

	g.server.mu.RLock()
	_, standaloneExists := g.server.tools[name]
	g.server.mu.RUnlock()

	if standaloneExists {
		panic(fmt.Sprintf("hamr: group tool %q conflicts with an existing standalone tool", name))
	}

	// Check other groups for the same name.
	g.server.mu.RLock()
	for gName, og := range g.server.toolGroups {
		if _, exists := og.tools[name]; exists {
			g.server.mu.RUnlock()
			panic(fmt.Sprintf("hamr: group tool %q conflicts with a tool in group %q", name, gName))
		}
	}
	g.server.mu.RUnlock()

	if _, exists := g.tools[name]; exists {
		panic(fmt.Sprintf("hamr: tool %q already registered in group %q", name, g.name))
	}

	g.tools[name] = td
}

// Resource registers a resource with the server. The uri uniquely identifies
// the resource in the MCP resource list; name is a short human-readable label
// and description provides additional context. The handler is invoked when a
// client requests the resource. Resource panics if the URI is already
// registered or the handler is not a function.
func (s *Server) Resource(uri, name, description string, handler any) *Server {
	rd := s.buildResourceDef(uri, name, description, handler)

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.resources[uri]; exists {
		panic(fmt.Sprintf("mcpx: resource %q already registered", uri))
	}
	s.resources[uri] = rd
	return s
}

// Prompt registers a prompt with the server. Prompts are reusable message
// templates that MCP clients can list and invoke. Prompt panics if the name
// is empty, the name is already registered, or the handler is not a function.
func (s *Server) Prompt(name, description string, handler any) *Server {
	pd := s.buildPromptDef(name, description, handler)

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.prompts[name]; exists {
		panic(fmt.Sprintf("mcpx: prompt %q already registered", name))
	}
	s.prompts[name] = pd
	return s
}

// Use adds one or more global middleware to the server. Global middleware
// wraps every tool call; the first middleware in the list is the outermost
// wrapper. Per-tool middleware registered via Tool(..., mw) runs inside the
// global middleware. Use returns the server for method chaining.
func (s *Server) Use(mws ...middleware.Middleware) *Server {
	s.middlewares = append(s.middlewares, mws...)
	return s
}

// AddTools registers all tools provided by a ToolCollection in a single call.
// This is the idiomatic way to use pre-built toolbox packages:
//
//	s.AddTools(toolbox.FileSystem("/safe/path"))
func (s *Server) AddTools(collection ToolCollection) *Server {
	for _, t := range collection.Tools() {
		s.Tool(t.Name, t.Description, t.Handler)
	}
	return s
}

// ToolCollection is the interface implemented by toolbox packages that group
// related tools together. Implement Tools() to return the set of tools that
// should be registered when AddTools is called.
type ToolCollection interface {
	Tools() []ToolInfo
}

// ToolInfo describes a single tool within a ToolCollection. Name and
// Description are passed directly to Server.Tool; Handler must satisfy the
// same signature constraints documented on that method.
type ToolInfo struct {
	Name        string
	Description string
	Handler     any
}

// OnCall sets a callback that fires after every tool invocation completes.
// The callback receives the tool name, the decoded argument map, the wall-clock
// duration of the call, and any error returned by the handler (nil on success).
// OnCall is used internally by RunSSEWithDashboard to feed the TUI; it is also
// useful for custom metrics or logging.
func (s *Server) OnCall(fn func(tool string, args map[string]any, duration time.Duration, err error)) *Server {
	s.onCall = fn
	return s
}

// Run starts the server using the stdio transport. It reads newline-delimited
// JSON-RPC messages from stdin and writes responses to stdout. This is the
// standard transport used by Claude Desktop and the MCP CLI. Run blocks until
// stdin is closed or the process receives a termination signal.
func (s *Server) Run() error {
	return s.runStdio()
}

// RunSSE starts the server using the SSE (Server-Sent Events) HTTP transport
// on the given TCP address (e.g. ":8080"). MCP clients connect over HTTP and
// receive server messages as SSE events. RunSSE blocks until the server is
// shut down.
func (s *Server) RunSSE(addr string) error {
	return s.runSSE(addr)
}

// RunSSEWithDashboard starts the server with SSE transport on addr and
// simultaneously launches the TUI dashboard in the terminal. The dashboard
// displays a live, scrollable table of every tool call including timing and
// error status. This is the recommended mode during local development.
func (s *Server) RunSSEWithDashboard(addr string) error {
	return s.runSSEWithDashboard(addr)
}

// buildToolDef validates the handler signature and builds the tool definition.
func (s *Server) buildToolDef(name, description string, handler any) *toolDef {
	if name == "" {
		panic("mcpx: tool name cannot be empty")
	}
	if description == "" {
		panic(fmt.Sprintf("mcpx: tool %q must have a description", name))
	}
	if handler == nil {
		panic(fmt.Sprintf("mcpx: tool %q handler cannot be nil", name))
	}

	hType := reflect.TypeOf(handler)
	if hType.Kind() != reflect.Func {
		panic(fmt.Sprintf("mcpx: tool %q handler must be a function, got %s", name, hType.Kind()))
	}

	// Validate: must have 2 params (context.Context, T)
	if hType.NumIn() != 2 {
		panic(fmt.Sprintf("mcpx: tool %q handler must have exactly 2 parameters (context.Context, T), got %d", name, hType.NumIn()))
	}

	// First param must be context.Context
	ctxType := reflect.TypeOf((*context.Context)(nil)).Elem()
	if !hType.In(0).Implements(ctxType) {
		panic(fmt.Sprintf("mcpx: tool %q handler first parameter must be context.Context", name))
	}

	// Second param must be a struct
	inputType := hType.In(1)
	if inputType.Kind() == reflect.Ptr {
		inputType = inputType.Elem()
	}
	if inputType.Kind() != reflect.Struct {
		panic(fmt.Sprintf("mcpx: tool %q handler second parameter must be a struct, got %s", name, inputType.Kind()))
	}

	// Validate return: must have 2 returns (T, error)
	if hType.NumOut() != 2 {
		panic(fmt.Sprintf("mcpx: tool %q handler must return exactly 2 values, got %d", name, hType.NumOut()))
	}

	// Second return must be error
	errType := reflect.TypeOf((*error)(nil)).Elem()
	if !hType.Out(1).Implements(errType) {
		panic(fmt.Sprintf("mcpx: tool %q handler second return must be error", name))
	}

	// Determine return style
	var rs returnStyle
	switch hType.Out(0) {
	case reflect.TypeOf(""):
		rs = returnString
	case reflect.TypeOf([]Content{}):
		rs = returnContent
	case reflect.TypeOf(Result{}):
		rs = returnResult
	default:
		panic(fmt.Sprintf("mcpx: tool %q handler first return must be string, []Content, or Result, got %s", name, hType.Out(0)))
	}

	// Generate schema from input type
	inputSchema := schema.GenerateFromType(inputType)

	return &toolDef{
		Name:        name,
		Description: description,
		Schema:      inputSchema,
		Handler:     handler,
		inputType:   inputType,
		handlerVal:  reflect.ValueOf(handler),
		returnStyle: rs,
	}
}

// buildResourceDef validates and builds a resource definition.
func (s *Server) buildResourceDef(uri, name, description string, handler any) *resourceDef {
	if uri == "" {
		panic("mcpx: resource URI cannot be empty")
	}

	hType := reflect.TypeOf(handler)
	if hType.Kind() != reflect.Func {
		panic(fmt.Sprintf("mcpx: resource %q handler must be a function", uri))
	}

	return &resourceDef{
		URI:         uri,
		Name:        name,
		Description: description,
		Handler:     handler,
		handlerVal:  reflect.ValueOf(handler),
	}
}

// buildPromptDef validates and builds a prompt definition.
func (s *Server) buildPromptDef(name, description string, handler any) *promptDef {
	if name == "" {
		panic("mcpx: prompt name cannot be empty")
	}

	hType := reflect.TypeOf(handler)
	if hType.Kind() != reflect.Func {
		panic(fmt.Sprintf("mcpx: prompt %q handler must be a function", name))
	}

	return &promptDef{
		Name:        name,
		Description: description,
		Handler:     handler,
		handlerVal:  reflect.ValueOf(handler),
	}
}

// invokeTool calls a registered tool with the given JSON arguments.
func (s *Server) invokeTool(ctx context.Context, name string, rawArgs json.RawMessage) (Result, error) {
	s.mu.RLock()
	td, ok := s.tools[name]
	if !ok {
		for _, g := range s.toolGroups {
			if td2, found := g.tools[name]; found {
				td = td2
				ok = true
				break
			}
		}
	}
	s.mu.RUnlock()

	if !ok {
		return ErrorResult(fmt.Sprintf("unknown tool: %s", name)), fmt.Errorf("unknown tool: %s", name)
	}

	// Parse raw JSON into map
	var args map[string]any
	if len(rawArgs) > 0 {
		if err := json.Unmarshal(rawArgs, &args); err != nil {
			return ErrorResult(fmt.Sprintf("invalid JSON arguments: %v", err)), err
		}
	}
	if args == nil {
		args = make(map[string]any)
	}

	// Apply defaults
	args = validate.ApplyDefaults(td.Schema, args)

	// Validate
	if errs := validate.Validate(td.Schema, args); len(errs) > 0 {
		return ErrorResult(errs.Error()), &ValidationError{
			Tool:   name,
			Errors: validationErrorStrings(errs),
		}
	}

	// Build the middleware chain
	handler := func(ctx context.Context, toolName string, a map[string]any) (any, error) {
		return s.callToolHandler(ctx, td, a)
	}

	// Apply tool-specific middleware (innermost)
	for i := len(td.toolMiddlewares) - 1; i >= 0; i-- {
		handler = td.toolMiddlewares[i](handler)
	}

	// Apply global middleware (outermost)
	for i := len(s.middlewares) - 1; i >= 0; i-- {
		handler = s.middlewares[i](handler)
	}

	// Call through middleware chain
	start := time.Now()
	result, err := handler(ctx, name, args)
	duration := time.Since(start)

	// Fire the onCall callback if set
	if s.onCall != nil {
		s.onCall(name, args, duration, err)
	}

	if err != nil {
		return ErrorResult(err.Error()), err
	}

	// Convert result
	switch v := result.(type) {
	case Result:
		return v, nil
	case string:
		return NewResult(TextContent(v)), nil
	case []Content:
		return NewResult(v...), nil
	default:
		return ErrorResult("unexpected handler return type"), fmt.Errorf("unexpected handler return type: %T", result)
	}
}

// callToolHandler does the actual handler invocation via reflection.
func (s *Server) callToolHandler(ctx context.Context, td *toolDef, args map[string]any) (any, error) {
	// Marshal args back to JSON, then unmarshal into the typed struct
	jsonBytes, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal args: %w", err)
	}

	inputPtr := reflect.New(td.inputType)
	if err := json.Unmarshal(jsonBytes, inputPtr.Interface()); err != nil {
		return nil, fmt.Errorf("failed to unmarshal into %s: %w", td.inputType.Name(), err)
	}

	// Call the handler
	results := td.handlerVal.Call([]reflect.Value{
		reflect.ValueOf(ctx),
		inputPtr.Elem(),
	})

	// Extract error
	var retErr error
	if !results[1].IsNil() {
		retErr = results[1].Interface().(error)
	}

	if retErr != nil {
		return nil, &ToolError{
			Tool:  td.Name,
			Cause: retErr,
		}
	}

	return results[0].Interface(), nil
}

func validationErrorStrings(errs validate.ValidationErrors) []string {
	out := make([]string, len(errs))
	for i, e := range errs {
		out[i] = e.Error()
	}
	return out
}

// ListTools returns a snapshot of all registered tool definitions. Each entry
// contains the tool name, description, and JSON Schema map. The slice order is
// not guaranteed. This method is safe for concurrent use.
func (s *Server) ListTools() []toolDef {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tools := make([]toolDef, 0, len(s.tools))
	for _, td := range s.tools {
		tools = append(tools, *td)
	}
	return tools
}

// EstimateSchemaTokens returns a rough token estimate for each tool entry (and
// group selector) that would appear in a tools/list response. The estimate is
// computed by marshalling each entry to JSON and dividing the byte length by 4
// (the standard tokens-per-byte heuristic for English/code text).
//
// The returned map keys are tool names (or "group__<name>" for group selectors).
func (s *Server) EstimateSchemaTokens() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make(map[string]int, len(s.tools)+len(s.toolGroups))

	for _, td := range s.tools {
		entry := map[string]any{
			"name":        td.Name,
			"description": td.Description,
			"inputSchema": td.Schema,
		}
		b, _ := json.Marshal(entry)
		out[td.Name] = len(b) / 4
	}

	for _, g := range s.toolGroups {
		entry := map[string]any{
			"name":        "group__" + g.name,
			"description": g.description + " (call this to see available operations)",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
		}
		b, _ := json.Marshal(entry)
		out["group__"+g.name] = len(b) / 4
	}

	return out
}
