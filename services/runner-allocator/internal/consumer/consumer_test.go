package consumer

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Aadithya-J/code_nest/services/runner-allocator/internal/store"
	"github.com/segmentio/kafka-go"
)

type mockProvisioner struct {
	provisionCalls   []string
	deprovisionCalls []string
	shouldFail       bool
}

func (m *mockProvisioner) ProvisionWorkspace(ctx context.Context, projectID, gitRepoURL string) error {
	m.provisionCalls = append(m.provisionCalls, projectID)
	if m.shouldFail {
		return &mockError{"provisioning failed"}
	}
	return nil
}

func (m *mockProvisioner) DeprovisionWorkspace(ctx context.Context, projectID string) error {
	m.deprovisionCalls = append(m.deprovisionCalls, projectID)
	return nil
}

type mockError struct {
	message string
}

func (e *mockError) Error() string {
	return e.message
}

func TestKafkaConsumer_ProcessMessage(t *testing.T) {
	inMemoryStore := store.NewInMemoryStore(2, 5)
	mockProv := &mockProvisioner{}
	consumer := &KafkaConsumer{
		store:       inMemoryStore,
		provisioner: mockProv,
	}
	ctx := context.Background()

	t.Run("WorkspaceCreateRequested_Success", func(t *testing.T) {
		event := WorkspaceEvent{
			EventType: "WORKSPACE_CREATE_REQUESTED",
			Timestamp: time.Now(),
			Payload: WorkspacePayload{
				ProjectID:  "test-project-1",
				GitRepoURL: "https://github.com/user/repo.git",
			},
		}

		eventData, _ := json.Marshal(event)
		msg := kafka.Message{
			Value: eventData,
		}

		err := consumer.processMessage(ctx, msg)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(mockProv.provisionCalls) != 1 {
			t.Errorf("Expected 1 provision call, got %d", len(mockProv.provisionCalls))
		}

		if mockProv.provisionCalls[0] != "test-project-1" {
			t.Errorf("Expected project ID 'test-project-1', got '%s'", mockProv.provisionCalls[0])
		}

		slot, err := inMemoryStore.GetSlotByProjectID(ctx, "test-project-1")
		if err != nil {
			t.Fatalf("Expected to find assigned slot: %v", err)
		}
		if !slot.IsBusy {
			t.Error("Slot should be marked as busy")
		}
	})

	t.Run("WorkspaceCreateRequested_NoSlots", func(t *testing.T) {
		inMemoryStore.AssignSlot(ctx, "slot-2", "existing-project")

		event := WorkspaceEvent{
			EventType: "WORKSPACE_CREATE_REQUESTED",
			Payload: WorkspacePayload{
				ProjectID: "queued-project",
			},
		}

		eventData, _ := json.Marshal(event)
		msg := kafka.Message{Value: eventData}

		err := consumer.processMessage(ctx, msg)
		if err != nil {
			t.Fatalf("Expected no error for queuing, got: %v", err)
		}

		queueLength, _ := inMemoryStore.GetQueueLength(ctx)
		if queueLength != 1 {
			t.Errorf("Expected queue length 1, got %d", queueLength)
		}
	})

	t.Run("WorkspaceReleaseRequested", func(t *testing.T) {
		event := WorkspaceEvent{
			EventType: "WORKSPACE_RELEASE_REQUESTED",
			Payload: WorkspacePayload{
				ProjectID: "test-project-1",
				SessionID: "session-test-1",
			},
		}

		eventData, _ := json.Marshal(event)
		msg := kafka.Message{Value: eventData}

		err := consumer.processMessage(ctx, msg)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(mockProv.deprovisionCalls) != 1 {
			t.Errorf("Expected 1 deprovision call, got %d", len(mockProv.deprovisionCalls))
		}

		_, err = inMemoryStore.GetSlotByProjectID(ctx, "test-project-1")
		if err != store.ErrNotFound {
			t.Error("Slot should be released after deletion")
		}
	})

	t.Run("UnknownEventType", func(t *testing.T) {
		event := WorkspaceEvent{
			EventType: "UNKNOWN_EVENT",
			Payload:   WorkspacePayload{ProjectID: "test"},
		}

		eventData, _ := json.Marshal(event)
		msg := kafka.Message{Value: eventData}

		err := consumer.processMessage(ctx, msg)
		if err != nil {
			t.Fatalf("Unknown events should not cause errors: %v", err)
		}
	})

	t.Run("ProvisioningFailure", func(t *testing.T) {
		mockProv.shouldFail = true
		event := WorkspaceEvent{
			EventType: "WORKSPACE_CREATE_REQUESTED",
			Payload: WorkspacePayload{
				ProjectID: "failing-project",
			},
		}

		eventData, _ := json.Marshal(event)
		msg := kafka.Message{Value: eventData}

		consumer.processMessage(ctx, msg)

		_, err := inMemoryStore.GetSlotByProjectID(ctx, "failing-project")
		if err != store.ErrNotFound {
			t.Error("Failed provision should release slot")
		}
	})
}
