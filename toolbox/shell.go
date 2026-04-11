package toolbox

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/AKhilRaghav0/hamr"
)

const (
	defaultShellMaxOutput = 10 * 1024 // 10 KiB
	shellTimeout          = 30 * time.Second
)

// shellConfig holds configuration for ShellTools.
type shellConfig struct {
	allowedCommands map[string]struct{} // nil means all commands allowed
	maxOutput       int
}

// ShellOption is a functional option for ShellTools.
type ShellOption func(*shellConfig)

// WithAllowedCommands restricts execution to the named commands only.
// If this option is not provided, any command may be run.
func WithAllowedCommands(cmds ...string) ShellOption {
	return func(c *shellConfig) {
		if c.allowedCommands == nil {
			c.allowedCommands = make(map[string]struct{})
		}
		for _, cmd := range cmds {
			c.allowedCommands[cmd] = struct{}{}
		}
	}
}

// WithMaxOutput sets the maximum number of bytes captured from a command's
// combined stdout+stderr. Output beyond this limit is truncated.
// Default is 10 KiB.
func WithMaxOutput(n int) ShellOption {
	return func(c *shellConfig) {
		c.maxOutput = n
	}
}

// ShellTools is a collection of shell command execution tools.
type ShellTools struct {
	workDir string
	cfg     shellConfig
}

// Shell returns a ShellTools collection whose commands always execute inside
// workDir. Use WithAllowedCommands to restrict which executables may be run.
func Shell(workDir string, opts ...ShellOption) *ShellTools {
	cfg := shellConfig{
		maxOutput: defaultShellMaxOutput,
	}
	for _, o := range opts {
		o(&cfg)
	}
	return &ShellTools{workDir: workDir, cfg: cfg}
}

// Tools implements hamr.ToolCollection.
func (s *ShellTools) Tools() []hamr.ToolInfo {
	return []hamr.ToolInfo{
		{
			Name:        "run_command",
			Description: "Execute a shell command in the configured working directory. Returns combined stdout and stderr.",
			Handler:     s.runCommand,
		},
	}
}

// ---- input structs ----

// RunCommandInput is the input for the run_command tool.
type RunCommandInput struct {
	Command string   `json:"command" desc:"the executable to run"`
	Args    []string `json:"args" desc:"command-line arguments" optional:"true"`
}

// ---- handlers ----

func (s *ShellTools) runCommand(ctx context.Context, in RunCommandInput) (string, error) {
	if in.Command == "" {
		return "", fmt.Errorf("run_command: command must not be empty")
	}

	// Reject null bytes in the command name. On Linux the kernel truncates
	// exec args at the first null byte, which could let an attacker hide the
	// real binary name from an allowlist check (e.g. "rm\x00" passes an "rm"
	// check lexically but some layers would strip the suffix).
	if strings.ContainsRune(in.Command, 0) {
		return "", fmt.Errorf("run_command: command name contains null byte")
	}

	// Reject path separators in the command name. exec.LookPath already handles
	// the case where the name contains a slash by treating it as a literal path;
	// rejecting them here prevents an attacker from bypassing a name-based
	// allowlist with an absolute or relative path (e.g. "/bin/rm" when only
	// "rm" is allowed).
	if strings.ContainsRune(in.Command, '/') {
		return "", fmt.Errorf("run_command: command name must not contain path separators")
	}

	// Enforce allowlist when configured.
	if s.cfg.allowedCommands != nil {
		if _, ok := s.cfg.allowedCommands[in.Command]; !ok {
			return "", fmt.Errorf("run_command: command %q is not in the allowed list", in.Command)
		}
	}

	// Cap execution time.
	ctx, cancel := context.WithTimeout(ctx, shellTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, in.Command, in.Args...)
	cmd.Dir = s.workDir

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	runErr := cmd.Run()

	output := buf.String()
	truncated := false
	if len(output) > s.cfg.maxOutput {
		output = output[:s.cfg.maxOutput]
		truncated = true
	}

	var sb strings.Builder
	sb.WriteString(output)
	if truncated {
		sb.WriteString(fmt.Sprintf("\n[output truncated at %d bytes]", s.cfg.maxOutput))
	}
	if runErr != nil {
		sb.WriteString(fmt.Sprintf("\n[exit error: %v]", runErr))
	}

	return sb.String(), nil
}
