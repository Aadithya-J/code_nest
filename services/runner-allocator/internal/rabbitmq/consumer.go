package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/Aadithya-J/code_nest/services/runner-allocator/internal/models"
	"github.com/Aadithya-J/code_nest/services/runner-allocator/internal/store"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	ExchangeName  = "workspace"
	ExchangeType  = "topic"
	QueueRequests = "workspace.requests"
	QueueStatus   = "workspace.status"
)

// Provisioner interface for workspace operations
type Provisioner interface {
	ProvisionWorkspace(ctx context.Context, projectID, gitRepoURL string) error
	DeprovisionWorkspace(ctx context.Context, projectID string) error
}

// SlotProvisioner extends Provisioner with slot-based operations
type SlotProvisioner interface {
	Provisioner
	AssignSlotToProject(ctx context.Context, slotID string, assignment *models.SlotAssignment) error
	ReleaseSlot(ctx context.Context, slotID string) error
}

// TokenProvider interface for retrieving GitHub tokens
type TokenProvider interface {
	GetGitHubToken(ctx context.Context, userID string) (string, error)
}

type WorkspaceEvent struct {
	EventType string           `json:"event_type"`
	Timestamp time.Time        `json:"timestamp"`
	Payload   WorkspacePayload `json:"payload"`
}

type WorkspacePayload struct {
	ProjectID    string `json:"project_id"`
	UserID       string `json:"user_id"`
	GitRepoURL   string `json:"git_repo_url"`
	SessionID    string `json:"session_id"`
	GitHubToken  string `json:"github_token"`
	TargetBranch string `json:"target_branch"`
}

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

// Consumer processes workspace request messages from RabbitMQ
type Consumer struct {
	conn            *amqp.Connection
	channel         *amqp.Channel
	store           store.Store
	provisioner     Provisioner
	authClient      TokenProvider
	statusPublisher func(ctx context.Context, sessionID, projectID, status, message string)
	rabbitMQURL     string
}

// NewConsumer creates a new RabbitMQ consumer
func NewConsumer(rabbitMQURL string, store store.Store, provisioner Provisioner, authClient TokenProvider) (*Consumer, error) {
	conn, err := amqp.Dial(rabbitMQURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to open channel: %w", err)
	}

	// Declare exchange
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

	// Declare requests queue
	_, err = ch.QueueDeclare(
		QueueRequests,
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to declare requests queue: %w", err)
	}

	// Bind queue to exchange
	err = ch.QueueBind(
		QueueRequests,
		"*.requested", // routing key pattern
		ExchangeName,
		false,
		nil,
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to bind queue: %w", err)
	}

	// Set QoS - process one message at a time
	err = ch.Qos(
		1,     // prefetch count
		0,     // prefetch size
		false, // global
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("failed to set QoS: %w", err)
	}

	log.Println("✅ RabbitMQ consumer connected and queue bound")

	consumer := &Consumer{
		conn:        conn,
		channel:     ch,
		store:       store,
		provisioner: provisioner,
		authClient:  authClient,
		rabbitMQURL: rabbitMQURL,
	}

	consumer.statusPublisher = consumer.publishStatus

	return consumer, nil
}

// Start begins consuming messages from the queue
func (c *Consumer) Start(ctx context.Context) error {
	msgs, err := c.channel.Consume(
		QueueRequests,
		"runner-allocator", // consumer tag
		false,              // auto-ack (we'll manually ack)
		false,              // exclusive
		false,              // no-local
		false,              // no-wait
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to register consumer: %w", err)
	}

	log.Println("🚀 RabbitMQ consumer started, waiting for messages...")

	for {
		select {
		case <-ctx.Done():
			log.Println("RabbitMQ consumer shutting down")
			return ctx.Err()
		case msg, ok := <-msgs:
			if !ok {
				return fmt.Errorf("message channel closed")
			}

			if err := c.handleMessage(ctx, msg.Body); err != nil {
				log.Printf("❌ Error processing message: %v", err)
				// Negative acknowledgment - requeue the message
				msg.Nack(false, true)
			} else {
				// Acknowledge successful processing
				msg.Ack(false)
			}
		}
	}
}

func (c *Consumer) handleMessage(ctx context.Context, body []byte) error {
	var event WorkspaceEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return fmt.Errorf("failed to unmarshal event: %w", err)
	}

	log.Printf("📨 Received event: %s for project %s", event.EventType, event.Payload.ProjectID)

	switch event.EventType {
	case "WORKSPACE_CREATE_REQUESTED":
		return c.handleCreateRequest(ctx, &event)
	case "WORKSPACE_RELEASE_REQUESTED":
		return c.handleReleaseRequest(ctx, &event)
	case "WORKSPACE_PAUSE_REQUESTED":
		return c.handlePauseRequest(ctx, &event)
	default:
		log.Printf("⚠️  Unknown event type: %s", event.EventType)
		return nil // Don't requeue unknown events
	}
}

func (c *Consumer) handleCreateRequest(ctx context.Context, event *WorkspaceEvent) error {
	projectID := event.Payload.ProjectID
	gitRepoURL := event.Payload.GitRepoURL
	sessionID := event.Payload.SessionID
	githubToken := event.Payload.GitHubToken

	// Fetch GitHub token if missing
	if githubToken == "" && c.authClient != nil {
		token, err := c.authClient.GetGitHubToken(ctx, event.Payload.UserID)
		if err != nil {
			log.Printf("⚠️  Failed to get GitHub token for user %s: %v", event.Payload.UserID, err)
			// Continue without token (will fail to push but workspace might still work)
		} else {
			githubToken = token
			log.Printf("🔑 Retrieved GitHub token for user %s", event.Payload.UserID)
		}
	}

	// Try to find a free slot
	slot, err := c.store.FindFreeSlot(ctx)
	if err != nil {
		if errors.Is(err, store.ErrNoSlotsAvailable) {
			log.Printf("⏳ No slots available, queueing project %s", projectID)
			queuedReq := &store.QueuedRequest{
				ProjectID:    projectID,
				UserID:       event.Payload.UserID,
				GitRepoURL:   gitRepoURL,
				SessionID:    sessionID,
				GitHubToken:  githubToken,
				TargetBranch: event.Payload.TargetBranch,
				QueuedAt:     time.Now(),
			}
			if queuedReq.TargetBranch == "" {
				queuedReq.TargetBranch = "main"
			}
			if _, queueErr := c.store.AddToQueue(ctx, queuedReq); queueErr != nil {
				if errors.Is(queueErr, store.ErrQueueFull) {
					log.Printf("❌ Queue is full, rejecting project %s", projectID)
					c.emitStatus(ctx, sessionID, projectID, "FAILED", "Server busy: Request queue is full")
					return nil // Ack the message to stop the loop
				}
				return fmt.Errorf("failed to add to queue: %w", queueErr)
			}
			c.emitStatus(ctx, sessionID, projectID, "QUEUED", "No slots available, added to queue")
			return nil
		}
		return fmt.Errorf("failed to find free slot: %w", err)
	}

	// Assign the slot
	if err := c.store.AssignSlot(ctx, slot.ID, projectID); err != nil {
		return fmt.Errorf("failed to assign slot: %w", err)
	}

	log.Printf("✅ Assigned slot %s to project %s", slot.ID, projectID)

	// Provision workspace if using SlotProvisioner
	if slotProv, ok := c.provisioner.(SlotProvisioner); ok {
		assignment := &models.SlotAssignment{
			ProjectID:    projectID,
			SessionID:    sessionID,
			GitRepoURL:   gitRepoURL,
			GitHubToken:  githubToken,
			RabbitMQURL:  c.rabbitMQURL,
			TargetBranch: event.Payload.TargetBranch,
		}
		if assignment.TargetBranch == "" {
			assignment.TargetBranch = "main"
		}

		if err := slotProv.AssignSlotToProject(ctx, slot.ID, assignment); err != nil {
			c.store.ReleaseSlot(ctx, projectID)
			return fmt.Errorf("failed to provision workspace: %w", err)
		}
	}

	c.emitStatus(ctx, sessionID, projectID, "RUNNING", fmt.Sprintf("Workspace running in slot %s", slot.ID))
	return nil
}

func (c *Consumer) handleReleaseRequest(ctx context.Context, event *WorkspaceEvent) error {
	projectID := event.Payload.ProjectID

	// Find which slot has this project
	slots, err := c.store.GetAllSlots(ctx)
	if err != nil {
		return fmt.Errorf("failed to get slots: %w", err)
	}

	var slotID string
	for _, slot := range slots {
		if slot.ProjectID == projectID {
			slotID = slot.ID
			break
		}
	}

	if slotID == "" {
		// Project not in any slot, just acknowledge
		log.Printf("⚠️  Project %s not found in any slot (already released?)", projectID)
		c.emitStatus(ctx, event.Payload.SessionID, projectID, "RELEASED", "Workspace already released")
		return nil
	}

	// Deprovision workspace if using SlotProvisioner
	if slotProv, ok := c.provisioner.(SlotProvisioner); ok {
		if err := slotProv.ReleaseSlot(ctx, slotID); err != nil {
			log.Printf("⚠️  Warning: failed to release slot %s: %v", slotID, err)
		}
	}

	if err := c.store.ReleaseSlot(ctx, projectID); err != nil {
		return fmt.Errorf("failed to release slot: %w", err)
	}

	log.Printf("✅ Released slot %s for project %s", slotID, projectID)

	// Process next queued project if any
	nextReq, err := c.store.GetNextFromQueue(ctx)
	if err == nil && nextReq != nil {
		log.Printf("📋 Processing queued project %s from queue", nextReq.ProjectID)

		// Assign the newly freed slot to the queued project
		if err := c.store.AssignSlot(ctx, slotID, nextReq.ProjectID); err != nil {
			log.Printf("❌ Failed to assign slot to queued project: %v", err)
			return nil
		}

		if slotProv, ok := c.provisioner.(SlotProvisioner); ok {
			assignment := &models.SlotAssignment{
				ProjectID:    nextReq.ProjectID,
				SessionID:    nextReq.SessionID,
				GitRepoURL:   nextReq.GitRepoURL,
				GitHubToken:  nextReq.GitHubToken,
				RabbitMQURL:  c.rabbitMQURL,
				TargetBranch: "main", // TODO: Store this in QueuedRequest
			}

			if err := slotProv.AssignSlotToProject(ctx, slotID, assignment); err != nil {
				log.Printf("❌ Failed to provision queued project: %v", err)
				c.store.ReleaseSlot(ctx, nextReq.ProjectID)
			} else {
				c.emitStatus(ctx, nextReq.SessionID, nextReq.ProjectID, "RUNNING", fmt.Sprintf("Workspace running in slot %s", slotID))
			}
		} else {
			c.emitStatus(ctx, nextReq.SessionID, nextReq.ProjectID, "RUNNING", fmt.Sprintf("Workspace running in slot %s", slotID))
		}
	}

	c.emitStatus(ctx, event.Payload.SessionID, projectID, "RELEASED", "Workspace released")
	return nil
}

func (c *Consumer) handlePauseRequest(ctx context.Context, event *WorkspaceEvent) error {
	log.Printf("⏸️  Pause request for project %s (not implemented yet)", event.Payload.ProjectID)
	return nil
}

func (c *Consumer) publishStatus(ctx context.Context, sessionID, projectID, status, message string) {
	statusEvent := WorkspaceStatusEvent{
		EventType: "WORKSPACE_STATUS_UPDATED",
		Timestamp: time.Now().UTC(),
		Payload: WorkspaceStatusPayload{
			SessionID: sessionID,
			ProjectID: projectID,
			Status:    status,
			Message:   message,
		},
	}

	data, err := json.Marshal(statusEvent)
	if err != nil {
		log.Printf("❌ Failed to marshal status event: %v", err)
		return
	}

	err = c.channel.PublishWithContext(
		ctx,
		ExchangeName,
		"status.updated", // routing key
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         data,
			DeliveryMode: amqp.Persistent,
		},
	)
	if err != nil {
		log.Printf("❌ Failed to publish status: %v", err)
	}
}

func (c *Consumer) emitStatus(ctx context.Context, sessionID, projectID, status, message string) {
	if c.statusPublisher != nil {
		c.statusPublisher(ctx, sessionID, projectID, status, message)
		return
	}

	c.publishStatus(ctx, sessionID, projectID, status, message)
}

// Close closes the RabbitMQ connection
func (c *Consumer) Close() error {
	if c.channel != nil {
		if err := c.channel.Close(); err != nil {
			log.Printf("Error closing RabbitMQ channel: %v", err)
		}
	}
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// SetStatusPublisher allows tests to intercept status events without publishing to RabbitMQ.
func (c *Consumer) SetStatusPublisher(publisher func(ctx context.Context, sessionID, projectID, status, message string)) {
	c.statusPublisher = publisher
}
