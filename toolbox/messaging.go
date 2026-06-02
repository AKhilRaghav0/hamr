package toolbox

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/AKhilRaghav0/hamr"
	"github.com/redis/go-redis/v9"
)

const (
	defaultMessagingTimeout = 30 * time.Second
	defaultRedisMaxMessages = 100
)

// defaultRedisAddr is the default Redis server address.
const defaultRedisAddr = "localhost:6379"

// MessagingConfig holds configuration for messaging tools.
type MessagingConfig struct {
	timeout      time.Duration
	redisAddr    string
	redisPassword string
}

// MessagingOption is a functional option for MessagingTools.
type MessagingOption func(*MessagingConfig)

// WithMessagingTimeout sets the messaging API timeout. Default is 30 seconds.
func WithMessagingTimeout(d time.Duration) MessagingOption {
	return func(c *MessagingConfig) {
		c.timeout = d
	}
}

// WithRedisAddr sets the Redis server address.
func WithRedisAddr(addr string) MessagingOption {
	return func(c *MessagingConfig) {
		c.redisAddr = addr
	}
}

// WithRedisPassword sets the Redis password.
func WithRedisPassword(password string) MessagingOption {
	return func(c *MessagingConfig) {
		c.redisPassword = password
	}
}

// MessagingTools is a collection of messaging service tools.
// It provides basic Redis Pub/Sub and RabbitMQ operations.
type MessagingTools struct {
	redisClient *redis.Client
	cfg       MessagingConfig
}

// Messaging returns a MessagingTools collection with the given options applied.
func Messaging(opts ...MessagingOption) *MessagingTools {
	cfg := MessagingConfig{
		timeout:  defaultMessagingTimeout,
		redisAddr: defaultRedisAddr,
	}
	for _, o := range opts {
		o(&cfg)
	}

	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.redisAddr,
		Password: cfg.redisPassword,
	})

	return &MessagingTools{
		redisClient: redisClient,
		cfg:         cfg,
	}
}

// Tools implements hamr.ToolCollection.
func (m *MessagingTools) Tools() []hamr.ToolInfo {
	return []hamr.ToolInfo{
		// Redis Pub/Sub tools
		{
			Name:        "redis_publish",
			Description: "Publish a message to a Redis channel.",
			Handler:     m.redisPublish,
		},
		{
			Name:        "redis_subscribe",
			Description: "Subscribe to a Redis channel and retrieve messages.",
			Handler:     m.redisSubscribe,
		},
		// RabbitMQ tools
		{
			Name:        "rabbitmq_publish",
			Description: "Publish a message to a RabbitMQ exchange.",
			Handler:     m.rabbitmqPublish,
		},
		{
			Name:        "rabbitmq_consume",
			Description: "Consume messages from a RabbitMQ queue.",
			Handler:     m.rabbitmqConsume,
		},
	}
}

// RedisPublishInput is the input for the redis_publish tool.
type RedisPublishInput struct {
	Channel string `json:"channel" desc:"Redis channel to publish to"`
	Message string `json:"message" desc:"Message to publish"`
}

// RedisSubscribeInput is the input for the redis_subscribe tool.
type RedisSubscribeInput struct {
	Channel   string `json:"channel" desc:"Redis channel to subscribe to"`
	Count     int    `json:"count" desc:"maximum number of messages to receive" optional:"true"`
	TimeoutMs int    `json:"timeout_ms" desc:"timeout in milliseconds to wait for messages" optional:"true"`
}

// RabbitMQPublishInput is the input for the rabbitmq_publish tool.
type RabbitMQPublishInput struct {
	URL      string `json:"url" desc:"RabbitMQ connection URL (amqp://...)"`
	Exchange string `json:"exchange" desc:"Exchange name to publish to"`
	RoutingKey string `json:"routing_key" desc:"Routing key for the message"`
	Message  string `json:"message" desc:"Message to publish"`
}

// RabbitMQConsumeInput is the input for the rabbitmq_consume tool.
type RabbitMQConsumeInput struct {
	URL    string `json:"url" desc:"RabbitMQ connection URL (amqp://...)"`
	Queue  string `json:"queue" desc:"Queue name to consume from"`
	Count  int    `json:"count" desc:"maximum number of messages to receive" optional:"true"`
}

// redisPublish publishes a message to a Redis channel.
func (m *MessagingTools) redisPublish(ctx context.Context, in RedisPublishInput) (string, error) {
	if in.Channel == "" {
		return "", fmt.Errorf("redis_publish: channel must not be empty")
	}

	ctx, cancel := context.WithTimeout(ctx, m.cfg.timeout)
	defer cancel()

	n, err := m.redisClient.Publish(ctx, in.Channel, in.Message).Result()
	if err != nil {
		return "", fmt.Errorf("redis_publish: %w", err)
	}

	return fmt.Sprintf("published to %d subscriber(s) on channel %s", n, in.Channel), nil
}

// redisSubscribe reads messages from a Redis channel.
func (m *MessagingTools) redisSubscribe(ctx context.Context, in RedisSubscribeInput) (string, error) {
	if in.Channel == "" {
		return "", fmt.Errorf("redis_subscribe: channel must not be empty")
	}

	count := in.Count
	if count <= 0 {
		count = defaultRedisMaxMessages
	}

	timeout := m.cfg.timeout
	if in.TimeoutMs > 0 {
		timeout = time.Duration(in.TimeoutMs) * time.Millisecond
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	sub := m.redisClient.Subscribe(ctx, in.Channel)
	defer sub.Close()

	ch := sub.Channel()
	messages := make([]string, 0, count)
	received := 0

	for {
		select {
		case msg := <-ch:
			if msg.Payload != "" {
				messages = append(messages, msg.Payload)
				received++
				if received >= count {
					goto done
				}
			}
		case <-ctx.Done():
			goto done
		}
	}
done:

	if len(messages) == 0 {
		return "no messages received", nil
	}

	result := make([]string, len(messages))
	for i, msg := range messages {
		result[i] = msg
	}
	return fmt.Sprintf("received %d message(s):\n%s", len(messages), joinMessages(result)), nil
}

// rabbitmqPublish publishes a message to a RabbitMQ exchange.
func (m *MessagingTools) rabbitmqPublish(ctx context.Context, in RabbitMQPublishInput) (string, error) {
	if in.URL == "" || in.Exchange == "" || in.Message == "" {
		return "", fmt.Errorf("rabbitmq_publish: url, exchange, and message must not be empty")
	}

	// Security: validate URL scheme
	if !isAMQPURL(in.URL) {
		return "", fmt.Errorf("rabbitmq_publish: invalid URL scheme, expected amqp://")
	}

	// Note: Full RabbitMQ implementation would use streadway/amqp or similar
	// For now, return not implemented with guidance
	return "rabbitmq_publish: RabbitMQ client not configured; add 'github.com/streadway/amqp' to use this tool", nil
}

// rabbitmqConsume consumes messages from a RabbitMQ queue.
func (m *MessagingTools) rabbitmqConsume(ctx context.Context, in RabbitMQConsumeInput) (string, error) {
	if in.URL == "" || in.Queue == "" {
		return "", fmt.Errorf("rabbitmq_consume: url and queue must not be empty")
	}

	// Security: validate URL scheme
	if !isAMQPURL(in.URL) {
		return "", fmt.Errorf("rabbitmq_consume: invalid URL scheme, expected amqp://")
	}

	// Note: Full RabbitMQ implementation would use streadway/amqp or similar
	return "rabbitmq_consume: RabbitMQ client not configured; add 'github.com/streadway/amqp' to use this tool", nil
}

// isAMQPURL validates that a URL uses the amqp scheme.
func isAMQPURL(url string) bool {
	return len(url) >= 7 && (url[:7] == "amqp://" || url[:8] == "amqps://")
}

// joinMessages joins messages with newlines.
func joinMessages(msgs []string) string {
	return strings.Join(msgs, "\n")
}