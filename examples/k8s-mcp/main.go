// k8s-mcp is an MCP server that lets Claude interact with your Kubernetes cluster.
// It wraps kubectl so Claude can check pod status, read logs, describe resources, etc.
//
// Usage:
//
//	go build -o k8s-mcp .
//	# Make sure kubectl is configured, then add to Claude Desktop config
//
// Now you can ask Claude: "What pods are failing in production?"
// and it will actually check your cluster.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/AKhilRaghav0/hamr"
	"github.com/AKhilRaghav0/hamr/middleware"
)

// --- Input types ---

type GetPodsInput struct {
	Namespace string `json:"namespace" desc:"kubernetes namespace" default:"default"`
	Selector  string `json:"selector" desc:"label selector like app=nginx" optional:"true"`
}

type GetLogsInput struct {
	Pod       string `json:"pod" desc:"pod name"`
	Namespace string `json:"namespace" desc:"kubernetes namespace" default:"default"`
	Lines     int    `json:"lines" desc:"number of log lines" default:"50"`
	Container string `json:"container" desc:"container name (for multi-container pods)" optional:"true"`
}

type DescribeInput struct {
	Resource  string `json:"resource" desc:"resource type (pod, deployment, service, etc.)"`
	Name      string `json:"name" desc:"resource name"`
	Namespace string `json:"namespace" desc:"kubernetes namespace" default:"default"`
}

type GetResourcesInput struct {
	Resource  string `json:"resource" desc:"resource type: pods, deployments, services, ingresses, configmaps, etc."`
	Namespace string `json:"namespace" desc:"namespace, or 'all' for all namespaces" default:"default"`
}

type GetEventsInput struct {
	Namespace string `json:"namespace" desc:"namespace to get events from" default:"default"`
}

type TopInput struct {
	Resource  string `json:"resource" desc:"'pods' or 'nodes'" default:"pods" enum:"pods,nodes"`
	Namespace string `json:"namespace" desc:"namespace (only for pods)" default:"default"`
}

// --- Handlers ---

func getPods(_ context.Context, in GetPodsInput) (string, error) {
	args := []string{"get", "pods", "-n", in.Namespace, "-o", "wide"}
	if in.Selector != "" {
		args = append(args, "-l", in.Selector)
	}
	return kubectl(args...)
}

func getLogs(_ context.Context, in GetLogsInput) (string, error) {
	args := []string{"logs", in.Pod, "-n", in.Namespace, "--tail", fmt.Sprintf("%d", in.Lines)}
	if in.Container != "" {
		args = append(args, "-c", in.Container)
	}
	return kubectl(args...)
}

func describe(_ context.Context, in DescribeInput) (string, error) {
	return kubectl("describe", in.Resource, in.Name, "-n", in.Namespace)
}

func getResources(_ context.Context, in GetResourcesInput) (string, error) {
	args := []string{"get", in.Resource}
	if in.Namespace == "all" {
		args = append(args, "--all-namespaces")
	} else {
		args = append(args, "-n", in.Namespace)
	}
	args = append(args, "-o", "wide")
	return kubectl(args...)
}

func getEvents(_ context.Context, in GetEventsInput) (string, error) {
	return kubectl("get", "events", "-n", in.Namespace, "--sort-by=.lastTimestamp")
}

func top(_ context.Context, in TopInput) (string, error) {
	if in.Resource == "nodes" {
		return kubectl("top", "nodes")
	}
	return kubectl("top", "pods", "-n", in.Namespace)
}

var kubectlPath = "/usr/local/bin/kubectl"

func kubectl(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, kubectlPath, args...)
	// Use custom kubeconfig if set
	if kc := os.Getenv("KUBECONFIG"); kc != "" {
		cmd.Env = append(os.Environ(), "KUBECONFIG="+kc)
	}
	out, err := cmd.CombinedOutput()
	result := strings.TrimSpace(string(out))
	if err != nil {
		if result != "" {
			return "", fmt.Errorf("kubectl %s: %s", strings.Join(args, " "), result)
		}
		return "", fmt.Errorf("kubectl %s: %w", strings.Join(args, " "), err)
	}
	if result == "" {
		return "no output", nil
	}
	return result, nil
}

func main() {
	s := mcpx.New("k8s-mcp", "1.0.0",
		mcpx.WithDescription("Kubernetes cluster tools — pods, logs, resources, events"),
	)

	s.Use(
		middleware.Logger(),
		middleware.Recovery(),
		middleware.Timeout(20*time.Second),
	)

	s.Tool("get_pods", "List pods in a namespace (with optional label selector)", getPods)
	s.Tool("get_logs", "Get logs from a pod", getLogs)
	s.Tool("describe", "Describe a Kubernetes resource in detail", describe)
	s.Tool("get_resources", "List any resource type (deployments, services, etc.)", getResources)
	s.Tool("get_events", "Get recent cluster events in a namespace", getEvents)
	s.Tool("top", "Show resource usage (CPU/memory) for pods or nodes", top)

	// Check for --dashboard flag
	for _, arg := range os.Args[1:] {
		if arg == "--dashboard" || arg == "-d" {
			fmt.Fprintf(os.Stderr, "k8s-mcp starting with dashboard on :9091\n")
			log.Fatal(s.RunSSEWithDashboard(":9091"))
		}
	}

	log.Fatal(s.Run())
}
