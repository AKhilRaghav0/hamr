package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
)

var devCmd = &cobra.Command{
	Use:   "dev [path]",
	Short: "Build and run the MCP server with live-reload on file changes",
	Long: `Start the MCP server in development mode.

hamr dev:
  - Builds the server binary using "go build"
  - Runs the binary
  - Watches all .go files for changes using fsnotify
  - On change: kills the running process, rebuilds, restarts
  - Handles SIGINT (Ctrl-C) gracefully`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDev,
}

// devState holds mutable state for the dev-mode runner.
type devState struct {
	mu      sync.Mutex
	proc    *os.Process // currently running server process, nil if not started
	binPath string      // path to the compiled binary
}

func runDev(cmd *cobra.Command, args []string) error {
	root := "."
	if len(args) == 1 {
		root = args[0]
	}

	abs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	// Place the binary in a temp directory so it stays off the source tree.
	tmpDir, err := os.MkdirTemp("", "hamr-dev-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	state := &devState{
		binPath: filepath.Join(tmpDir, "hamr-server"),
	}

	devLog("info", "starting dev mode — watching %s", abs)

	// Intercept SIGINT / SIGTERM so we can clean up the child process.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// rebuildAndRestart performs a full build+restart cycle.
	rebuildAndRestart := func() {
		state.killServer()
		if err := state.build(abs); err != nil {
			devLog("error", "build failed: %v", err)
			return
		}
		if err := state.startServer(); err != nil {
			devLog("error", "start failed: %v", err)
		}
	}

	// Initial build and start.
	rebuildAndRestart()

	// Set up the file watcher.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}
	defer watcher.Close()

	if err := watchGoFiles(watcher, abs); err != nil {
		return fmt.Errorf("watch files: %w", err)
	}

	// Debounce timer — wait 300 ms after the last event before rebuilding so
	// that rapid successive writes (e.g. from an editor saving multiple files)
	// collapse into a single rebuild.
	debounce := time.NewTimer(0)
	if !debounce.Stop() {
		<-debounce.C
	}

	for {
		select {
		case sig := <-sigCh:
			devLog("info", "received %v — shutting down", sig)
			state.killServer()
			return nil

		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if !isGoFile(event.Name) {
				continue
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Remove) {
				devLog("info", "change detected: %s", filepath.Base(event.Name))
				debounce.Reset(300 * time.Millisecond)
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			devLog("warn", "watcher error: %v", err)

		case <-debounce.C:
			rebuildAndRestart()
		}
	}
}

// build runs "go build -o <binPath> ." in the given directory.
func (s *devState) build(dir string) error {
	devLog("info", "building...")
	start := time.Now()

	cmd := exec.Command("go", "build", "-o", s.binPath, ".")
	cmd.Dir = dir
	cmd.Stdout = os.Stderr // route build output to stderr so it doesn't pollute stdio
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return err
	}

	devLog("info", "build succeeded in %s", time.Since(start).Round(time.Millisecond))
	return nil
}

// startServer executes the compiled binary and stores the process handle.
func (s *devState) startServer() error {
	cmd := exec.Command(s.binPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("exec server: %w", err)
	}

	s.mu.Lock()
	s.proc = cmd.Process
	s.mu.Unlock()

	devLog("info", "server started (pid %d)", cmd.Process.Pid)

	// Reap the child in the background so we get its exit status logged.
	go func() {
		state := cmd.Wait()
		s.mu.Lock()
		s.proc = nil
		s.mu.Unlock()
		if state != nil {
			devLog("warn", "server exited: %v", state)
		} else {
			devLog("info", "server exited cleanly")
		}
	}()

	return nil
}

// killServer sends SIGTERM to the running server process and waits briefly.
func (s *devState) killServer() {
	s.mu.Lock()
	proc := s.proc
	s.proc = nil
	s.mu.Unlock()

	if proc == nil {
		return
	}

	devLog("info", "stopping server (pid %d)", proc.Pid)
	// Try graceful shutdown first.
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		// Already gone — that is fine.
		return
	}

	// Give the process 2 s to exit before sending SIGKILL.
	done := make(chan struct{})
	go func() {
		proc.Wait() //nolint:errcheck
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		devLog("warn", "server did not exit in time, sending SIGKILL")
		proc.Kill() //nolint:errcheck
	}
}

// watchGoFiles adds the root directory and all subdirectories (excluding
// vendor/.git/testdata) to the watcher.
func watchGoFiles(w *fsnotify.Watcher, root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			base := info.Name()
			if base == "vendor" || base == ".git" || base == "testdata" {
				return filepath.SkipDir
			}
			return w.Add(path)
		}
		return nil
	})
}

// isGoFile returns true when path has a .go extension.
func isGoFile(path string) bool {
	return filepath.Ext(path) == ".go"
}

// devLog prints a timestamped, coloured log line to stderr.
func devLog(level, format string, args ...any) {
	ts := time.Now().Format("15:04:05")
	msg := fmt.Sprintf(format, args...)

	var levelStr string
	switch level {
	case "info":
		levelStr = colGreen + "INF" + colReset
	case "warn":
		levelStr = colYellow + "WRN" + colReset
	case "error":
		levelStr = colRed + "ERR" + colReset
	default:
		levelStr = level
	}

	fmt.Fprintf(os.Stderr, "%s%s%s %s  %s\n", colBold, ts, colReset, levelStr, msg)
}
