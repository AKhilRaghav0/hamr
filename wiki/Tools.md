# Tools

Tools are the core of any MCP server. A tool is just a function that the AI can call. This page covers everything about defining tools in hamr.

## The basics

A tool in mcpx has three parts:

1. **An input struct** — defines what arguments the tool accepts
2. **A handler function** — contains your actual logic
3. **A registration call** — tells the server about the tool

```go
// 1. Input struct
type SearchInput struct {
    Query string `json:"query" desc:"what to search for"`
}

// 2. Handler function
func Search(ctx context.Context, input SearchInput) (string, error) {
    return "results for: " + input.Query, nil
}

// 3. Registration
s.Tool("search", "Search for things", Search)
```

That's the whole pattern. Everything else on this page is just details about each part.

## Input structs

The input struct is where the magic happens. mcpx uses reflection to inspect your struct and generate a JSON Schema from it. The AI sees this schema and knows what arguments it can pass.

### Field naming

Use the `json` tag to set the field name in the schema. This is what the AI sees.

```go
type Input struct {
    MaxResults int `json:"max_results"`  // AI sees "max_results"
}
```

Fields without a `json` tag use their Go field name. Unexported fields are ignored. Fields tagged `json:"-"` are skipped entirely.

### Descriptions

The `desc` tag sets the description for a field. This is important — the AI reads these to understand what each argument does.

```go
type Input struct {
    Query string `json:"query" desc:"the search query, supports boolean operators"`
}
```

Write descriptions like you're explaining the field to a colleague. Be specific. "search query" is okay. "the search query, supports AND/OR operators and quoted phrases" is better.

### Default values

The `default` tag sets a value that's automatically applied when the field is omitted from the request.

```go
type Input struct {
    Count  int    `json:"count" default:"10"`
    Format string `json:"format" default:"text"`
}
```

Defaults are parsed to the correct Go type. `"10"` becomes `int(10)`, `"true"` becomes `bool(true)`, `"3.14"` becomes `float64(3.14)`.

### Required vs optional

By default, every field is required. The AI must provide it or the request fails validation. If a field is optional, mark it:

```go
type Input struct {
    Query  string `json:"query"`                    // required (default)
    Filter string `json:"filter" optional:"true"`   // optional
}
```

### Enums

Restrict a field to specific values:

```go
type Input struct {
    Format string `json:"format" enum:"json,xml,csv"`
}
```

If the AI passes a value not in the list, validation rejects it with a clear error message.

### Numeric bounds

Set minimum and maximum values for numbers:

```go
type Input struct {
    Page    int `json:"page" min:"1"`
    PerPage int `json:"per_page" min:"1" max:"100" default:"20"`
}
```

### String patterns

Validate strings against a regex:

```go
type Input struct {
    Email string `json:"email" pattern:"^[^@]+@[^@]+\\.[^@]+$"`
}
```

### Nested structs

Structs can contain other structs. The schema is generated recursively.

```go
type Options struct {
    Verbose bool `json:"verbose" default:"false"`
    Timeout int  `json:"timeout" default:"30"`
}

type Input struct {
    Query   string  `json:"query"`
    Options Options `json:"options" optional:"true"`
}
```

### Slices and maps

Both work as you'd expect:

```go
type Input struct {
    Tags    []string          `json:"tags" optional:"true"`
    Headers map[string]string `json:"headers" optional:"true"`
}
```

Slices become JSON arrays. Maps become JSON objects with `additionalProperties`.

### Supported types

| Go type | JSON Schema type |
|---------|-----------------|
| `string` | `string` |
| `int`, `int64`, etc. | `integer` |
| `float32`, `float64` | `number` |
| `bool` | `boolean` |
| `[]T` | `array` with items of type T |
| `map[string]T` | `object` with additionalProperties of type T |
| `*T` | same as T (pointers are unwrapped) |
| `time.Time` | `string` with format `date-time` |
| nested struct | `object` with properties |

## Handler functions

Handlers must follow one of these signatures:

```go
// Return a string
func(context.Context, InputStruct) (string, error)

// Return multiple content blocks
func(context.Context, InputStruct) ([]hamr.Content, error)

// Return a structured result
func(context.Context, InputStruct) (hamr.Result, error)
```

The first form is the most common. The string becomes a text content block in the MCP response.

### Content blocks

When you need to return more than text — like images, or a mix of text and data:

```go
func FetchImage(ctx context.Context, in Input) ([]hamr.Content, error) {
    imageData := fetchAndEncode(in.URL)
    return []hamr.Content{
        hamr.TextContent("Here's the image you requested:"),
        hamr.ImageContent("image/png", imageData),
    }, nil
}
```

### Error handling

Return an error and mcpx turns it into an MCP error response. The AI sees the error message and can react to it.

```go
func ReadFile(ctx context.Context, in Input) (string, error) {
    data, err := os.ReadFile(in.Path)
    if err != nil {
        return "", fmt.Errorf("could not read %s: %w", in.Path, err)
    }
    return string(data), nil
}
```

If you want to return a "soft error" that the AI can see but that doesn't count as a protocol error, use `hamr.ErrorResult`:

```go
func Risky(ctx context.Context, in Input) (hamr.Result, error) {
    result, err := tryThing()
    if err != nil {
        return hamr.ErrorResult("it didn't work: " + err.Error()), nil
    }
    return hamr.NewResult(hamr.TextContent(result)), nil
}
```

## Registration

Register tools on the server:

```go
s := hamr.New("my-server", "1.0.0")
s.Tool("search", "Search for things", Search)
s.Tool("read_file", "Read a file's contents", ReadFile)
```

The first argument is the tool name (what the AI sees). The second is the description. The third is your handler function.

hamr validates the handler signature at registration time, not at runtime. If your function doesn't match one of the accepted signatures, it panics immediately with a clear message telling you what's wrong. This means you catch mistakes during development, not in production.

### Per-tool middleware

You can attach middleware to individual tools:

```go
s.Tool("expensive_api", "Call a slow external API", handler,
    middleware.Cache(5 * time.Minute),
    middleware.Timeout(60 * time.Second),
)
```

See the [Middleware](Middleware) page for details.

### Tool collections

If you have a group of related tools, you can register them all at once using `AddTools` and the toolbox package:

```go
s.AddTools(toolbox.FileSystem("/safe/path"))
```

See the [Toolbox](Toolbox) page for what's available.
