package toolbox

import (
	"context"
	"testing"

	"github.com/AKhilRaghav0/hamr"
)

func TestCloudProviderTools_S3GetObject(t *testing.T) {
	t.Parallel()

	c := CloudProvider()
	tool := c.S3GetObject().(func(context.Context, struct {
		Bucket string `json:"bucket"`
		Key    string `json:"key"`
	}) (interface{}, error))

	ctx := context.Background()
	result, err := tool(ctx, struct {
		Bucket string
		Key    string
	}{Bucket: "my-bucket", Key: "my-key"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	expected := "content of my-bucket/my-key (from region us-east-1)"
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestCloudProviderTools_S3PutObject(t *testing.T) {
	t.Parallel()

	c := CloudProvider()
	tool := c.S3PutObject().(func(context.Context, struct {
		Bucket string `json:"bucket"`
		Key    string `json:"key"`
		Body   string `json:"body"`
	}) (interface{}, error))

	ctx := context.Background()
	result, err := tool(ctx, struct {
		Bucket string
		Key    string
		Body   string
	}{Bucket: "my-bucket", Key: "my-key", Body: "hello world"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	expected := "stored my-bucket/my-key with size 11 bytes (region us-east-1)"
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

func TestCloudProviderTools_EC2DescribeInstances_NoArgs(t *testing.T) {
	t.Parallel()

	c := CloudProvider()
	tool := c.EC2DescribeInstances().(func(context.Context, struct {
		InstanceIds []string `json:"instance_ids"`
	}) (interface{}, error))

	ctx := context.Background()
	result, err := tool(ctx, struct {
		InstanceIds []string
	}{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	instances, ok := result.([]interface{})
	if !ok {
		t.Fatalf("Expected slice of instances, got %T", result)
	}
	if len(instances) == 0 {
		t.Fatalf("Expected at least one instance")
	}
	// Check that the first instance has expected fields.
	first := instances[0].(map[string]interface{})
	if first["id"] == nil || first["state"] == nil || first["instance_type"] == nil || first["region"] == nil {
		t.Fatalf("Instance missing required fields")
	}
	if first["region"].(string) != "us-east-1" {
		t.Errorf("Expected region us-east-1, got %s", first["region"].(string))
	}
}

func TestCloudProviderTools_EC2DescribeInstances_WithArgs(t *testing.T) {
	t.Parallel()

	c := CloudProvider()
	tool := c.EC2DescribeInstances().(func(context.Context, struct {
		InstanceIds []string `json:"instance_ids"`
	}) (interface{}, error))

	ctx := context.Background()
	result, err := tool(ctx, struct {
		InstanceIds []string
	}{InstanceIds: []string{"i-abc", i-ndef"}})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	instances, ok := result.([]interface{})
	if !ok {
		t.Fatalf("Expected slice of instances, got %T", result)
	}
	if len(instances) != 2 {
		t.Fatalf("Expected 2 instances, got %d", len(instances))
	}
	// Check that the instance IDs match.
	for i, id := range []string{"i-abc", "i-ndef"} {
		inst := instances[i].(map[string]interface{})
		if inst["id"].(string) != id {
			t.Errorf("Expected instance ID %s, got %s", id, inst["id"].(string))
		}
	}
}

func TestCloudProviderTools_EC2StartInstances(t *testing.T) {
	t.Parallel()

	c := CloudProvider()
	tool := c.EC2StartInstances().(func(context.Context, struct {
		InstanceIds []string `json:"instance_ids"`
	}) (interface{}, error))

	ctx := context.Background()
	result, err := tool(ctx, struct {
		InstanceIds []string
	}{InstanceIds: []string{"i-123", "i-456"}})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	res := result.(map[string]interface{})
	if res["region"].(string) != "us-east-1" {
		t.Errorf("Expected region us-east-1, got %s", res["region"].(string))
	}
	started := res["started"].([]interface{})
	if len(started) != 2 {
		t.Fatalf("Expected 2 started instances, got %d", len(started))
	}
	if started[0].(string) != "i-123" || started[1].(string) != "i-456" {
		t.Errorf("Expected started instances i-123 and i-456")
	}
}

// Test with custom region and credentials.
func TestCloudProviderTools_WithOptions(t *testing.T) {
	t.Parallel()

	c := CloudProvider(
		WithRegion("eu-west-1"),
		WithCredentials("access-key", "secret-key"),
	)
	if c.Region != "eu-west-1" {
		t.Errorf("Expected region eu-west-1, got %s", c.Region)
	}
	if c.AccessKey != "access-key" || c.SecretKey != "secret-key" {
		t.Errorf("Expected credentials not set")
	}
	// Check that the tools use the updated region.
	tool := c.S3GetObject().(func(context.Context, struct {
		Bucket string `json:"bucket"`
		Key    string `json:"key"`
	}) (interface{}, error))

	ctx := context.Background()
	result, err := tool(ctx, struct {
		Bucket string
		Key    string
	}{Bucket: "b", Key: "k"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result != "content of b/k (from region eu-west-1)" {
		t.Errorf("Expected content with eu-west-1 region, got %v", result)
	}
}