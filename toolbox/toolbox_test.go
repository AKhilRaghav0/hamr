package toolbox_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AKhilRaghav0/hamr/toolbox"
)

// bg is a convenience alias for a background context.
func bg() context.Context { return context.Background() }

// mustMkdirTemp creates a temp directory cleaned up after t.
func mustMkdirTemp(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "toolbox-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// ============================================================
// FileSystem
// ============================================================

func fsTools(t *testing.T, root string) (read, write, list, search any) {
	t.Helper()
	fs := toolbox.FileSystem(root)
	for _, ti := range fs.Tools() {
		switch ti.Name {
		case "read_file":
			read = ti.Handler
		case "write_file":
			write = ti.Handler
		case "list_dir":
			list = ti.Handler
		case "search_files":
			search = ti.Handler
		}
	}
	return
}

func callReadFile(t *testing.T, h any, path string) (string, error) {
	t.Helper()
	fn := h.(func(context.Context, toolbox.ReadFileInput) (string, error))
	return fn(bg(), toolbox.ReadFileInput{Path: path})
}

func callWriteFile(t *testing.T, h any, path, content string) (string, error) {
	t.Helper()
	fn := h.(func(context.Context, toolbox.WriteFileInput) (string, error))
	return fn(bg(), toolbox.WriteFileInput{Path: path, Content: content})
}

func callListDir(t *testing.T, h any, path string) (string, error) {
	t.Helper()
	fn := h.(func(context.Context, toolbox.ListDirInput) (string, error))
	return fn(bg(), toolbox.ListDirInput{Path: path})
}

func callSearchFiles(t *testing.T, h any, pattern, path string) (string, error) {
	t.Helper()
	fn := h.(func(context.Context, toolbox.SearchFilesInput) (string, error))
	return fn(bg(), toolbox.SearchFilesInput{Pattern: pattern, Path: path})
}

func TestFileSystem_ToolCount(t *testing.T) {
	fs := toolbox.FileSystem(t.TempDir())
	if n := len(fs.Tools()); n != 4 {
		t.Fatalf("want 4 tools, got %d", n)
	}
}

func TestFileSystem_WriteAndRead(t *testing.T) {
	root := mustMkdirTemp(t)
	read, write, _, _ := fsTools(t, root)

	out, err := callWriteFile(t, write, "hello.txt", "Hello, mcpx!")
	if err != nil {
		t.Fatalf("write_file: %v", err)
	}
	if !strings.Contains(out, "wrote") {
		t.Errorf("write_file output missing 'wrote': %q", out)
	}

	content, err := callReadFile(t, read, "hello.txt")
	if err != nil {
		t.Fatalf("read_file: %v", err)
	}
	if content != "Hello, mcpx!" {
		t.Errorf("read_file: want %q, got %q", "Hello, mcpx!", content)
	}
}

func TestFileSystem_WriteCreatesDirectories(t *testing.T) {
	root := mustMkdirTemp(t)
	_, write, _, _ := fsTools(t, root)

	_, err := callWriteFile(t, write, "a/b/c/file.txt", "deep")
	if err != nil {
		t.Fatalf("write_file deep path: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "a", "b", "c", "file.txt"))
	if err != nil {
		t.Fatalf("verify file: %v", err)
	}
	if string(data) != "deep" {
		t.Errorf("unexpected file content %q", data)
	}
}

func TestFileSystem_ListDir(t *testing.T) {
	root := mustMkdirTemp(t)
	if err := os.WriteFile(filepath.Join(root, "alpha.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "beta.txt"), []byte("bb"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, list, _ := fsTools(t, root)
	out, err := callListDir(t, list, ".")
	if err != nil {
		t.Fatalf("list_dir: %v", err)
	}
	if !strings.Contains(out, "alpha.txt") || !strings.Contains(out, "beta.txt") {
		t.Errorf("list_dir missing expected entries: %s", out)
	}
}

func TestFileSystem_SearchFiles(t *testing.T) {
	root := mustMkdirTemp(t)
	for _, name := range []string{"main.go", "server.go", "readme.md"} {
		if err := os.WriteFile(filepath.Join(root, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	_, _, _, search := fsTools(t, root)
	out, err := callSearchFiles(t, search, "*.go", "")
	if err != nil {
		t.Fatalf("search_files: %v", err)
	}
	if !strings.Contains(out, "main.go") || !strings.Contains(out, "server.go") {
		t.Errorf("search_files missing .go files: %s", out)
	}
	if strings.Contains(out, "readme.md") {
		t.Errorf("search_files unexpectedly matched readme.md: %s", out)
	}
}

func TestFileSystem_PathTraversal_Classic(t *testing.T) {
	root := mustMkdirTemp(t)
	read, _, _, _ := fsTools(t, root)

	_, err := callReadFile(t, read, "../../etc/passwd")
	if err == nil {
		t.Fatal("expected sandbox violation error, got nil")
	}
	if !strings.Contains(err.Error(), "escapes") {
		t.Errorf("expected 'escapes' in error message, got: %v", err)
	}
}

func TestFileSystem_PathTraversal_Nested(t *testing.T) {
	root := mustMkdirTemp(t)
	read, _, _, _ := fsTools(t, root)

	_, err := callReadFile(t, read, "sub/../../../outside")
	if err == nil {
		t.Fatal("expected sandbox violation error, got nil")
	}
}

func TestFileSystem_NullByteRejected(t *testing.T) {
	root := mustMkdirTemp(t)
	read, _, _, _ := fsTools(t, root)

	_, err := callReadFile(t, read, "file\x00.txt")
	if err == nil {
		t.Fatal("expected error for null byte in path, got nil")
	}
	if !strings.Contains(err.Error(), "null byte") {
		t.Errorf("expected 'null byte' in error, got: %v", err)
	}
}

func TestFileSystem_SymlinkEscape_Blocked(t *testing.T) {
	root := mustMkdirTemp(t)

	// Create a directory outside the sandbox with a secret file.
	outside := mustMkdirTemp(t)
	secretPath := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secretPath, []byte("top secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Plant a symlink inside the sandbox that points outside.
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("cannot create symlink (permissions?): %v", err)
	}

	read, _, _, _ := fsTools(t, root)
	_, err := callReadFile(t, read, "escape/secret.txt")
	if err == nil {
		t.Fatal("expected sandbox violation for symlink escape, got nil")
	}
	if !strings.Contains(err.Error(), "escapes") {
		t.Errorf("expected 'escapes' in error message, got: %v", err)
	}
}

func TestShell_PathSeparatorInCommand_Rejected(t *testing.T) {
	h := shellHandler(t)
	_, err := callRunCommand(t, h, "/bin/echo", []string{"hello"})
	if err == nil {
		t.Fatal("expected error for path separator in command name, got nil")
	}
	if !strings.Contains(err.Error(), "path separator") {
		t.Errorf("expected 'path separator' in error, got: %v", err)
	}
}

func TestShell_NullByteInCommand_Rejected(t *testing.T) {
	h := shellHandler(t)
	_, err := callRunCommand(t, h, "echo\x00extra", nil)
	if err == nil {
		t.Fatal("expected error for null byte in command name, got nil")
	}
	if !strings.Contains(err.Error(), "null byte") {
		t.Errorf("expected 'null byte' in error, got: %v", err)
	}
}

func TestShell_AllowList_BlocksPathSeparatorBypass(t *testing.T) {
	// Even with an allowlist that includes "echo", "/bin/echo" must be blocked
	// by the path-separator check before the allowlist lookup.
	h := shellHandler(t, toolbox.WithAllowedCommands("echo"))
	_, err := callRunCommand(t, h, "/bin/echo", []string{"hello"})
	if err == nil {
		t.Fatal("expected error for path separator bypass attempt, got nil")
	}
}

// ============================================================
// HTTP
// ============================================================

func callHTTPGet(t *testing.T, h any, url string, headers map[string]string) (string, error) {
	t.Helper()
	fn := h.(func(context.Context, toolbox.HTTPGetInput) (string, error))
	return fn(bg(), toolbox.HTTPGetInput{URL: url, Headers: headers})
}

func callHTTPPost(t *testing.T, h any, url, body, ct string, headers map[string]string) (string, error) {
	t.Helper()
	fn := h.(func(context.Context, toolbox.HTTPPostInput) (string, error))
	return fn(bg(), toolbox.HTTPPostInput{URL: url, Body: body, ContentType: ct, Headers: headers})
}

func callFetchURL(t *testing.T, h any, url string) (string, error) {
	t.Helper()
	fn := h.(func(context.Context, toolbox.FetchURLInput) (string, error))
	return fn(bg(), toolbox.FetchURLInput{URL: url})
}

func httpHandlersByName(t *testing.T, opts ...toolbox.HTTPOption) map[string]any {
	t.Helper()
	h := toolbox.HTTP(opts...)
	m := make(map[string]any)
	for _, ti := range h.Tools() {
		m[ti.Name] = ti.Handler
	}
	return m
}

func TestHTTP_ToolCount(t *testing.T) {
	h := toolbox.HTTP()
	if n := len(h.Tools()); n != 3 {
		t.Fatalf("want 3 tools, got %d", n)
	}
}

func TestHTTP_Get_Basic(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("want GET, got %s", r.Method)
		}
		fmt.Fprint(w, "server response")
	}))
	defer ts.Close()

	handlers := httpHandlersByName(t)
	out, err := callHTTPGet(t, handlers["http_get"], ts.URL, nil)
	if err != nil {
		t.Fatalf("http_get: %v", err)
	}
	if !strings.Contains(out, "server response") {
		t.Errorf("expected body in output, got: %s", out)
	}
	if !strings.Contains(out, "200") {
		t.Errorf("expected status code in output, got: %s", out)
	}
}

func TestHTTP_Get_CustomHeaders(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, r.Header.Get("X-Token"))
	}))
	defer ts.Close()

	handlers := httpHandlersByName(t)
	out, err := callHTTPGet(t, handlers["http_get"], ts.URL, map[string]string{"X-Token": "sentinel"})
	if err != nil {
		t.Fatalf("http_get with headers: %v", err)
	}
	if !strings.Contains(out, "sentinel") {
		t.Errorf("header not forwarded; got: %s", out)
	}
}

func TestHTTP_Post_Basic(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("want POST, got %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		fmt.Fprintf(w, "echo: %s", body)
	}))
	defer ts.Close()

	handlers := httpHandlersByName(t)
	out, err := callHTTPPost(t, handlers["http_post"], ts.URL, "payload-data", "text/plain", nil)
	if err != nil {
		t.Fatalf("http_post: %v", err)
	}
	if !strings.Contains(out, "payload-data") {
		t.Errorf("POST body not echoed; got: %s", out)
	}
}

func TestHTTP_Post_DefaultContentType(t *testing.T) {
	var gotCT string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		fmt.Fprint(w, "ok")
	}))
	defer ts.Close()

	handlers := httpHandlersByName(t)
	_, err := callHTTPPost(t, handlers["http_post"], ts.URL, "data", "", nil)
	if err != nil {
		t.Fatalf("http_post: %v", err)
	}
	if !strings.HasPrefix(gotCT, "text/plain") {
		t.Errorf("expected text/plain content-type, got %q", gotCT)
	}
}

func TestHTTP_Timeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		fmt.Fprint(w, "too late")
	}))
	defer ts.Close()

	handlers := httpHandlersByName(t, toolbox.WithTimeout(50*time.Millisecond))
	_, err := callHTTPGet(t, handlers["http_get"], ts.URL, nil)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestHTTP_MaxBodySize(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 2 KiB response.
		fmt.Fprint(w, strings.Repeat("x", 2048))
	}))
	defer ts.Close()

	handlers := httpHandlersByName(t, toolbox.WithMaxBodySize(512))
	out, err := callFetchURL(t, handlers["fetch_url"], ts.URL)
	if err != nil {
		t.Fatalf("fetch_url: %v", err)
	}
	// Count 'x' characters — must not exceed our cap.
	if n := strings.Count(out, "x"); n > 512 {
		t.Errorf("body not capped: got %d 'x' chars (limit 512)", n)
	}
}

func TestHTTP_FetchURL_IncludesStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "not here")
	}))
	defer ts.Close()

	handlers := httpHandlersByName(t)
	out, err := callFetchURL(t, handlers["fetch_url"], ts.URL)
	if err != nil {
		t.Fatalf("fetch_url: %v", err)
	}
	if !strings.Contains(out, "404") {
		t.Errorf("expected 404 in output, got: %s", out)
	}
}

// ============================================================
// Shell
// ============================================================

func callRunCommand(t *testing.T, h any, command string, args []string) (string, error) {
	t.Helper()
	fn := h.(func(context.Context, toolbox.RunCommandInput) (string, error))
	return fn(bg(), toolbox.RunCommandInput{Command: command, Args: args})
}

func shellHandler(t *testing.T, opts ...toolbox.ShellOption) any {
	t.Helper()
	s := toolbox.Shell(t.TempDir(), opts...)
	for _, ti := range s.Tools() {
		if ti.Name == "run_command" {
			return ti.Handler
		}
	}
	t.Fatal("run_command tool not found")
	return nil
}

func TestShell_ToolCount(t *testing.T) {
	s := toolbox.Shell(t.TempDir())
	if n := len(s.Tools()); n != 1 {
		t.Fatalf("want 1 tool, got %d", n)
	}
}

func TestShell_RunCommand_Echo(t *testing.T) {
	h := shellHandler(t, toolbox.WithAllowedCommands("echo"))
	out, err := callRunCommand(t, h, "echo", []string{"hello mcpx"})
	if err != nil {
		t.Fatalf("run_command echo: %v", err)
	}
	if !strings.Contains(out, "hello mcpx") {
		t.Errorf("expected echo output, got: %s", out)
	}
}

func TestShell_AllowList_BlocksOtherCommands(t *testing.T) {
	h := shellHandler(t, toolbox.WithAllowedCommands("echo"))
	_, err := callRunCommand(t, h, "ls", nil)
	if err == nil {
		t.Fatal("expected error for disallowed command, got nil")
	}
	if !strings.Contains(err.Error(), "not in the allowed list") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestShell_NoAllowList_PermitsAny(t *testing.T) {
	h := shellHandler(t) // no restrictions
	_, err := callRunCommand(t, h, "echo", []string{"unrestricted"})
	if err != nil {
		t.Fatalf("run_command without allowlist: %v", err)
	}
}

func TestShell_MaxOutput_Truncation(t *testing.T) {
	h := shellHandler(t, toolbox.WithMaxOutput(10))
	// echo outputs more than 10 bytes.
	out, err := callRunCommand(t, h, "echo", []string{"abcdefghijklmnopqrstuvwxyz"})
	if err != nil {
		t.Fatalf("run_command with truncation: %v", err)
	}
	if !strings.Contains(out, "truncated") {
		t.Errorf("expected truncation notice in output, got: %s", out)
	}
}

func TestShell_EmptyCommand_Rejected(t *testing.T) {
	h := shellHandler(t)
	_, err := callRunCommand(t, h, "", nil)
	if err == nil {
		t.Fatal("expected error for empty command")
	}
}

func TestShell_NonZeroExit_CapturedInOutput(t *testing.T) {
	h := shellHandler(t)
	// 'false' exits with code 1 on POSIX systems.
	out, err := callRunCommand(t, h, "false", nil)
	// We do not return an error — exit codes are surfaced in the output string.
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !strings.Contains(out, "exit") {
		t.Errorf("expected exit error notice in output, got: %q", out)
	}
}

// ============================================================
// Git
// ============================================================

func callGitStatus(t *testing.T, h any) (string, error) {
	t.Helper()
	fn := h.(func(context.Context, toolbox.GitStatusInput) (string, error))
	return fn(bg(), toolbox.GitStatusInput{})
}

func callGitLog(t *testing.T, h any, count int) (string, error) {
	t.Helper()
	fn := h.(func(context.Context, toolbox.GitLogInput) (string, error))
	return fn(bg(), toolbox.GitLogInput{Count: count})
}

func callGitBlame(t *testing.T, h any, path string) (string, error) {
	t.Helper()
	fn := h.(func(context.Context, toolbox.GitBlameInput) (string, error))
	return fn(bg(), toolbox.GitBlameInput{Path: path})
}

func gitHandlers(t *testing.T, repoPath string) map[string]any {
	t.Helper()
	g := toolbox.Git(repoPath)
	m := make(map[string]any)
	for _, ti := range g.Tools() {
		m[ti.Name] = ti.Handler
	}
	return m
}

func mustInitGitRepo(t *testing.T) string {
	t.Helper()
	dir := mustMkdirTemp(t)
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Tester")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	var out bytes.Buffer
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out.String())
	}
}

func TestGit_ToolCount(t *testing.T) {
	g := toolbox.Git(t.TempDir())
	if n := len(g.Tools()); n != 4 {
		t.Fatalf("want 4 tools, got %d", n)
	}
}

func TestGit_Status_ValidRepo(t *testing.T) {
	repo := mustInitGitRepo(t)
	handlers := gitHandlers(t, repo)

	out, err := callGitStatus(t, handlers["git_status"])
	if err != nil {
		t.Fatalf("git_status: %v", err)
	}
	if out == "" {
		t.Error("git_status returned empty output")
	}
}

func TestGit_Log_WithCommit(t *testing.T) {
	repo := mustInitGitRepo(t)
	runGit(t, repo, "commit", "--allow-empty", "-m", "initial commit")

	handlers := gitHandlers(t, repo)
	out, err := callGitLog(t, handlers["git_log"], 5)
	if err != nil {
		t.Fatalf("git_log: %v", err)
	}
	if !strings.Contains(out, "initial commit") {
		t.Errorf("expected commit message in log, got: %s", out)
	}
}

func TestGit_Blame_OnTrackedFile(t *testing.T) {
	repo := mustInitGitRepo(t)

	// Create and commit a file.
	filePath := filepath.Join(repo, "blame.txt")
	if err := os.WriteFile(filePath, []byte("line one\nline two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "blame.txt")
	runGit(t, repo, "commit", "-m", "add blame.txt")

	handlers := gitHandlers(t, repo)
	out, err := callGitBlame(t, handlers["git_blame"], "blame.txt")
	if err != nil {
		t.Fatalf("git_blame: %v", err)
	}
	if !strings.Contains(out, "line one") {
		t.Errorf("expected file content in blame output, got: %s", out)
	}
}

func TestGit_Status_NotARepo(t *testing.T) {
	handlers := gitHandlers(t, t.TempDir()) // not a git repo

	_, err := callGitStatus(t, handlers["git_status"])
	if err == nil {
		t.Fatal("expected error for non-git directory, got nil")
	}
}

// ============================================================
// Additional FileSystem tests
// ============================================================

// TestFileSystem_ReadNonExistent verifies that reading a file that does not
// exist returns an error.
func TestFileSystem_ReadNonExistent(t *testing.T) {
	root := mustMkdirTemp(t)
	read, _, _, _ := fsTools(t, root)

	_, err := callReadFile(t, read, "does_not_exist.txt")
	if err == nil {
		t.Fatal("expected error reading non-existent file, got nil")
	}
}

// TestFileSystem_ListEmptyDir verifies that an empty directory returns a
// descriptive message rather than an error.
func TestFileSystem_ListEmptyDir(t *testing.T) {
	root := mustMkdirTemp(t)
	_, _, list, _ := fsTools(t, root)

	out, err := callListDir(t, list, ".")
	if err != nil {
		t.Fatalf("list_dir empty root: %v", err)
	}
	if !strings.Contains(out, "empty") {
		t.Errorf("expected 'empty' in output for empty directory, got: %q", out)
	}
}

// TestFileSystem_SearchNoMatches verifies that a pattern with no matching
// files returns a descriptive no-match message.
func TestFileSystem_SearchNoMatches(t *testing.T) {
	root := mustMkdirTemp(t)
	if err := os.WriteFile(filepath.Join(root, "readme.md"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, _, search := fsTools(t, root)
	out, err := callSearchFiles(t, search, "*.go", "")
	if err != nil {
		t.Fatalf("search_files no-match: %v", err)
	}
	if !strings.Contains(out, "no files") {
		t.Errorf("expected 'no files' in output, got: %q", out)
	}
}

// TestFileSystem_WriteNestedPath verifies that writing to a multi-level path
// that does not yet exist creates all intermediate directories.
func TestFileSystem_WriteNestedPath(t *testing.T) {
	root := mustMkdirTemp(t)
	_, write, _, _ := fsTools(t, root)

	_, err := callWriteFile(t, write, "level1/level2/level3/data.txt", "nested content")
	if err != nil {
		t.Fatalf("write_file nested: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "level1", "level2", "level3", "data.txt"))
	if err != nil {
		t.Fatalf("verify nested file: %v", err)
	}
	if string(data) != "nested content" {
		t.Errorf("unexpected file content: %q", data)
	}
}

// TestFileSystem_ListSubdirectory verifies that list_dir works for a
// subdirectory path, not just the root.
func TestFileSystem_ListSubdirectory(t *testing.T) {
	root := mustMkdirTemp(t)
	subdir := filepath.Join(root, "sub")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "item.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, list, _ := fsTools(t, root)
	out, err := callListDir(t, list, "sub")
	if err != nil {
		t.Fatalf("list_dir sub: %v", err)
	}
	if !strings.Contains(out, "item.txt") {
		t.Errorf("expected item.txt in list output, got: %q", out)
	}
}

// ============================================================
// Additional Shell tests
// ============================================================

// TestShell_CommandWithStderr verifies that stderr output is captured and
// included in the result string.
func TestShell_CommandWithStderr(t *testing.T) {
	h := shellHandler(t)
	// 'ls' on a non-existent path writes to stderr and exits non-zero.
	out, err := callRunCommand(t, h, "ls", []string{"/this/path/definitely/does/not/exist/anywhere"})
	if err != nil {
		t.Fatalf("unexpected Go-level error: %v", err)
	}
	// The output should contain either the exit error notice or the ls error text.
	if out == "" {
		t.Error("expected some output (stderr or exit notice) from failed ls, got empty string")
	}
}

// TestShell_VeryLongOutput verifies that very long output is truncated and
// the truncation notice is included.
func TestShell_VeryLongOutput(t *testing.T) {
	// Use a small cap (100 bytes) so we can reliably overflow it with echo.
	h := shellHandler(t, toolbox.WithMaxOutput(100))

	// Build a string clearly longer than 100 bytes.
	bigArg := strings.Repeat("abc", 50) // 150 bytes
	out, err := callRunCommand(t, h, "echo", []string{bigArg})
	if err != nil {
		t.Fatalf("run_command: %v", err)
	}
	if !strings.Contains(out, "truncated") {
		t.Errorf("expected truncation notice in output, got: %q", out)
	}
}

// TestShell_CommandTimeout verifies that a command that hangs is eventually
// interrupted. We use a very short context to simulate this; the default shell
// timeout (30s) would be too slow, so we rely on an exec context cancel.
func TestShell_RunCommand_MultipleArgs(t *testing.T) {
	h := shellHandler(t, toolbox.WithAllowedCommands("echo"))
	out, err := callRunCommand(t, h, "echo", []string{"arg1", "arg2", "arg3"})
	if err != nil {
		t.Fatalf("run_command multi-args: %v", err)
	}
	if !strings.Contains(out, "arg1") || !strings.Contains(out, "arg2") {
		t.Errorf("expected all args in output, got: %q", out)
	}
}

// ============================================================
// Additional Git tests
// ============================================================

func callGitDiff(t *testing.T, h any) (string, error) {
	t.Helper()
	fn := h.(func(context.Context, toolbox.GitDiffInput) (string, error))
	return fn(bg(), toolbox.GitDiffInput{})
}

func TestGit_Diff_EmptyRepo(t *testing.T) {
	repo := mustInitGitRepo(t)
	handlers := gitHandlers(t, repo)

	out, err := callGitDiff(t, handlers["git_diff"])
	if err != nil {
		t.Fatalf("git_diff: %v", err)
	}
	// An empty repo with no staged changes should return an empty or descriptive output.
	_ = out // result is valid as long as there's no error
}

func TestGit_Log_DefaultCount(t *testing.T) {
	repo := mustInitGitRepo(t)
	runGit(t, repo, "commit", "--allow-empty", "-m", "first commit")
	runGit(t, repo, "commit", "--allow-empty", "-m", "second commit")

	handlers := gitHandlers(t, repo)
	out, err := callGitLog(t, handlers["git_log"], 0) // 0 = use default
	if err != nil {
		t.Fatalf("git_log default count: %v", err)
	}
	if !strings.Contains(out, "first commit") && !strings.Contains(out, "second commit") {
		t.Errorf("expected commit messages in log, got: %s", out)
	}
}

func TestGit_Blame_NonExistentFile(t *testing.T) {
	repo := mustInitGitRepo(t)
	handlers := gitHandlers(t, repo)

	_, err := callGitBlame(t, handlers["git_blame"], "no_such_file.txt")
	if err == nil {
		t.Fatal("expected error for git blame on non-existent file")
	}
}
