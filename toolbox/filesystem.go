// Package toolbox provides pre-built tool collections for mcpx servers.
// Each collection groups related tools that can be registered with a single
// call to s.AddTools(toolbox.FileSystem("/safe/path")).
package toolbox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AKhilRaghav0/hamr"
)

// containsNullByte reports whether s contains a null byte (U+0000).
// Null bytes in paths are used in injection attacks and are never valid on any
// supported OS.
func containsNullByte(s string) bool {
	return strings.ContainsRune(s, 0)
}

// FileSystemTools is a sandboxed collection of file-system tools scoped to a
// single root directory. All path arguments are validated to ensure they
// remain within the root; path traversal attempts are rejected with an error.
type FileSystemTools struct {
	root string
}

// FileSystem returns a FileSystemTools collection rooted at root. All tool
// operations are confined to that directory tree.
func FileSystem(root string) *FileSystemTools {
	abs, err := filepath.Abs(root)
	if err != nil {
		// If Abs fails the OS is in a very bad state; propagate via panic so
		// the problem is surfaced at server startup, not silently at call time.
		panic(fmt.Sprintf("toolbox/filesystem: cannot resolve root %q: %v", root, err))
	}
	// Resolve the root itself through any symlinks so that the containment
	// check in safePath compares real paths on both sides of the equation.
	// On macOS, for instance, os.MkdirTemp returns a path under /var which is
	// a symlink to /private/var; without resolving the root here the
	// post-EvalSymlinks check would incorrectly report a sandbox escape.
	realRoot, err := filepath.EvalSymlinks(abs)
	if err != nil {
		panic(fmt.Sprintf("toolbox/filesystem: cannot resolve root symlinks %q: %v", abs, err))
	}
	return &FileSystemTools{root: realRoot}
}

// Tools implements mcpx.ToolCollection.
func (f *FileSystemTools) Tools() []mcpx.ToolInfo {
	return []mcpx.ToolInfo{
		{
			Name:        "read_file",
			Description: "Read the contents of a file within the sandbox. Returns the raw text content.",
			Handler:     f.readFile,
		},
		{
			Name:        "write_file",
			Description: "Write text content to a file within the sandbox. Creates intermediate directories if needed.",
			Handler:     f.writeFile,
		},
		{
			Name:        "list_dir",
			Description: "List the contents of a directory within the sandbox. Returns each entry with its type and size.",
			Handler:     f.listDir,
		},
		{
			Name:        "search_files",
			Description: "Search for files matching a glob pattern within the sandbox. Returns all matching paths.",
			Handler:     f.searchFiles,
		},
	}
}

// safePath resolves userPath relative to the sandbox root and verifies the
// result stays inside it. It returns the absolute path or an error.
//
// Security measures applied in order:
//  1. Null-byte rejection — null bytes are never valid in file paths and are a
//     classic injection vector.
//  2. Lexical containment check — filepath.Join + filepath.Abs + filepath.Rel
//     rejects classic "../.." traversal before any I/O.
//  3. Symlink resolution — filepath.EvalSymlinks follows every symlink in the
//     resolved path, then the containment check is repeated on the real path.
//     This prevents an attacker from placing a symlink inside the sandbox that
//     points to a location outside it.
func (f *FileSystemTools) safePath(userPath string) (string, error) {
	// 1. Null-byte injection guard.
	if containsNullByte(userPath) {
		return "", fmt.Errorf("path contains null byte")
	}

	// 2. Lexical containment — catches ".." traversal without any I/O.
	joined := filepath.Join(f.root, userPath)
	abs, err := filepath.Abs(joined)
	if err != nil {
		return "", fmt.Errorf("cannot resolve path: %w", err)
	}
	if err := f.assertUnderRoot(abs, userPath); err != nil {
		return "", err
	}

	// 3. Symlink resolution — resolves all symlinks in the path and re-checks
	// containment, closing the symlink-escape attack vector.
	//
	// EvalSymlinks fails when the path does not yet exist (e.g. write_file with
	// a new filename). In that case we resolve the nearest existing ancestor and
	// check that it lies within the sandbox — then the final path, which is a
	// child of that ancestor, must also be inside.
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		real, err = f.evalSymlinksPartial(abs)
		if err != nil {
			// Path component genuinely cannot be resolved; treat as invalid.
			return "", fmt.Errorf("cannot resolve path components: %w", err)
		}
	}
	if err := f.assertUnderRoot(real, userPath); err != nil {
		return "", err
	}

	// Return the original (non-resolved) path so callers receive a predictable
	// path that matches the sandbox layout visible to the user.
	return abs, nil
}

// assertUnderRoot checks that candidate (an absolute path) is contained within
// f.root using filepath.Rel, which avoids any OS calls.
func (f *FileSystemTools) assertUnderRoot(candidate, userPath string) error {
	rel, err := filepath.Rel(f.root, candidate)
	if err != nil {
		return fmt.Errorf("cannot compute relative path: %w", err)
	}
	if strings.HasPrefix(rel, "..") {
		return fmt.Errorf("path %q escapes the sandbox root", userPath)
	}
	return nil
}

// evalSymlinksPartial walks up the path until it finds the deepest existing
// ancestor, resolves symlinks on that prefix, and then appends the remaining
// suffix. This allows the symlink check to work for paths that do not yet exist
// (e.g. the target of a write_file call).
func (f *FileSystemTools) evalSymlinksPartial(path string) (string, error) {
	// Collect path components from leaf to root until we find one that exists.
	suffix := ""
	cur := path
	for {
		resolved, err := filepath.EvalSymlinks(cur)
		if err == nil {
			// cur exists; reconstruct by appending the missing suffix.
			if suffix == "" {
				return resolved, nil
			}
			return filepath.Join(resolved, suffix), nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			// Reached the filesystem root without finding any existing component.
			return "", fmt.Errorf("no existing ancestor found for %q", path)
		}
		// Prepend the current base to the accumulating suffix.
		base := filepath.Base(cur)
		if suffix == "" {
			suffix = base
		} else {
			suffix = filepath.Join(base, suffix)
		}
		cur = parent
	}
}

// ---- input structs ----

// ReadFileInput is the input for the read_file tool.
type ReadFileInput struct {
	Path string `json:"path" desc:"path to the file, relative to the sandbox root"`
}

// WriteFileInput is the input for the write_file tool.
type WriteFileInput struct {
	Path    string `json:"path" desc:"path to the file, relative to the sandbox root"`
	Content string `json:"content" desc:"text content to write"`
}

// ListDirInput is the input for the list_dir tool.
type ListDirInput struct {
	Path string `json:"path" desc:"directory path relative to the sandbox root; use '.' for the root itself" default:"."`
}

// SearchFilesInput is the input for the search_files tool.
type SearchFilesInput struct {
	Pattern string `json:"pattern" desc:"glob pattern for file names, e.g. '*.go' or '**/*.json'"`
	Path    string `json:"path" desc:"directory to search in, relative to sandbox root; defaults to root" optional:"true"`
}

// ---- handlers ----

func (f *FileSystemTools) readFile(_ context.Context, in ReadFileInput) (string, error) {
	abs, err := f.safePath(in.Path)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		return "", fmt.Errorf("read_file: %w", err)
	}
	return string(data), nil
}

func (f *FileSystemTools) writeFile(_ context.Context, in WriteFileInput) (string, error) {
	abs, err := f.safePath(in.Path)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", fmt.Errorf("write_file: create directories: %w", err)
	}
	if err := os.WriteFile(abs, []byte(in.Content), 0o644); err != nil {
		return "", fmt.Errorf("write_file: %w", err)
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(in.Content), in.Path), nil
}

func (f *FileSystemTools) listDir(_ context.Context, in ListDirInput) (string, error) {
	if in.Path == "" {
		in.Path = "."
	}
	abs, err := f.safePath(in.Path)
	if err != nil {
		return "", err
	}

	entries, err := os.ReadDir(abs)
	if err != nil {
		return "", fmt.Errorf("list_dir: %w", err)
	}

	if len(entries) == 0 {
		return fmt.Sprintf("directory %q is empty", in.Path), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Contents of %s:\n", in.Path))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			sb.WriteString(fmt.Sprintf("  %s [error reading info]\n", e.Name()))
			continue
		}
		entryType := "file"
		sizeStr := fmt.Sprintf("%d B", info.Size())
		if e.IsDir() {
			entryType = "dir "
			sizeStr = "-"
		} else if e.Type()&os.ModeSymlink != 0 {
			entryType = "link"
		}
		sb.WriteString(fmt.Sprintf("  [%s] %-40s %s\n", entryType, e.Name(), sizeStr))
	}
	return sb.String(), nil
}

func (f *FileSystemTools) searchFiles(_ context.Context, in SearchFilesInput) (string, error) {
	searchRoot := in.Path
	if searchRoot == "" {
		searchRoot = "."
	}
	absRoot, err := f.safePath(searchRoot)
	if err != nil {
		return "", err
	}

	// Build the glob pattern rooted at the search directory.
	globPattern := filepath.Join(absRoot, in.Pattern)

	matches, err := filepath.Glob(globPattern)
	if err != nil {
		return "", fmt.Errorf("search_files: invalid pattern %q: %w", in.Pattern, err)
	}

	if len(matches) == 0 {
		return fmt.Sprintf("no files matching %q found under %s", in.Pattern, searchRoot), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Files matching %q:\n", in.Pattern))
	for _, m := range matches {
		// Verify each result (belt-and-suspenders).
		rel, err := filepath.Rel(f.root, m)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		sb.WriteString(fmt.Sprintf("  %s\n", rel))
	}
	return sb.String(), nil
}
