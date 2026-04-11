// Example toolbox demonstrates how to use pre-built tool collections from the
// toolbox package. A single call to s.AddTools registers all tools in the
// collection, keeping server setup concise.
//
// This example mounts the FileSystem toolbox, which provides sandboxed file
// operations (read_file, write_file, list_dir, search_files) confined to a
// single directory tree.
//
// Run with: go run ./examples/toolbox
package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/AKhilRaghav0/hamr"
	"github.com/AKhilRaghav0/hamr/toolbox"
)

func main() {
	// Resolve the sandbox directory. The FileSystem toolbox rejects any path
	// that tries to escape this root, so it is safe to expose to an LLM.
	sandboxDir := filepath.Join(os.TempDir(), "mcpx-demo")
	if err := os.MkdirAll(sandboxDir, 0o755); err != nil {
		log.Fatalf("failed to create sandbox directory: %v", err)
	}

	s := hamr.New("file-server", "1.0.0")

	// AddTools registers all tools from a ToolCollection in one call.
	// toolbox.FileSystem provides: read_file, write_file, list_dir, search_files.
	// All paths are sandboxed to sandboxDir — traversal outside is rejected.
	s.AddTools(toolbox.FileSystem(sandboxDir))

	// Multiple collections can be composed on the same server.
	// For example, if a toolbox.HTTP() collection existed:
	//   s.AddTools(toolbox.HTTP())
	// Each collection brings its own set of tools, names must not conflict.

	if err := s.Run(); err != nil {
		log.Fatal(err)
	}
}
