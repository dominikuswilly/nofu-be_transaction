package rabbitmq

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Client struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	url     string
}

// NewClient creates a new RabbitMQ client and establishes connection
func NewClient(url string) (*Client, error) {
	slog.Info("Connecting to RabbitMQ", "url", maskPassword(url))

	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	channel, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to open channel: %w", err)
	}

	slog.Info("Successfully connected to RabbitMQ")

	return &Client{
		conn:    conn,
		channel: channel,
		url:     url,
	}, nil
}

// DeclareQueue declares a queue (creates it if it doesn't exist)
func (c *Client) DeclareQueue(queueName string) error {
	_, err := c.channel.QueueDeclare(
		queueName, // name
		true,      // durable
		false,     // delete when unused
		false,     // exclusive
		false,     // no-wait
		nil,       // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare queue: %w", err)
	}

	slog.Info("Queue declared successfully", "queue", queueName)
	return nil
}

// Consume starts consuming messages from a queue
func (c *Client) Consume(ctx context.Context, queueName string, handler func([]byte) error) error {
	// Set QoS to process one message at a time
	err := c.channel.Qos(
		1,     // prefetch count
		0,     // prefetch size
		false, // global
	)
	if err != nil {
		return fmt.Errorf("failed to set QoS: %w", err)
	}

	msgs, err := c.channel.Consume(
		queueName, // queue
		"",        // consumer tag
		false,     // auto-ack
		false,     // exclusive
		false,     // no-local
		false,     // no-wait
		nil,       // args
	)
	if err != nil {
		return fmt.Errorf("failed to register consumer: %w", err)
	}

	slog.Info("Started consuming messages", "queue", queueName)

	go func() {
		for {
			select {
			case <-ctx.Done():
				slog.Info("Consumer context cancelled, stopping...")
				return
			case msg, ok := <-msgs:
				if !ok {
					slog.Warn("Message channel closed, attempting to reconnect...")
					// Attempt to reconnect
					time.Sleep(5 * time.Second)
					if err := c.reconnect(); err != nil {
						slog.Error("Failed to reconnect", "error", err)
						return
					}
					return
				}

				slog.Info("Received message", "body_length", len(msg.Body))

				// Process message
				if err := handler(msg.Body); err != nil {
					slog.Error("Failed to process message", "error", err)
					// Reject and requeue the message
					if err := msg.Nack(false, true); err != nil {
						slog.Error("Failed to nack message", "error", err)
					}
				} else {
					// Acknowledge the message
					if err := msg.Ack(false); err != nil {
						slog.Error("Failed to ack message", "error", err)
					}
				}
			}
		}
	}()

	return nil
}

// reconnect attempts to reconnect to RabbitMQ
func (c *Client) reconnect() error {
	slog.Info("Attempting to reconnect to RabbitMQ")

	conn, err := amqp.Dial(c.url)
	if err != nil {
		return fmt.Errorf("failed to reconnect: %w", err)
	}

	channel, err := conn.Channel()
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to open channel on reconnect: %w", err)
	}

	// Close old connection if exists
	if c.conn != nil && !c.conn.IsClosed() {
		c.conn.Close()
	}

	c.conn = conn
	c.channel = channel

	slog.Info("Reconnected to RabbitMQ successfully")
	return nil
}

// Close closes the RabbitMQ connection
func (c *Client) Close() error {
	slog.Info("Closing RabbitMQ connection")

	if c.channel != nil {
		if err := c.channel.Close(); err != nil {
			slog.Error("Failed to close channel", "error", err)
		}
	}

	if c.conn != nil && !c.conn.IsClosed() {
		if err := c.conn.Close(); err != nil {
			return fmt.Errorf("failed to close connection: %w", err)
		}
	}

	slog.Info("RabbitMQ connection closed")
	return nil
}

// maskPassword masks the password in the URL for logging
func maskPassword(url string) string {
	// Simple masking - in production, use a proper URL parser
	return "amqp://***:***@..."
}
