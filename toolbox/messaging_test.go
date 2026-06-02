package toolbox

import (
	"context"
	"testing"

	"github.com/AKhilRaghav0/hamr"
)

func TestMessagingTools_RabbitMQPublish(t *testing.T) {
	t.Parallel()

	c := Messaging()
	tool := c.RabbitMQPublish().(func(context.Context, struct {
		Exchange   string `json:"exchange"`
		RoutingKey string `json:"routing_key"`
		Body       string `json:"body"`
	}) (interface{}, error))

	ctx := context.Background()
	result, err := tool(ctx, struct {
		Exchange   string
		RoutingKey string
		Body       string
	}{Exchange: "my-exchange", RoutingKey: "my-key", Body: "Hello World"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	res := result.(map[string]interface{})
	if res["exchange"].(string) != "my-exchange" {
		t.Errorf("Expected exchange my-exchange, got %s", res["exchange"].(string))
	}
	if res["routing_key"].(string) != "my-key" {
		t.Errorf("Expected routing key my-key, got %s", res["routing_key"].(string))
	}
	if res["body"].(string) != "Hello World" {
		t.Errorf("Expected body Hello World, got %s", res["body"].(string))
	}
	if res["timestamp"] == nil {
		t.Fatalf("Expected timestamp to be set")
	}
}

func TestMessagingTools_RabbitMQConsume(t *testing.T) {
	t.Parallel()

	c := Messaging()
	tool := c.RabbitMQConsume().(func(context.Context, struct {
		Queue   string `json:"queue"`
		AutoAck bool   `json:"auto_ack"`
	}) (interface{}, error))

	ctx := context.Background()
	result, err := tool(ctx, struct {
		Queue   string
		AutoAck bool
	}{Queue: "my-queue", AutoAck: true})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	res := result.(map[string]interface{})
	if res["queue"].(string) != "my-queue" {
		t.Errorf("Expected queue my-queue, got %s", res["queue"].(string))
	}
	if res["body"].(string) != "Hello from RabbitMQ" {
		t.Errorf("Expected body Hello from RabbitMQ, got %s", res["body"].(string))
	}
	if res["auto_ack"].(bool) != true {
		t.Errorf("Expected auto_ack true, got %v", res["auto_ack"].(bool))
	}
	if res["timestamp"] == nil {
		t.Fatalf("Expected timestamp to be set")
	}
}

func TestMessagingTools_RedisPublish(t *testing.T) {
	t.Parallel()

	c := Messaging()
	tool := c.RedisPublish().(func(context.Context, struct {
		Channel string `json:"channel"`
		Message string `json:"message"`
	}) (interface{}, error))

	ctx := context.Background()
	result, err := tool(ctx, struct {
		Channel string
		Message string
	}{Channel: "my-channel", Message: "Hello Redis"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	res := result.(map[string]interface{})
	if res["channel"].(string) != "my-channel" {
		t.Errorf("Expected channel my-channel, got %s", res["channel"].(string))
	}
	if res["message"].(string) != "Hello Redis" {
		t.Errorf("Expected message Hello Redis, got %s", res["message"].(string))
	}
	if res["timestamp"] == nil {
		t.Fatalf("Expected timestamp to be set")
	}
	if res["db"].(int) != 0 {
		t.Errorf("Expected db 0, got %d", res["db"].(int))
	}
}

func TestMessagingTools_RedisSubscribe(t *testing.T) {
	t.Parallel()

	c := Messaging()
	tool := c.RedisSubscribe().(func(context.Context, struct {
		Channel string `json:"channel"`
		Count   int    `json:"count"`
	}) (interface{}, error))

	ctx := context.Background()
	// Test default count (1)
	result, err := tool(ctx, struct {
		Channel string
		Count   int
	}{Channel: "my-channel", Count: 1})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	messages := result.([]interface{})
	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}
	msg := messages[0].(map[string]interface{})
	if msg["channel"].(string) != "my-channel" {
		t.Errorf("Expected channel my-channel, got %s", msg["channel"].(string))
	}
	if msg["message"].(string) != "Message 1 from Redis" {
		t.Errorf("Expected message 'Message 1 from Redis', got %s", msg["message"].(string))
	}
	if msg["timestamp"] == nil {
		t.Fatalf("Expected timestamp to be set")
	}
	if msg["db"].(int) != 0 {
		t.Errorf("Expected db 0, got %d", msg["db"].(int))
	}

	// Test count = 3
	result, err = tool(ctx, struct {
		Channel string
		Count   int
	}{Channel: "my-channel", Count: 3})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	messages = result.([]interface{})
	if len(messages) != 3 {
		t.Fatalf("Expected 3 messages, got %d", len(messages))
	}
	// Check the third message.
	msg = messages[2].(map[string]interface{})
	if msg["message"].(string) != "Message 3 from Redis" {
		t.Errorf("Expected message 'Message 3 from Redis', got %s", msg["message"].(string))
	}
}

// Test with custom options.
func TestMessagingTools_WithOptions(t *testing.T) {
	t.Parallel()

	c := Messaging(
		WithHost("messaging-host"),
		WithPort(9999),
		WithVirtualHost("/test"),
		WithCredentials("user", "pass"),
		WithRedisDB(2),
	)
	if c.Host != "messaging-host" {
		t.Errorf("Expected host messaging-host, got %s", c.Host)
	}
	if c.Port != 9999 {
		t.Errorf("Expected port 9999, got %d", c.Port)
	}
	if c.VirtualHost != "/test" {
		t.Errorf("Expected virtual host /test, got %s", c.VirtualHost)
	}
	if c.Username != "user" || c.Password != "pass" {
		t.Errorf("Expected credentials not set")
	}
	if c.RedisDB != 2 {
		t.Errorf("Expected redis db 2, got %d", c.RedisDB)
	}
	// Check that the tools use the updated RedisDB.
	tool := c.RedisPublish().(func(context.Context, struct {
		Channel string `json:"channel"`
		Message string `json:"message"`
	}) (interface{}, error))

	ctx := context.Background()
	result, err := tool(ctx, struct {
		Channel string
		Message string
	}{Channel: "c", Message: "m"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	res := result.(map[string]interface{})
	if res["db"].(int) != 2 {
		t.Errorf("Expected redis db 2, got %d", res["db"].(int))
	}
}