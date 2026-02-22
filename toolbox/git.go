package toolbox

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/AKhilRaghav0/hamr"
)

const gitTimeout = 30 * time.Second

// GitTools is a collection of read-only git operations on a repository.
type GitTools struct {
	repoPath string
}

// Git returns a GitTools collection that operates on the git repository at
// repoPath. All operations are read-only and shell out to the system git binary.
func Git(repoPath string) *GitTools {
	return &GitTools{repoPath: repoPath}
}

// Tools implements mcpx.ToolCollection.
func (g *GitTools) Tools() []mcpx.ToolInfo {
	return []mcpx.ToolInfo{
		{
			Name:        "git_status",
			Description: "Show the working tree status of the repository (equivalent to git status).",
			Handler:     g.gitStatus,
		},
		{
			Name:        "git_diff",
			Description: "Show changes in the working tree or staged changes (equivalent to git diff or git diff --staged).",
			Handler:     g.gitDiff,
		},
		{
			Name:        "git_log",
			Description: "Show the commit history for the repository (equivalent to git log).",
			Handler:     g.gitLog,
		},
		{
			Name:        "git_blame",
			Description: "Show what revision and author last modified each line of a file (equivalent to git blame).",
			Handler:     g.gitBlame,
		},
	}
}

// ---- input structs ----

// GitStatusInput is the input for the git_status tool.
type GitStatusInput struct{}

// GitDiffInput is the input for the git_diff tool.
type GitDiffInput struct {
	Staged bool `json:"staged" desc:"show staged (cached) changes instead of unstaged changes" optional:"true"`
}

// GitLogInput is the input for the git_log tool.
type GitLogInput struct {
	Count int `json:"count" desc:"number of commits to show" default:"10" optional:"true"`
}

// GitBlameInput is the input for the git_blame tool.
type GitBlameInput struct {
	Path string `json:"path" desc:"path to the file to blame, relative to the repository root"`
}

// ---- helpers ----

// run executes a git command with a bounded timeout and returns its combined output.
func (g *GitTools) run(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = g.repoPath

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut

	if err := cmd.Run(); err != nil {
		combined := out.String() + errOut.String()
		if combined != "" {
			return "", fmt.Errorf("git %v: %w\n%s", args, err, combined)
		}
		return "", fmt.Errorf("git %v: %w", args, err)
	}

	return out.String(), nil
}

// ---- handlers ----

func (g *GitTools) gitStatus(ctx context.Context, _ GitStatusInput) (string, error) {
	out, err := g.run(ctx, "status")
	if err != nil {
		return "", fmt.Errorf("git_status: %w", err)
	}
	return out, nil
}

func (g *GitTools) gitDiff(ctx context.Context, in GitDiffInput) (string, error) {
	args := []string{"diff"}
	if in.Staged {
		args = append(args, "--staged")
	}
	out, err := g.run(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("git_diff: %w", err)
	}
	if out == "" {
		if in.Staged {
			return "no staged changes", nil
		}
		return "no unstaged changes", nil
	}
	return out, nil
}

func (g *GitTools) gitLog(ctx context.Context, in GitLogInput) (string, error) {
	count := in.Count
	if count <= 0 {
		count = 10
	}
	out, err := g.run(ctx,
		"log",
		fmt.Sprintf("-n%d", count),
		"--pretty=format:%h %ad %an%n  %s%n",
		"--date=short",
	)
	if err != nil {
		return "", fmt.Errorf("git_log: %w", err)
	}
	if out == "" {
		return "no commits found", nil
	}
	return out, nil
}

func (g *GitTools) gitBlame(ctx context.Context, in GitBlameInput) (string, error) {
	if in.Path == "" {
		return "", fmt.Errorf("git_blame: path must not be empty")
	}
	out, err := g.run(ctx, "blame", "--", in.Path)
	if err != nil {
		return "", fmt.Errorf("git_blame: %w", err)
	}
	return out, nil
}
