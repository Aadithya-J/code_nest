package store

import (
	"context"
	"testing"
)

func TestInMemoryStore_SlotManagement(t *testing.T) {
	store := NewInMemoryStore(3, 10)
	ctx := context.Background()

	t.Run("FindFreeSlot", func(t *testing.T) {
		slot, err := store.FindFreeSlot(ctx)
		if err != nil {
			t.Fatalf("Expected to find free slot, got error: %v", err)
		}
		if slot == nil {
			t.Fatal("Expected slot, got nil")
		}
		if slot.IsBusy {
			t.Error("New slot should not be busy")
		}
	})

	t.Run("AssignSlot", func(t *testing.T) {
		slot, err := store.FindFreeSlot(ctx)
		if err != nil {
			t.Fatalf("Failed to locate free slot: %v", err)
		}

		if err := store.AssignSlot(ctx, slot.ID, "project-123"); err != nil {
			t.Fatalf("Failed to assign slot: %v", err)
		}

		assignedSlot, err := store.GetSlotByProjectID(ctx, "project-123")
		if err != nil {
			t.Fatalf("Failed to find assigned slot: %v", err)
		}
		if assignedSlot.ProjectID != "project-123" {
			t.Errorf("Expected project ID 'project-123', got '%s'", assignedSlot.ProjectID)
		}
		if !assignedSlot.IsBusy {
			t.Error("Assigned slot should be busy")
		}
	})

	t.Run("ReleaseSlot", func(t *testing.T) {
		slot, err := store.FindFreeSlot(ctx)
		if err != nil {
			t.Fatalf("Failed to locate free slot: %v", err)
		}

		if err := store.AssignSlot(ctx, slot.ID, "project-456"); err != nil {
			t.Fatalf("Failed to assign slot: %v", err)
		}

		if err := store.ReleaseSlot(ctx, "project-456"); err != nil {
			t.Fatalf("Failed to release slot: %v", err)
		}

		slots, _ := store.GetAllSlots(ctx)
		var releasedSlot *Slot
		for _, s := range slots {
			if s.ID == slot.ID {
				releasedSlot = s
				break
			}
		}

		if releasedSlot.IsBusy {
			t.Error("Released slot should not be busy")
		}
		if releasedSlot.ProjectID != "" {
			t.Error("Released slot should have empty project ID")
		}
	})
}

func TestInMemoryStore_QueueManagement(t *testing.T) {
	store := NewInMemoryStore(1, 3)
	ctx := context.Background()

	t.Run("AddToQueue", func(t *testing.T) {
		slot, err := store.FindFreeSlot(ctx)
		if err != nil {
			t.Fatalf("Failed to locate free slot: %v", err)
		}
		if err := store.AssignSlot(ctx, slot.ID, "busy-project"); err != nil {
			t.Fatalf("Failed to assign slot: %v", err)
		}

		req1 := &QueuedRequest{
			ProjectID:  "queued-project-1",
			UserID:     "user-1",
			GitRepoURL: "https://github.com/test/repo1.git",
			SessionID:  "session-1",
		}
		position, err := store.AddToQueue(ctx, req1)
		if err != nil {
			t.Fatalf("Failed to add to queue: %v", err)
		}
		if position != 1 {
			t.Errorf("Expected position 1, got %d", position)
		}

		req2 := &QueuedRequest{
			ProjectID:  "queued-project-2",
			UserID:     "user-2",
			GitRepoURL: "https://github.com/test/repo2.git",
			SessionID:  "session-2",
		}
		position, err = store.AddToQueue(ctx, req2)
		if err != nil {
			t.Fatalf("Failed to add to queue: %v", err)
		}
		if position != 2 {
			t.Errorf("Expected position 2, got %d", position)
		}
	})

	t.Run("GetNextInQueue", func(t *testing.T) {
		request, err := store.GetNextInQueue(ctx)
		if err != nil {
			t.Fatalf("Failed to get next in queue: %v", err)
		}
		if request.ProjectID != "queued-project-1" {
			t.Errorf("Expected 'queued-project-1', got '%s'", request.ProjectID)
		}

		length, _ := store.GetQueueLength(ctx)
		if length != 1 {
			t.Errorf("Expected queue length 1, got %d", length)
		}
	})

	t.Run("QueueFull", func(t *testing.T) {
		req3 := &QueuedRequest{ProjectID: "project-3", UserID: "user-3", SessionID: "session-3"}
		req4 := &QueuedRequest{ProjectID: "project-4", UserID: "user-4", SessionID: "session-4"}
		req5 := &QueuedRequest{ProjectID: "project-5", UserID: "user-5", SessionID: "session-5"}

		if _, err := store.AddToQueue(ctx, req3); err != nil {
			t.Fatalf("Unexpected error adding third queue entry: %v", err)
		}
		if _, err := store.AddToQueue(ctx, req4); err != nil {
			t.Fatalf("Unexpected error adding fourth queue entry: %v", err)
		}
		_, err := store.AddToQueue(ctx, req5)
		if err != ErrQueueFull {
			t.Errorf("Expected ErrQueueFull, got %v", err)
		}
	})

	t.Run("NoSlotsAvailable", func(t *testing.T) {
		_, err := store.FindFreeSlot(ctx)
		if err != ErrNoSlotsAvailable {
			t.Errorf("Expected ErrNoSlotsAvailable, got %v", err)
		}
	})
}
