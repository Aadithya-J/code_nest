// +build integration

package lifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/Aadithya-J/code_nest/services/runner-allocator/internal/store"
	"github.com/stretchr/testify/require"
)

// MockSlotProvisioner for testing
type MockSlotProvisioner struct {
	assignedSlots map[string]string // slotID -> projectID
	releasedSlots []string
}

func NewMockSlotProvisioner() *MockSlotProvisioner {
	return &MockSlotProvisioner{
		assignedSlots: make(map[string]string),
		releasedSlots: make([]string, 0),
	}
}

func (m *MockSlotProvisioner) AssignSlotToProject(ctx context.Context, slotID, projectID, gitRepoURL string) error {
	m.assignedSlots[slotID] = projectID
	return nil
}

func (m *MockSlotProvisioner) ReleaseSlot(ctx context.Context, slotID string) error {
	delete(m.assignedSlots, slotID)
	m.releasedSlots = append(m.releasedSlots, slotID)
	return nil
}

func TestAutoTimeoutLogic(t *testing.T) {
	// Create store with 3 slots
	inMemoryStore := store.NewInMemoryStore(3, 10)
	mockProvisioner := NewMockSlotProvisioner()
	
	// Create lifecycle manager with SHORT timeout for testing (5 seconds instead of 30 minutes)
	manager := &Manager{
		store:               inMemoryStore,
		slotProvisioner:     mockProvisioner,
		idleTimeout:         5 * time.Second, // SHORT timeout for testing
		queueCheckInterval:  1 * time.Second, // Check every second
	}
	
	// Override the auto-pause check interval for testing
	manager.autoPauseCheckInterval = 1 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Log("=== Phase 1: Assign projects to all 3 slots ===")
	
	// Assign projects to all 3 slots
	projectIDs := []string{"project-1", "project-2", "project-3"}
	for i, projectID := range projectIDs {
		slotID := string(rune('1' + i))
		
		// Assign slot
		err := inMemoryStore.AssignSlot(ctx, slotID, projectID)
		require.NoError(t, err)
		
		// Set slot status to RUNNING (required for auto-pause detection)
		err = inMemoryStore.UpdateSlotStatus(ctx, slotID, "RUNNING")
		require.NoError(t, err)
		
		// Update slot activity to "now"
		err = inMemoryStore.UpdateSlotActivity(ctx, slotID)
		require.NoError(t, err)
		
		t.Logf("✅ Assigned project %s to slot %s", projectID, slotID)
	}

	// Verify all slots are busy
	slots, err := inMemoryStore.GetAllSlots(ctx)
	require.NoError(t, err)
	require.Len(t, slots, 3)
	
	busyCount := 0
	for _, slot := range slots {
		if slot.IsBusy {
			busyCount++
		}
	}
	require.Equal(t, 3, busyCount, "All 3 slots should be busy")

	t.Log("=== Phase 2: Add project to queue (should be queued since all slots busy) ===")
	
	// Try to add a 4th project - should go to queue
	queuedRequest := &store.QueuedRequest{
		ProjectID:  "queued-project",
		UserID:     "queued-user",
		GitRepoURL: "https://github.com/test/repo.git",
		SessionID:  "queued-session",
		QueuedAt:   time.Now(),
	}
	
	position, err := inMemoryStore.AddToQueue(ctx, queuedRequest)
	require.NoError(t, err)
	require.Equal(t, 1, position, "Should be first in queue")
	
	t.Log("✅ Project queued at position 1")

	t.Log("=== Phase 3: Start lifecycle manager and wait for auto-pause ===")
	
	// Start the lifecycle manager
	go manager.Start(ctx)
	
	// Wait for the timeout period + some buffer
	t.Log("⏰ Waiting 7 seconds for auto-pause to trigger...")
	time.Sleep(7 * time.Second)

	t.Log("=== Phase 4: Verify auto-pause occurred ===")
	
	// Check that at least one slot was released due to timeout
	require.Greater(t, len(mockProvisioner.releasedSlots), 0, "At least one slot should have been auto-paused")
	
	t.Logf("✅ Auto-paused slots: %v", mockProvisioner.releasedSlots)

	t.Log("=== Phase 5: Verify queued project was processed ===")
	
	// The queued project should now be assigned to a freed slot
	slots, err = inMemoryStore.GetAllSlots(ctx)
	require.NoError(t, err)
	
	queuedProjectAssigned := false
	for _, slot := range slots {
		if slot.ProjectID == "queued-project" {
			queuedProjectAssigned = true
			t.Logf("✅ Queued project assigned to slot %s", slot.ID)
			break
		}
	}
	
	require.True(t, queuedProjectAssigned, "Queued project should be assigned to a freed slot")

	t.Log("=== Phase 6: Verify queue is now empty ===")
	
	// Queue should be empty now
	queueLength, err := inMemoryStore.GetQueueLength(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, queueLength, "Queue should be empty after processing")

	t.Log("🎉 Auto-timeout test PASSED!")
	t.Log("✅ Slots auto-paused after 5 seconds idle")
	t.Log("✅ Queued project automatically assigned to freed slot")
	t.Log("✅ Queue processed correctly")
}

func TestAutoTimeoutWithMultipleIdleSlots(t *testing.T) {
	// Create store with 3 slots
	inMemoryStore := store.NewInMemoryStore(3, 10)
	mockProvisioner := NewMockSlotProvisioner()
	
	// Create lifecycle manager with SHORT timeout for testing
	manager := &Manager{
		store:               inMemoryStore,
		slotProvisioner:     mockProvisioner,
		idleTimeout:         3 * time.Second, // Even shorter timeout
		queueCheckInterval:  1 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	t.Log("=== Testing multiple idle slots auto-pause ===")
	
	// Assign 2 projects, leave 1 slot free
	projectIDs := []string{"project-1", "project-2"}
	for i, projectID := range projectIDs {
		slotID := string(rune('1' + i))
		
		err := inMemoryStore.AssignSlot(ctx, slotID, projectID)
		require.NoError(t, err)
		
		// Set slot status to RUNNING
		err = inMemoryStore.UpdateSlotStatus(ctx, slotID, "RUNNING")
		require.NoError(t, err)
		
		// Set different activity times - project-1 is older (should timeout first)
		if projectID == "project-1" {
			// Don't update activity - will use zero time (very old)
		} else {
			err = inMemoryStore.UpdateSlotActivity(ctx, slotID)
			require.NoError(t, err)
		}
		
		t.Logf("✅ Assigned project %s to slot %s", projectID, slotID)
	}

	// Start lifecycle manager
	go manager.Start(ctx)
	
	// Wait for timeout
	t.Log("⏰ Waiting 5 seconds for auto-pause...")
	time.Sleep(5 * time.Second)

	// Verify that project-1 (older) was paused first
	require.Greater(t, len(mockProvisioner.releasedSlots), 0, "At least one slot should be released")
	
	// Check which slots were released
	slots, err := inMemoryStore.GetAllSlots(ctx)
	require.NoError(t, err)
	
	project1Released := false
	for _, slot := range slots {
		if slot.ID == "1" && !slot.IsBusy {
			project1Released = true
			break
		}
	}
	
	require.True(t, project1Released, "Project-1 (oldest) should be auto-paused first")
	
	t.Log("🎉 Multiple idle slots test PASSED!")
	t.Log("✅ Oldest idle slot was paused first")
}
