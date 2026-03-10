package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/spf13/cobra"
)

var (
	initDescription string
	initTransport   string
)

var initCmd = &cobra.Command{
	Use:   "init <name>",
	Short: "Scaffold a new hamr MCP server project",
	Long: `Create a new MCP server project with a complete, ready-to-run directory structure.

The generated project includes:
  - main.go          entry point that wires up the server
  - tools/example.go an example tool with a typed input struct
  - go.mod           module file with hamr dependency
  - Makefile         build/run/dev targets
  - README.md        project documentation
  - claude.json      Claude Desktop configuration snippet
  - .gitignore       standard Go gitignore`,
	Args: cobra.ExactArgs(1),
	RunE: runInit,
}

func init() {
	initCmd.Flags().StringVarP(&initDescription, "description", "d", "An MCP server built with hamr", "Short description of the MCP server")
	initCmd.Flags().StringVarP(&initTransport, "transport", "t", "stdio", `Transport type: "stdio" or "sse"`)
}

// projectData is the template context passed to every generated file.
type projectData struct {
	Name        string // e.g. "my-server"
	PackageName string // e.g. "myserver"  (valid Go identifier)
	Description string
	Transport   string
	Module      string // Go module path, e.g. "github.com/you/my-server"
}

func runInit(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Validate transport flag.
	if initTransport != "stdio" && initTransport != "sse" {
		return fmt.Errorf("invalid transport %q: must be \"stdio\" or \"sse\"", initTransport)
	}

	// Build template context.
	data := projectData{
		Name:        name,
		PackageName: toPackageName(name),
		Description: initDescription,
		Transport:   initTransport,
		Module:      "github.com/you/" + name,
	}

	// Refuse to overwrite an existing directory.
	if _, err := os.Stat(name); err == nil {
		return fmt.Errorf("directory %q already exists", name)
	}

	// Create directory tree.
	dirs := []string{
		name,
		filepath.Join(name, "tools"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", d, err)
		}
	}

	// Generate each file from its template.
	files := []struct {
		path string
		tmpl string
	}{
		{filepath.Join(name, "main.go"), mainGoTmpl},
		{filepath.Join(name, "tools", "example.go"), exampleToolTmpl},
		{filepath.Join(name, "go.mod"), goModTmpl},
		{filepath.Join(name, "Makefile"), makefileTmpl},
		{filepath.Join(name, "README.md"), readmeTmpl},
		{filepath.Join(name, "claude.json"), claudeJSONTmpl},
		{filepath.Join(name, ".gitignore"), gitignoreTmpl},
	}

	for _, f := range files {
		if err := writeTemplate(f.path, f.tmpl, data); err != nil {
			return err
		}
	}

	printSuccess(name, data)
	return nil
}

// writeTemplate renders tmplSrc with data and writes it to path.
func writeTemplate(path, tmplSrc string, data projectData) error {
	t, err := template.New(filepath.Base(path)).Parse(tmplSrc)
	if err != nil {
		return fmt.Errorf("parse template for %s: %w", path, err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create file %s: %w", path, err)
	}
	defer f.Close()

	if err := t.Execute(f, data); err != nil {
		return fmt.Errorf("render template for %s: %w", path, err)
	}
	return nil
}

// toPackageName converts a project name (possibly hyphenated) to a valid Go
// package identifier by stripping hyphens, underscores, dots and any other
// non-letter/non-digit characters, then lowercasing. If the result starts with
// a digit or is empty, it is prefixed with "pkg" to keep it a valid identifier.
func toPackageName(name string) string {
	pkg := strings.ToLower(name)
	// Keep only ASCII letters and digits.
	var b strings.Builder
	for _, r := range pkg {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	pkg = b.String()
	// A valid Go identifier must not be empty or start with a digit.
	if pkg == "" {
		return "main"
	}
	if pkg[0] >= '0' && pkg[0] <= '9' {
		pkg = "pkg" + pkg
	}
	return pkg
}

func printSuccess(name string, data projectData) {
	cyan := "\033[36m"
	green := "\033[32m"
	bold := "\033[1m"
	reset := "\033[0m"

	fmt.Printf("\n%s%sProject %q created successfully!%s\n\n", bold, green, name, reset)
	fmt.Printf("%sNext steps:%s\n", bold, reset)
	fmt.Printf("  %s1.%s cd %s\n", cyan, reset, name)
	fmt.Printf("  %s2.%s go mod tidy\n", cyan, reset)
	fmt.Printf("  %s3.%s go run .\n", cyan, reset)
	fmt.Println()
	fmt.Printf("%sDevelopment mode (auto-rebuild on change):%s\n", bold, reset)
	fmt.Printf("  hamr dev\n")
	fmt.Println()
	fmt.Printf("%sClaude Desktop integration:%s\n", bold, reset)
	fmt.Printf("  See %s/claude.json for the config snippet.\n", name)
	fmt.Println()
}

// ---------------------------------------------------------------------------
// File templates
// ---------------------------------------------------------------------------

const mainGoTmpl = `package main

import (
	"log"

	"github.com/AKhilRaghav0/hamr"
	"{{.Module}}/tools"
)

func main() {
	s := hamr.New("{{.Name}}", "0.1.0")

	// Register tools from the tools package.
	s.Tool("echo", "Echo the input text back to the caller", tools.Echo)
	s.Tool("greet", "Return a personalised greeting", tools.Greet)
{{- if eq .Transport "sse"}}

	// SSE transport — clients connect over HTTP.
	if err := s.RunSSE(":8080"); err != nil {
		log.Fatal(err)
	}
{{- else}}

	// Stdio transport (default) — attach to Claude Desktop or any MCP client.
	if err := s.Run(); err != nil {
		log.Fatal(err)
	}
{{- end}}
}
`

const exampleToolTmpl = `// Package tools contains the tool handlers for the {{.Name}} MCP server.
package tools

import (
	"context"
	"fmt"
)

// EchoInput is the input schema for the echo tool.
// Field tags drive JSON schema generation and validation.
type EchoInput struct {
	Text string ` + "`" + `json:"text" desc:"the text to echo back" required:"true"` + "`" + `
}

// Echo returns the input text unchanged.
func Echo(ctx context.Context, input EchoInput) (string, error) {
	return input.Text, nil
}

// GreetInput is the input schema for the greet tool.
type GreetInput struct {
	Name     string ` + "`" + `json:"name"     desc:"the person to greet"      required:"true"` + "`" + `
	Greeting string ` + "`" + `json:"greeting" desc:"greeting word to use"       default:"Hello"` + "`" + `
}

// Greet returns a personalised greeting message.
func Greet(ctx context.Context, input GreetInput) (string, error) {
	return fmt.Sprintf("%s, %s! This server is powered by hamr.", input.Greeting, input.Name), nil
}
`

const goModTmpl = `module {{.Module}}

go 1.22

require github.com/AKhilRaghav0/hamr v0.1.0
`

const makefileTmpl = `.PHONY: build run dev tidy clean

BINARY := {{.Name}}

build:
	go build -o $(BINARY) .

run: build
	./$(BINARY)

dev:
	hamr dev

tidy:
	go mod tidy

clean:
	rm -f $(BINARY)
`

const readmeTmpl = `# {{.Name}}

{{.Description}}

Built with [hamr](https://github.com/AKhilRaghav0/hamr) — a high-level Go framework for MCP servers.

## Tools

| Name  | Description                          |
|-------|--------------------------------------|
| echo  | Echo the input text back to caller   |
| greet | Return a personalised greeting       |

## Running

` + "```" + `sh
go mod tidy
go run .
` + "```" + `

### Development mode (auto-rebuild)

` + "```" + `sh
hamr dev
` + "```" + `

## Claude Desktop

Copy the contents of ` + "`claude.json`" + ` into your Claude Desktop ` + "`mcpServers`" + ` configuration block.
`

const claudeJSONTmpl = `{
  "mcpServers": {
    "{{.Name}}": {
      "command": "{{.Name}}",
      "args": []
    }
  }
}
`

const gitignoreTmpl = `# Compiled binary
{{.Name}}

# Go toolchain artifacts
*.test
*.out
vendor/

# Environment / secrets
.env
*.env

# Editor directories
.idea/
.vscode/
*.swp
`
