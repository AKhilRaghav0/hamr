package toolbox

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AKhilRaghav0/hamr"
)

// ContainerOrchestrationTools provides tools for interacting with container orchestration platforms (Docker, Kubernetes).
type ContainerOrchestrationTools struct {
	// Host is the Docker daemon host or Kubernetes API server host.
	Host string
	// Port is the port for the host.
	Port int
	// Namespace is the Kubernetes namespace to use.
	Namespace string
}

// ContainerOrchestrationOption is a functional option for configuring the ContainerOrchestrationTools.
type ContainerOrchestrationOption func(*ContainerOrchestrationTools)

// WithHost sets the host for Docker or Kubernetes.
func WithHost(host string) ContainerOrchestrationOption {
	return func(c *ContainerOrchestrationTools) {
		c.Host = host
	}
}

// WithPort sets the port for the host.
func WithPort(port int) ContainerOrchestrationOption {
	return func(c *ContainerOrchestrationTools) {
		c.Port = port
	}
}

// WithNamespace sets the Kubernetes namespace.
func WithNamespace(namespace string) ContainerOrchestrationOption {
	return func(c *ContainerOrchestrationTools) {
		c.Namespace = namespace
	}
}

// ContainerOrchestration returns a new ContainerOrchestrationTools instance with the given options.
func ContainerOrchestration(opts ...ContainerOrchestrationOption) *ContainerOrchestrationTools {
	c := &ContainerOrchestrationTools{
		Host:     "localhost",
		Port:     2375, // Docker default port
		Namespace: "default",
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// DockerListContainers returns a tool handler that simulates listing Docker containers.
func (c *ContainerOrchestrationTools) DockerListContainers() interface{} {
	return func(ctx context.Context, args struct {
		All bool `json:"all" desc:"Show all containers (default: just running)"`)
	}) (interface{}, error) {
		// Simulate returning container information.
		containers := []struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Status string `json:"status"`
			Image  string `json:"image"`
		}{}
		if args.All {
			containers = append(containers, struct {
				ID     string `json:"id"`
				Name   string `json:"name"`
				Status string `json:"status"`
				Image  string `json:"image"`
			}{ID: "container1", Name: "web", Status: "running", Image: "nginx:latest"})
			containers = append(containers, struct {
				ID     string `json:"id"`
				Name   string `json:"name"`
				Status string `json:"status"`
				Image  string `json:"image"`
			}{ID: "container2", Name: "db", Status: "exited", Image: "postgres:13"})
		} else {
			containers = append(containers, struct {
				ID     string `json:"id"`
				Name   string `json:"name"`
				Status string `json:"status"`
				Image  string `json:"image"`
			}{ID: "container1", Name: "web", Status: "running", Image: "nginx:latest"})
		}
		return containers, nil
	}
}

// DockerRunContainer returns a tool handler that simulates running a Docker container.
func (c *ContainerOrchestrationTools) DockerRunContainer() interface{} {
	return func(ctx context.Context, args struct {
		Image   string `json:"image" desc:"Docker image to run" required:"true"`
		Name    string `json:"name" desc:"Container name"`
		Ports   []string `json:"ports" desc:"Port mappings (e.g., 8080:80)"`
		Env     map[string]string `json:"env" desc:"Environment variables"`
		Command []string `json:"command" desc:"Command to run (overrides image default)"`)
	}) (interface{}, error) {
		if args.Image == "" {
			return nil, errors.New("image is required")
		}
		// Simulate running the container and returning the container ID.
		containerID := fmt.Sprintf("docker-%d", time.Now().UnixNano())
		return map[string]interface{}{
			"id":     containerID,
			"image":  args.Image,
			"name":   args.Name,
			"status": "running",
		}, nil
	}
}

// KubernetesListPods returns a tool handler that simulates listing Kubernetes pods.
func (c *ContainerOrchestrationTools) KubernetesListPods() interface{} {
	return func(ctx context.Context, args struct {
		Namespace string `json:"namespace" desc:"Kubernetes namespace (optional, uses toolbox default if not provided)"`
		Label     string `json:"label" desc:"Label selector (optional)"`)
	}) (interface{}, error) {
		namespace := args.Namespace
		if namespace == "" {
			namespace = c.Namespace
		}
		// Simulate returning pod information.
		pods := []struct {
			Name     string `json:"name"`
			Namespace string `json:"namespace"`
			Status   string `json:"status"`
			Containers []string `json:"containers"`
		}{}
		pods = append(pods, struct {
			Name     string `json:"name"`
			Namespace string `json:"namespace"`
			Status   string `json:"status"`
			Containers []string `json:"containers"`
		}{Name: "web-pod", Namespace: namespace, Status: "Running", Containers: []string{"nginx"}})
		pods = append(pods, struct {
			Name     string `json:"name"`
			Namespace string `json:"namespace"`
			Status   string `json:"status"`
			Containers []string `json:"containers"`
		}{Name: "db-pod", Namespace: namespace, Status: "Running", Containers: []string{"postgres"}})
		return pods, nil
	}
}

// KubernetesCreatePod returns a tool handler that simulates creating a Kubernetes pod.
func (c *ContainerOrchestrationTools) KubernetesCreatePod() interface{} {
	return func(ctx context.Context, args struct {
		Name      string `json:"name" desc:"Pod name" required:"true"`
		Namespace string `json:"namespace" desc:"Kubernetes namespace (optional, uses toolbox default if not provided)"`
		Image     string `json:"image" desc:"Container image" required:"true"`
	}) (interface{}, error) {
		if args.Name == "" || args.Image == "" {
			return nil, errors.New("name and image are required")
		}
		namespace := args.Namespace
		if namespace == "" {
			namespace = c.Namespace
		}
		// Simulate creating the pod.
		return map[string]interface{}{
			"name":      args.Name,
			"namespace": namespace,
			"image":     args.Image,
			"status":    "Created",
		}, nil
	}
}