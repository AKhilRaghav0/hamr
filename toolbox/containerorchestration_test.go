package toolbox

import (
	"context"
	"testing"

	"github.com/AKhilRaghav0/hamr"
)

func TestContainerOrchestrationTools_DockerListContainers(t *testing.T) {
	t.Parallel()

	c := ContainerOrchestration()
	tool := c.DockerListContainers().(func(context.Context, struct {
		All bool `json:"all"`
	}) (interface{}, error))

	ctx := context.Background()
	// Test with All=false (default)
	result, err := tool(ctx, struct {
		All bool
	}{All: false})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	containers, ok := result.([]interface{})
	if !ok {
		t.Fatalf("Expected slice of containers, got %T", result)
	}
	if len(containers) == 0 {
		t.Fatalf("Expected at least one container")
	}
	// Check the first container.
	first := containers[0].(map[string]interface{})
	if first["status"].(string) != "running" {
		t.Errorf("Expected running container, got %s", first["status"].(string))
	}

	// Test with All=true
	result, err = tool(ctx, struct {
		All bool
	}{All: true})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	containers = result.([]interface{})
	if len(containers) < 2 {
		t.Fatalf("Expected at least two containers when All=true")
	}
}

func TestContainerOrchestrationTools_DockerRunContainer(t *testing.T) {
	t.Parallel()

	c := ContainerOrchestration()
	tool := c.DockerRunContainer().(func(context.Context, struct {
		Image   string `json:"image"`
		Name    string `json:"name"`
		Ports   []string `json:"ports"`
		Env     map[string]string `json:"env"`
		Command []string `json:"command"`
	}) (interface{}, error))

	ctx := context.Background()
	result, err := tool(ctx, struct {
		Image   string
		Name    string
		Ports   []string
		Env     map[string]string
		Command []string
	}{Image: "my-image:latest", Name: "my-container", Ports: []string{"8080:80"}, Env: map[string]string{"ENV": "value"}, Command: []string{"ls"}})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	res := result.(map[string]interface{})
	if res["image"].(string) != "my-image:latest" {
		t.Errorf("Expected image my-image:latest, got %s", res["image"].(string))
	}
	if res["name"].(string) != "my-container" {
		t.Errorf("Expected name my-container, got %s", res["name"].(string))
	}
	if res["status"].(string) != "running" {
		t.Errorf("Expected status running, got %s", res["status"].(string))
	}
	// Check that an ID was generated.
	if res["id"] == nil {
		t.Fatalf("Expected container ID to be set")
	}
}

func TestContainerOrchestrationTools_KubernetesListPods(t *testing.T) {
	t.Parallel()

	c := ContainerOrchestration()
	tool := c.KubernetesListPods().(func(context.Context, struct {
		Namespace string `json:"namespace"`
		Label     string `json:"label"`
	}) (interface{}, error))

	ctx := context.Background()
	// Test with default namespace.
	result, err := tool(ctx, struct {
		Namespace string
		Label     string
	}{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	pods, ok := result.([]interface{})
	if !ok {
		t.Fatalf("Expected slice of pods, got %T", result)
	}
	if len(pods) == 0 {
		t.Fatalf("Expected at least one pod")
	}
	// Check that the namespace is default.
	first := pods[0].(map[string]interface{})
	if first["namespace"].(string) != "default" {
		t.Errorf("Expected namespace default, got %s", first["namespace"].(string))
	}

	// Test with custom namespace.
	result, err = tool(ctx, struct {
		Namespace string
		Label     string
	}{Namespace: "custom-ns"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	pods = result.([]interface{})
	if len(pods) == 0 {
		t.Fatalf("Expected at least one pod")
	}
	first = pods[0].(map[string]interface{})
	if first["namespace"].(string) != "custom-ns" {
		t.Errorf("Expected namespace custom-ns, got %s", first["namespace"].(string))
	}
}

func TestContainerOrchestrationTools_KubernetesCreatePod(t *testing.T) {
	t.Parallel()

	c := ContainerOrchestration()
	tool := c.KubernetesCreatePod().(func(context.Context, struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
		Image     string `json:"image"`
	}) (interface{}, error))

	ctx := context.Background()
	result, err := tool(ctx, struct {
		Name      string
		Namespace string
		Image     string
	}{Name: "my-pod", Namespace: "test-ns", Image: "my-image:latest"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	res := result.(map[string]interface{})
	if res["name"].(string) != "my-pod" {
		t.Errorf("Expected pod name my-pod, got %s", res["name"].(string))
	}
	if res["namespace"].(string) != "test-ns" {
		t.Errorf("Expected namespace test-ns, got %s", res["namespace"].(string))
	}
	if res["image"].(string) != "my-image:latest" {
		t.Errorf("Expected image my-image:latest, got %s", res["image"].(string))
	}
	if res["status"].(string) != "Created" {
		t.Errorf("Expected status Created, got %s", res["status"].(string))
	}
}

// Test with custom options.
func TestContainerOrchestrationTools_WithOptions(t *testing.T) {
	t.Parallel()

	c := ContainerOrchestration(
		WithHost("kubernetes-host"),
		WithPort(6443),
		WithNamespace("kube-system"),
	)
	if c.Host != "kubernetes-host" {
		t.Errorf("Expected host kubernetes-host, got %s", c.Host)
	}
	if c.Port != 6443 {
		t.Errorf("Expected port 6443, got %d", c.Port)
	}
	if c.Namespace != "kube-system" {
		t.Errorf("Expected namespace kube-system, got %s", c.Namespace)
	}
	// Check that the tools use the updated namespace.
	tool := c.KubernetesListPods().(func(context.Context, struct {
		Namespace string `json:"namespace"`
		Label     string `json:"label"`
	}) (interface{}, error))

	ctx := context.Background()
	result, err := tool(ctx, struct {
		Namespace string
		Label     string
	}{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	pods := result.([]interface{})
	if len(pods) == 0 {
		t.Fatalf("Expected at least one pod")
	}
	first := pods[0].(map[string]interface{})
	if first["namespace"].(string) != "kube-system" {
		t.Errorf("Expected namespace kube-system, got %s", first["namespace"].(string))
	}
}