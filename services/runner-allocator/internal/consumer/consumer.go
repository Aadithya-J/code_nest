package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/Aadithya-J/code_nest/services/runner-allocator/internal/models"
	"github.com/Aadithya-J/code_nest/services/runner-allocator/internal/store"
	"github.com/segmentio/kafka-go"
)

// Provisioner is an interface that defines the contract for managing workspace slots.
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

type WorkspaceEvent struct {
	EventType string           `json:"event_type"`
	Timestamp time.Time        `json:"timestamp"`
	Payload   WorkspacePayload `json:"payload"`
}

type WorkspacePayload struct {
	ProjectID  string `json:"project_id"`
	UserID     string `json:"user_id"`
	GitRepoURL string `json:"git_repo_url"`
	SessionID  string `json:"session_id"`
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

// KafkaConsumer listens to a Kafka topic and processes messages.
type KafkaConsumer struct {
	reader      *kafka.Reader
	store       store.Store // Use the interface type
	provisioner Provisioner
}

// NewKafkaConsumer creates a new Kafka consumer.
func NewKafkaConsumer(brokerURL, topic string, store store.Store, provisioner Provisioner) *KafkaConsumer {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  []string{brokerURL},
		Topic:    topic,
		GroupID:  "runner-allocator-group",
		MinBytes: 1,
		MaxBytes: 10e6,
	})

	return &KafkaConsumer{
		reader:      reader,
		store:       store,
		provisioner: provisioner,
	}
}

// NewKafkaConsumerWithReader creates a new Kafka consumer with a custom reader.
func NewKafkaConsumerWithReader(reader *kafka.Reader, store store.Store, provisioner Provisioner) *KafkaConsumer {
	return &KafkaConsumer{
		reader:      reader,
		store:       store,
		provisioner: provisioner,
	}
}

func (c *KafkaConsumer) Start(ctx context.Context) error {
	log.Println("Kafka consumer started")
	for {
		select {
		case <-ctx.Done():
			log.Println("Kafka consumer shutting down")
			return ctx.Err()
		default:
		}

		msg, err := c.reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Printf("Error reading message: %v", err)
			continue
		}

		if err := c.processMessage(ctx, msg); err != nil {
			log.Printf("Error processing message: %v", err)
		}
	}
}

func (c *KafkaConsumer) processMessage(ctx context.Context, msg kafka.Message) error {
	event := &WorkspaceEvent{}
	if err := json.Unmarshal(msg.Value, event); err != nil {
		return fmt.Errorf("failed to unmarshal event: %w", err)
	}

	switch event.EventType {
	case "WORKSPACE_CREATE_REQUESTED":
		return c.handleCreateRequest(ctx, event)
	case "WORKSPACE_RELEASE_REQUESTED":
		return c.handleReleaseRequest(ctx, event)
	case "WORKSPACE_PAUSE_REQUESTED":
		return c.handlePauseRequest(ctx, event)
	default:
		log.Printf("Unknown event type: %s", event.EventType)
		return nil
	}
}

func (c *KafkaConsumer) handleCreateRequest(ctx context.Context, event *WorkspaceEvent) error {
	slot, err := c.store.FindFreeSlot(ctx)
	if err != nil {
		queuedRequest := &store.QueuedRequest{
			ProjectID:  event.Payload.ProjectID,
			UserID:     event.Payload.UserID,
			GitRepoURL: event.Payload.GitRepoURL,
			SessionID:  event.Payload.SessionID,
		}
		position, queueErr := c.store.AddToQueue(ctx, queuedRequest)
		if queueErr != nil {
			return c.publishStatusEvent(ctx, event.Payload.ProjectID, event.Payload.SessionID, "FAILED", fmt.Sprintf("Queue full: %v", queueErr))
		}
		return c.publishStatusEvent(ctx, event.Payload.ProjectID, event.Payload.SessionID, "QUEUED", fmt.Sprintf("Position in queue: %d", position))
	}

	if err := c.store.AssignSlot(ctx, slot.ID, event.Payload.ProjectID); err != nil {
		return fmt.Errorf("failed to assign slot: %w", err)
	}

	c.publishStatusEvent(ctx, event.Payload.ProjectID, event.Payload.SessionID, "PROVISIONING", "Starting workspace")

	// Use SlotProvisioner if available (k3s), otherwise fallback to regular Provisioner (Docker)
	if slotProv, ok := c.provisioner.(SlotProvisioner); ok {
		// k3s slot-based provisioning
		assignment := &models.SlotAssignment{
			ProjectID:    event.Payload.ProjectID,
			SessionID:    event.Payload.SessionID,
			GitRepoURL:   event.Payload.GitRepoURL,
			GitHubToken:  "", // TODO: Add to payload
			RabbitMQURL:  "", // TODO: Add rabbitmqURL
			TargetBranch: "main",
		}
		if err := slotProv.AssignSlotToProject(ctx, slot.ID, assignment); err != nil {
			c.store.ReleaseSlot(ctx, event.Payload.ProjectID)
			return c.publishStatusEvent(ctx, event.Payload.ProjectID, event.Payload.SessionID, "FAILED", err.Error())
		}
	} else if c.provisioner != nil {
		// Legacy Docker provisioning (creates new containers)
		if err := c.provisioner.ProvisionWorkspace(ctx, event.Payload.ProjectID, event.Payload.GitRepoURL); err != nil {
			c.store.ReleaseSlot(ctx, event.Payload.ProjectID)
			return c.publishStatusEvent(ctx, event.Payload.ProjectID, event.Payload.SessionID, "FAILED", err.Error())
		}
	}

	return c.publishStatusEvent(ctx, event.Payload.ProjectID, event.Payload.SessionID, "READY", "Workspace ready")
}
func (c *KafkaConsumer) handleReleaseRequest(ctx context.Context, event *WorkspaceEvent) error {
	if c.provisioner != nil {
		c.provisioner.DeprovisionWorkspace(ctx, event.Payload.ProjectID)
	}

	if err := c.store.ReleaseSlot(ctx, event.Payload.ProjectID); err != nil {
		return fmt.Errorf("failed to release slot: %w", err)
	}

	c.processQueue(ctx)

	return nil
}

func (c *KafkaConsumer) handlePauseRequest(ctx context.Context, event *WorkspaceEvent) error {
	c.publishStatusEvent(ctx, event.Payload.ProjectID, event.Payload.SessionID, "PAUSING", "Pausing workspace as requested")

	slots, err := c.store.GetAllSlots(ctx)
	if err != nil {
		return c.publishStatusEvent(ctx, event.Payload.ProjectID, event.Payload.SessionID, "FAILED", "Failed to find workspace slot")
	}

	var targetSlot *store.Slot
	for _, slot := range slots {
		if slot.ProjectID == event.Payload.ProjectID {
			targetSlot = slot
			break
		}
	}

	if targetSlot == nil {
		return c.publishStatusEvent(ctx, event.Payload.ProjectID, event.Payload.SessionID, "FAILED", "Workspace not found")
	}

	if slotProv, ok := c.provisioner.(SlotProvisioner); ok {
		if err := slotProv.ReleaseSlot(ctx, targetSlot.ID); err != nil {
			return c.publishStatusEvent(ctx, event.Payload.ProjectID, event.Payload.SessionID, "FAILED", fmt.Sprintf("Failed to pause workspace: %v", err))
		}
	}

	if err := c.store.ReleaseSlot(ctx, event.Payload.ProjectID); err != nil {
		return c.publishStatusEvent(ctx, event.Payload.ProjectID, event.Payload.SessionID, "FAILED", fmt.Sprintf("Failed to release slot: %v", err))
	}

	c.processQueue(ctx)

	return c.publishStatusEvent(ctx, event.Payload.ProjectID, event.Payload.SessionID, "PAUSED", "Workspace paused successfully")
}
func (c *KafkaConsumer) processQueue(ctx context.Context) {
	nextRequest, err := c.store.GetNextFromQueue(ctx)
	if err != nil || nextRequest == nil {
		return
	}

	event := &WorkspaceEvent{
		EventType: "WORKSPACE_CREATE_REQUESTED",
		Payload: WorkspacePayload{
			ProjectID:  nextRequest.ProjectID,
			UserID:     nextRequest.UserID,
			GitRepoURL: nextRequest.GitRepoURL,
			SessionID:  nextRequest.SessionID,
		},
	}
	c.handleCreateRequest(ctx, event)
}

func (c *KafkaConsumer) publishStatusEvent(ctx context.Context, projectID, sessionID, status, message string) error {
	statusEvent := WorkspaceStatusEvent{
		EventType: "WORKSPACE_STATUS_UPDATED",
		Timestamp: time.Now().UTC(),
		Payload: WorkspaceStatusPayload{
			ProjectID: projectID,
			SessionID: sessionID,
			Status:    status,
			Message:   message,
		},
	}

	data, err := json.Marshal(statusEvent)
	if err != nil {
		return err
	}

	if c.reader == nil {
		log.Printf("Status event (no kafka): %s - %s: %s", projectID, status, message)
		return nil
	}

	writer := &kafka.Writer{
		Addr:  kafka.TCP(c.reader.Config().Brokers...),
		Topic: "workspace.status",
	}
	defer writer.Close()

	return writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(projectID),
		Value: data,
	})
}
