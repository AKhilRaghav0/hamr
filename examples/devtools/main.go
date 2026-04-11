// devtools is a real, practical MCP server built with hamr.
// It provides developer productivity tools for use with Claude Desktop, Cursor, etc.
//
// Install:
//
//	go build -o devtools ./examples/devtools
//	# Add to Claude Desktop config (see below)
//
// Tools provided:
//   - run_command: Execute shell commands
//   - read_file: Read file contents
//   - write_file: Write/create files
//   - list_dir: List directory contents
//   - search_files: Search files by glob pattern
//   - search_code: Grep for patterns in code
//   - git_status: Show git status
//   - git_diff: Show git diff
//   - git_log: Show recent commits
//   - http_fetch: Fetch a URL
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/AKhilRaghav0/hamr"
	"github.com/AKhilRaghav0/hamr/middleware"
)

// --- Tool Input Types ---

type RunCommandInput struct {
	Command string   `json:"command" desc:"the command to run"`
	Args    []string `json:"args" desc:"command arguments" optional:"true"`
	Dir     string   `json:"dir" desc:"working directory (defaults to home)" optional:"true"`
}

type ReadFileInput struct {
	Path string `json:"path" desc:"absolute or relative file path"`
}

type WriteFileInput struct {
	Path    string `json:"path" desc:"file path to write to"`
	Content string `json:"content" desc:"content to write"`
}

type ListDirInput struct {
	Path string `json:"path" desc:"directory path" default:"."`
}

type SearchFilesInput struct {
	Pattern string `json:"pattern" desc:"glob pattern like '*.go' or '**/*.ts'"`
	Dir     string `json:"dir" desc:"directory to search in" default:"."`
}

type SearchCodeInput struct {
	Pattern string `json:"pattern" desc:"regex pattern to search for"`
	Dir     string `json:"dir" desc:"directory to search in" default:"."`
	Glob    string `json:"glob" desc:"file pattern filter like '*.go'" optional:"true"`
}

type GitStatusInput struct {
	Dir string `json:"dir" desc:"repository directory" default:"."`
}

type GitDiffInput struct {
	Dir    string `json:"dir" desc:"repository directory" default:"."`
	Staged bool   `json:"staged" desc:"show staged changes" optional:"true"`
}

type GitLogInput struct {
	Dir   string `json:"dir" desc:"repository directory" default:"."`
	Count int    `json:"count" desc:"number of commits" default:"10"`
}

type HTTPFetchInput struct {
	URL string `json:"url" desc:"URL to fetch"`
}

// --- Tool Handlers ---

func runCommand(_ context.Context, in RunCommandInput) (string, error) {
	if in.Command == "" {
		return "", fmt.Errorf("command is required")
	}
	dir := in.Dir
	if dir == "" {
		dir, _ = os.Getwd()
	}

	args := in.Args
	cmd := exec.Command(in.Command, args...)
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	result := string(out)
	if err != nil {
		result += fmt.Sprintf("\n[exit error: %v]", err)
	}
	if len(result) > 50000 {
		result = result[:50000] + "\n... [truncated]"
	}
	return result, nil
}

func readFile(_ context.Context, in ReadFileInput) (string, error) {
	path := resolvePath(in.Path)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", in.Path, err)
	}
	content := string(data)
	if len(content) > 100000 {
		content = content[:100000] + "\n... [truncated at 100KB]"
	}
	return content, nil
}

func writeFile(_ context.Context, in WriteFileInput) (string, error) {
	path := resolvePath(in.Path)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create dirs: %w", err)
	}
	if err := os.WriteFile(path, []byte(in.Content), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", in.Path, err)
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(in.Content), in.Path), nil
}

func listDir(_ context.Context, in ListDirInput) (string, error) {
	path := resolvePath(in.Path)
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", fmt.Errorf("list %s: %w", in.Path, err)
	}
	var sb strings.Builder
	for _, e := range entries {
		info, _ := e.Info()
		if info != nil {
			if e.IsDir() {
				fmt.Fprintf(&sb, "  [dir]  %s/\n", e.Name())
			} else {
				fmt.Fprintf(&sb, "  %6d  %s\n", info.Size(), e.Name())
			}
		} else {
			fmt.Fprintf(&sb, "         %s\n", e.Name())
		}
	}
	return sb.String(), nil
}

func searchFiles(_ context.Context, in SearchFilesInput) (string, error) {
	dir := resolvePath(in.Dir)
	pattern := filepath.Join(dir, in.Pattern)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", fmt.Errorf("glob: %w", err)
	}
	if len(matches) == 0 {
		return "no files found", nil
	}
	var sb strings.Builder
	for _, m := range matches {
		rel, _ := filepath.Rel(dir, m)
		if rel == "" {
			rel = m
		}
		sb.WriteString(rel + "\n")
	}
	return sb.String(), nil
}

func searchCode(_ context.Context, in SearchCodeInput) (string, error) {
	dir := resolvePath(in.Dir)
	args := []string{"-rn", "--color=never", in.Pattern, dir}
	if in.Glob != "" {
		args = []string{"-rn", "--color=never", "--include=" + in.Glob, in.Pattern, dir}
	}
	cmd := exec.Command("grep", args...)
	out, _ := cmd.CombinedOutput()
	result := string(out)
	if result == "" {
		return "no matches found", nil
	}
	if len(result) > 50000 {
		result = result[:50000] + "\n... [truncated]"
	}
	return result, nil
}

func gitStatus(_ context.Context, in GitStatusInput) (string, error) {
	return runGit(in.Dir, "status")
}

func gitDiff(_ context.Context, in GitDiffInput) (string, error) {
	if in.Staged {
		return runGit(in.Dir, "diff", "--staged")
	}
	return runGit(in.Dir, "diff")
}

func gitLog(_ context.Context, in GitLogInput) (string, error) {
	count := in.Count
	if count <= 0 {
		count = 10
	}
	return runGit(in.Dir, "log", fmt.Sprintf("-n%d", count), "--pretty=format:%h %ad %an | %s", "--date=short")
}

func httpFetch(_ context.Context, in HTTPFetchInput) (string, error) {
	cmd := exec.Command("curl", "-sL", "-m", "10", in.URL)
	out, err := cmd.CombinedOutput()
	result := string(out)
	if err != nil {
		return "", fmt.Errorf("fetch %s: %w\n%s", in.URL, err, result)
	}
	if len(result) > 50000 {
		result = result[:50000] + "\n... [truncated]"
	}
	return result, nil
}

// --- Helpers ---

func resolvePath(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	home, _ := os.UserHomeDir()
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	wd, _ := os.Getwd()
	return filepath.Join(wd, p)
}

func runGit(dir string, args ...string) (string, error) {
	d := resolvePath(dir)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = d
	out, err := cmd.CombinedOutput()
	result := string(out)
	if err != nil {
		return "", fmt.Errorf("git %v: %w\n%s", args, err, result)
	}
	if result == "" {
		return "no output", nil
	}
	return result, nil
}

func main() {
	s := hamr.New("devtools", "1.0.0",
		hamr.WithDescription("Developer productivity tools — files, git, shell, code search"),
	)

	s.Use(
		middleware.Logger(),
		middleware.Recovery(),
	)

	// File operations
	s.Tool("read_file", "Read the contents of a file", readFile)
	s.Tool("write_file", "Write content to a file (creates directories if needed)", writeFile)
	s.Tool("list_dir", "List files and directories", listDir)
	s.Tool("search_files", "Find files matching a glob pattern", searchFiles)
	s.Tool("search_code", "Search for a regex pattern in code files (grep)", searchCode)

	// Shell
	s.Tool("run_command", "Execute a shell command and return output", runCommand)

	// Git
	s.Tool("git_status", "Show git repository status", gitStatus)
	s.Tool("git_diff", "Show git diff (staged or unstaged)", gitDiff)
	s.Tool("git_log", "Show recent git commits", gitLog)

	// HTTP
	s.Tool("http_fetch", "Fetch a URL and return its content", httpFetch)

	// Check for --dashboard flag to run with TUI
	mode := "stdio"
	addr := ":9090"
	for i, arg := range os.Args[1:] {
		if arg == "--dashboard" || arg == "-d" {
			mode = "dashboard"
		}
		if arg == "--sse" {
			mode = "sse"
		}
		if arg == "--addr" && i+1 < len(os.Args[1:]) {
			addr = os.Args[i+2]
		}
	}

	switch mode {
	case "dashboard":
		// SSE + live TUI dashboard
		fmt.Fprintf(os.Stderr, "devtools starting with dashboard on %s (%d tools)\n", addr, len(s.ListTools()))
		if err := s.RunSSEWithDashboard(addr); err != nil {
			log.Fatal(err)
		}
	case "sse":
		fmt.Fprintf(os.Stderr, "devtools starting SSE on %s (%d tools)\n", addr, len(s.ListTools()))
		if err := s.RunSSE(addr); err != nil {
			log.Fatal(err)
		}
	default:
		// stdio (default) — for Claude Desktop
		if err := s.Run(); err != nil {
			log.Fatal(err)
		}
	}
}
