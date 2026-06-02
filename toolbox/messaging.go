package toolbox

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AKhilRaghav0/hamr"
)

// MessagingTools provides tools for interacting with messaging systems (RabbitMQ, Redis Pub/Sub).
type MessagingTools struct {
	// Host is the messaging server host.
	Host string
	// Port is the port for the host.
	Port int
	// VirtualHost is the RabbitMQ virtual host (optional).
	VirtualHost string
	// Password is the password for authentication (optional).
	Password string
	// Username is the username for authentication (optional).
	Username string
	// RedisDB is the Redis database number (optional).
	RedisDB int
}

// MessagingOption is a functional option for configuring the MessagingTools.
type MessagingOption func(*MessagingTools)

// WithHost sets the host for the messaging server.
func WithHost(host string) MessagingOption {
	return func(c *MessagingTools) {
		c.Host = host
	}
}

// WithPort sets the port for the host.
func WithPort(port int) MessagingOption {
	return func(c *MessagingTools) {
		c.Port = port
	}
}

// WithVirtualHost sets the RabbitMQ virtual host.
func WithVirtualHost(vhost string) MessagingOption {
	return func(c *MessagingTools) {
		c.VirtualHost = vhost
	}
}

// WithCredentials sets the username and password for authentication.
func WithCredentials(username, password string) MessagingOption {
	return func(c *MessagingTools) {
		c.Username = username
		c.Password = password
	}
}

// WithRedisDB sets the Redis database number.
func WithRedisDB(db int) MessagingOption {
	return func(c *MessagingTools) {
		c.RedisDB = db
	}
}

// Messaging returns a new MessagingTools instance with the given options.
func Messaging(opts ...MessagingOption) *MessagingTools {
	c := &MessagingTools{
		Host:     "localhost",
		Port:     5672, // Default RabbitMQ port
		VirtualHost: "/",
		Username: "guest",
		Password: "guest",
		RedisDB:  0,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// RabbitMQPublish returns a tool handler that simulates publishing a message to a RabbitMQ exchange.
func (c *MessagingTools) RabbitMQPublish() interface{} {
	return func(ctx context.Context, args struct {
		Exchange   string `json:"exchange" desc:"Exchange name" required:"true"`
		RoutingKey string `json:"routing_key" desc:"Routing key" required:"true"`
		Body       string `json:"body" desc:"Message body" required:"true"`
	}) (interface{}, error) {
		if args.Exchange == "" || args.RoutingKey == "" || args.Body == "" {
			return nil, errors.New("exchange, routing key, and body are required")
		}
		// Simulate publishing the message.
		return map[string]interface{}{
			"exchange":   args.Exchange,
			"routing_key": args.RoutingKey,
			"body":       args.Body,
			"timestamp":  time.Now().Unix(),
		}, nil
	}
}

// RabbitMQConsume returns a tool handler that simulates consuming a message from a RabbitMQ queue.
// Note: In a real implementation, this would be a long-running operation. Here we just return a mock message.
func (c *MessagingTools) RabbitMQConsume() interface{} {
	return func(ctx context.Context, args struct {
		Queue   string `json:"queue" desc:"Queue name" required:"true"`
		AutoAck bool   `json:"auto_ack" desc:"Automatically acknowledge messages (default: false)"`)
	}) (interface{}, error) {
		if args.Queue == "" {
			return nil, errors.New("queue name is required")
		}
		// Simulate consuming a message.
		return map[string]interface{}{
			"queue":   args.Queue,
			"body":    "Hello from RabbitMQ",
			"timestamp": time.Now().Unix(),
			"auto_ack": args.AutoAck,
		}, nil
	}
}

// RedisPublish returns a tool handler that simulates publishing a message to a Redis channel.
func (c *MessagingTools) RedisPublish() interface{} {
	return func(ctx context.Context, args struct {
		Channel string `json:"channel" desc:"Channel name" required:"true"`
		Message string `json:"message" desc:"Message to publish" required:"true"`
	}) (interface{}, error) {
		if args.Channel == "" || args.Message == "" {
			return nil, errors.New("channel and message are required")
		}
		// Simulate publishing the message.
		return map[string]interface{}{
			"channel":   args.Channel,
			"message":   args.Message,
			"timestamp": time.Now().Unix(),
			"db":        c.RedisDB,
		}, nil
	}
}

// RedisSubscribe returns a tool handler that simulates subscribing to a Redis channel.
// Note: In a real implementation, this would be a long-running operation. Here we just return a mock message.
func (c *MessagingTools) RedisSubscribe() interface{} {
	return func(ctx context.Context, args struct {
		Channel string `json:"channel" desc:"Channel name" required:"true"`
		Count   int    `json:"count" desc:"Number of messages to receive (default: 1)"`)
	}) (interface{}, error) {
		if args.Channel == "" {
			return nil, errors.New("channel name is required")
		}
		if args.Count <= 0 {
			args.Count = 1
		}
		// Simulate receiving messages.
		messages := []map[string]interface{}{}
		for i := 0; i < args.Count; i++ {
			messages = append(messages, map[string]interface{}{
				"channel":   args.Channel,
				"message":   fmt.Sprintf("Message %d from Redis", i+1),
				"timestamp": time.Now().Unix(),
				"db":        c.RedisDB,
			})
		}
		return messages, nil
	}
}