package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	ExchangeName = "workspace"
	ExchangeType = "topic"
	QueueStatus  = "api_gateway.workspace.status" // Unique queue for API Gateway
)

type WorkspaceStatusEvent struct {
	EventType string                 `json:"event_type"`
	Timestamp time.Time              `json:"timestamp"`
	Payload   WorkspaceStatusPayload `json:"payload"`
}

type WorkspaceStatusPayload struct {
	ProjectID string `json:"project_id"`
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
	Message   string `json:"message"`
}

type StatusHandler interface {
	HandleStatusUpdate(sessionID, status, message string)
}

type Consumer struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	handler StatusHandler
}

func NewConsumer(rabbitMQURL string, handler StatusHandler) (*Consumer, error) {
	conn, err := amqp.Dial(rabbitMQURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to open channel: %w", err)
	}

	// Declare exchange (idempotent)
	err = ch.ExchangeDeclare(
		ExchangeName,
		ExchangeType,
		true,  // durable
		false, // auto-deleted
		false, // internal
		false, // no-wait
		nil,
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to declare exchange: %w", err)
	}

	// Declare queue
	_, err = ch.QueueDeclare(
		QueueStatus,
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to declare queue: %w", err)
	}

	// Bind queue to exchange
	err = ch.QueueBind(
		QueueStatus,
		"status.updated", // routing key
		ExchangeName,
		false,
		nil,
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to bind queue: %w", err)
	}

	return &Consumer{
		conn:    conn,
		channel: ch,
		handler: handler,
	}, nil
}

func (c *Consumer) Start(ctx context.Context) error {
	msgs, err := c.channel.Consume(
		QueueStatus,
		"api-gateway", // consumer tag
		false,         // auto-ack
		false,         // exclusive
		false,         // no-local
		false,         // no-wait
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to register consumer: %w", err)
	}

	log.Println("🚀 API Gateway RabbitMQ consumer started")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-msgs:
			if !ok {
				return fmt.Errorf("message channel closed")
			}

			var event WorkspaceStatusEvent
			if err := json.Unmarshal(msg.Body, &event); err != nil {
				log.Printf("❌ Failed to unmarshal status event: %v", err)
				msg.Nack(false, false) // Don't requeue bad messages
				continue
			}

			c.handler.HandleStatusUpdate(event.Payload.SessionID, event.Payload.Status, event.Payload.Message)
			msg.Ack(false)
		}
	}
}

func (c *Consumer) Close() error {
	if c.channel != nil {
		c.channel.Close()
	}
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}
