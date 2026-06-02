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
	defaultContainerTimeout = 30 * time.Second
)

// ContainerConfig holds configuration for container tools.
type ContainerConfig struct {
	timeout     time.Duration
	kubectlPath string
	kubeContext string
}

// ContainerOption is a functional option for ContainerTools.
type ContainerOption func(*ContainerConfig)

// WithContainerTimeout sets the container API timeout. Default is 30 seconds.
func WithContainerTimeout(d time.Duration) ContainerOption {
	return func(c *ContainerConfig) {
		c.timeout = d
	}
}

// WithKubectlPath sets the path to the kubectl executable. Default is "kubectl".
func WithKubectlPath(path string) ContainerOption {
	return func(c *ContainerConfig) {
		c.kubectlPath = path
	}
}

// WithKubeContext sets the Kubernetes context to use.
func WithKubeContext(context string) ContainerOption {
	return func(c *ContainerConfig) {
		c.kubeContext = context
	}
}

// ContainerTools is a collection of container orchestration tools.
// It provides read-only operations that interact with Docker via its API.
type ContainerTools struct {
	cfg ContainerConfig
}

// Container returns a ContainerTools collection with the given options applied.
func Container(opts ...ContainerOption) *ContainerTools {
	cfg := ContainerConfig{
		timeout:     defaultContainerTimeout,
		kubectlPath: "kubectl",
		kubeContext: "",
	}
	for _, o := range opts {
		o(&cfg)
	}
	return &ContainerTools{cfg: cfg}
}

// Tools implements hamr.ToolCollection.
func (c *ContainerTools) Tools() []hamr.ToolInfo {
	return []hamr.ToolInfo{
		{
			Name:        "docker_list_containers",
			Description: "List running Docker containers.",
			Handler:     c.dockerListContainers,
		},
		{
			Name:        "docker_list_images",
			Description: "List Docker images on the host.",
			Handler:     c.dockerListImages,
		},
		{
			Name:        "docker_inspect_container",
			Description: "Inspect a Docker container by name or ID.",
			Handler:     c.dockerInspectContainer,
		},
		{
			Name:        "docker_logs",
			Description: "Get logs from a Docker container.",
			Handler:     c.dockerLogs,
		},
	}
}

// DockerListContainersInput is the input for docker_list_containers.
type DockerListContainersInput struct {
	All bool `json:"all" desc:"show all containers, not just running ones" optional:"true"`
}

// DockerListImagesInput is the input for docker_list_images.
type DockerListImagesInput struct{}

// DockerInspectContainerInput is the input for docker_inspect_container.
type DockerInspectContainerInput struct {
	Container string `json:"container" desc:"container name or ID to inspect"`
}

// DockerLogsInput is the input for docker_logs.
type DockerLogsInput struct {
	Container string `json:"container" desc:"container name or ID"`
	Tail      int    `json:"tail" desc:"number of lines to show from end" optional:"true"`
}

// dockerListContainers lists containers via the Docker CLI.
func (c *ContainerTools) dockerListContainers(ctx context.Context, in DockerListContainersInput) (string, error) {
	args := []string{"ps", "--format", "{{json .}}"}
	if in.All {
		args = append(args, "-a")
	}
	return c.runDockerCommand(ctx, args)
}

// dockerListImages lists Docker images.
func (c *ContainerTools) dockerListImages(ctx context.Context, _ DockerListImagesInput) (string, error) {
	args := []string{"images", "--format", "{{json .}}"}
	return c.runDockerCommand(ctx, args)
}

// dockerInspectContainer inspects a specific container.
func (c *ContainerTools) dockerInspectContainer(ctx context.Context, in DockerInspectContainerInput) (string, error) {
	if in.Container == "" {
		return "", fmt.Errorf("docker_inspect_container: container name must not be empty")
	}
	args := []string{"inspect", "--format", "{{json .}}", in.Container}
	return c.runDockerCommand(ctx, args)
}

// dockerLogs retrieves container logs.
func (c *ContainerTools) dockerLogs(ctx context.Context, in DockerLogsInput) (string, error) {
	if in.Container == "" {
		return "", fmt.Errorf("docker_logs: container name must not be empty")
	}
	args := []string{"logs", "--timestamps"}
	if in.Tail > 0 {
		args = append(args, fmt.Sprintf("--tail=%d", in.Tail))
	}
	args = append(args, in.Container)
	return c.runDockerCommand(ctx, args)
}

// runDockerCommand executes a docker CLI command and returns the output.
// It validates the command name to prevent injection attacks.
func (c *ContainerTools) runDockerCommand(ctx context.Context, args []string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.cfg.timeout)
	defer cancel()

	// Security: reject null bytes in any argument
	for _, arg := range args {
		if strings.ContainsRune(arg, 0) {
			return "", fmt.Errorf("docker: argument contains null byte")
		}
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut

	if err := cmd.Run(); err != nil {
		combined := out.String() + errOut.String()
		if combined != "" {
			return "", fmt.Errorf("docker %s: %w\n%s", strings.Join(args, " "), err, combined)
		}
		return "", fmt.Errorf("docker %s: %w", strings.Join(args, " "), err)
	}

	return out.String(), nil
}

// runKubectlCommand executes a kubectl CLI command and returns the output.
// It validates the command name to prevent injection attacks.
func (c *ContainerTools) runKubectlCommand(ctx context.Context, args []string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.cfg.timeout)
	defer cancel()

	// Security: reject null bytes in any argument
	for _, arg := range args {
		if strings.ContainsRune(arg, 0) {
			return "", fmt.Errorf("kubectl: argument contains null byte")
		}
	}

	// Prepend kubectl path and context if set
	kubectlArgs := []string{c.cfg.kubectlPath}
	if c.cfg.kubeContext != "" {
		kubectlArgs = append(kubectlArgs, "--context="+c.cfg.kubeContext)
	}
	kubectlArgs = append(kubectlArgs, args...)

	cmd := exec.CommandContext(ctx, kubectlArgs[0], kubectlArgs[1:]...)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut

	if err := cmd.Run(); err != nil {
		combined := out.String() + errOut.String()
		if combined != "" {
			return "", fmt.Errorf("kubectl %s: %w\n%s", strings.Join(args, " "), err, combined)
		}
		return "", fmt.Errorf("kubectl %s: %w", strings.Join(args, " "), err)
	}

	return out.String(), nil
}
}