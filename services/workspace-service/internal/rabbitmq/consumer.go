package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Aadithya-J/code_nest/services/workspace-service/internal/repository"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	QueueStatusService = "workspace.status.service" // Unique queue for workspace-service
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

type Consumer struct {
	conn        *amqp.Connection
	channel     *amqp.Channel
	sessionRepo *repository.SessionRepository
	rabbitMQURL string
}

func NewConsumer(rabbitMQURL string, sessionRepo *repository.SessionRepository) (*Consumer, error) {
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
		QueueStatusService,
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

	// Bind queue to exchange for status updates
	err = ch.QueueBind(
		QueueStatusService,
		"status.updated",
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
		conn:        conn,
		channel:     ch,
		sessionRepo: sessionRepo,
		rabbitMQURL: rabbitMQURL,
	}, nil
}

func (c *Consumer) Start(ctx context.Context) error {
	msgs, err := c.channel.Consume(
		QueueStatusService,
		"workspace-service", // consumer tag
		false,               // auto-ack
		false,               // exclusive
		false,               // no-local
		false,               // no-wait
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to register consumer: %w", err)
	}

	log.Println("🚀 Workspace Service Consumer started, waiting for status updates...")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-msgs:
			if !ok {
				return fmt.Errorf("message channel closed")
			}

			if err := c.handleMessage(ctx, msg.Body); err != nil {
				log.Printf("❌ Error processing message: %v", err)
				msg.Nack(false, true) // Requeue
			} else {
				msg.Ack(false)
			}
		}
	}
}

func (c *Consumer) handleMessage(ctx context.Context, body []byte) error {
	var event WorkspaceStatusEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return fmt.Errorf("failed to unmarshal event: %w", err)
	}

	log.Printf("📨 Received status update: %s for session %s. Message: %s (project=%s)", event.Payload.Status, event.Payload.SessionID, event.Payload.Message, event.Payload.ProjectID)

	// If RUNNING, extract slot ID and update it BEFORE updating status to avoid race conditions
	// where the client sees RUNNING but SlotID is still nil.
	if event.Payload.Status == "RUNNING" {
		if strings.Contains(event.Payload.Message, "slot") {
			parts := strings.Split(event.Payload.Message, "slot ")
			if len(parts) > 1 {
				slotID := strings.TrimSpace(parts[1])
				// Ensure we store the full pod name if it's just a number
				if !strings.HasPrefix(slotID, "workspace-slot-") {
					slotID = fmt.Sprintf("workspace-slot-%s", slotID)
				}

				log.Printf("📝 Updating slot ID to %s for session %s (project=%s)", slotID, event.Payload.SessionID, event.Payload.ProjectID)
				if err := c.sessionRepo.UpdateSlot(ctx, event.Payload.SessionID, &slotID); err != nil {
					log.Printf("⚠️ Failed to update slot ID: %v", err)
					// Continue to update status anyway, but log the error
				}
			} else {
				log.Printf("⚠️ Could not parse slot ID from message: %s", event.Payload.Message)
			}
		} else {
			log.Printf("⚠️ Message does not contain 'slot': %s", event.Payload.Message)
		}
	}

	// Update session status in DB
	if err := c.sessionRepo.UpdateStatus(ctx, event.Payload.SessionID, event.Payload.Status, event.Payload.Message); err != nil {
		return fmt.Errorf("failed to update session status: %w", err)
	}

	return nil
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
