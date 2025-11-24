package lifecycle

import (
	"context"
	"log"
	"time"

	"github.com/Aadithya-J/code_nest/services/runner-allocator/internal/models"
	"github.com/Aadithya-J/code_nest/services/runner-allocator/internal/store"
)

// Manager handles workspace lifecycle events like auto-pause and queue processing
type Manager struct {
	store                  store.Store
	slotProvisioner        SlotProvisioner
	idleTimeout            time.Duration
	queueCheckInterval     time.Duration
	autoPauseCheckInterval time.Duration
}

// SlotProvisioner interface for slot operations
type SlotProvisioner interface {
	AssignSlotToProject(ctx context.Context, slotID string, assignment *models.SlotAssignment) error
	ReleaseSlot(ctx context.Context, slotID string) error
}

// NewManager creates a new lifecycle manager
func NewManager(store store.Store, provisioner SlotProvisioner) *Manager {
	return &Manager{
		store:                  store,
		slotProvisioner:        provisioner,
		idleTimeout:            30 * time.Minute, // Auto-pause after 30 minutes
		queueCheckInterval:     10 * time.Second, // Check queue every 10 seconds
		autoPauseCheckInterval: 1 * time.Minute,  // Check for idle workspaces every minute
	}
}

// Start begins the lifecycle management background processes
func (m *Manager) Start(ctx context.Context) {
	log.Println("Starting lifecycle manager...")

	// Start auto-pause monitor
	go m.autoPauseMonitor(ctx)

	// Start queue processor
	go m.queueProcessor(ctx)

	log.Println("Lifecycle manager started")
}

// autoPauseMonitor checks for idle workspaces and pauses them
func (m *Manager) autoPauseMonitor(ctx context.Context) {
	ticker := time.NewTicker(m.autoPauseCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.checkIdleWorkspaces(ctx)
		}
	}
}

func (m *Manager) checkIdleWorkspaces(ctx context.Context) {
	slots, err := m.store.GetAllSlots(ctx)
	if err != nil {
		log.Printf("Error getting slots for idle check: %v", err)
		return
	}

	for _, slot := range slots {
		if slot.ProjectID != "" && slot.Status == "RUNNING" {
			idleDuration := time.Since(slot.LastActivity)

			if idleDuration > m.idleTimeout {
				log.Printf("Auto-pausing idle workspace in slot %s (idle for %v)", slot.ID, idleDuration)

				if err := m.pauseWorkspace(ctx, slot); err != nil {
					log.Printf("Error pausing workspace in slot %s: %v", slot.ID, err)
				}
			}
		}
	}
}

// pauseWorkspace pauses a workspace and frees up the slot
func (m *Manager) pauseWorkspace(ctx context.Context, slot *store.Slot) error {
	// Update slot status to PAUSED
	if err := m.store.UpdateSlotStatus(ctx, slot.ID, "PAUSED"); err != nil {
		return err
	}

	// Release the slot in k3s (this will stop the pod but preserve the PV)
	if err := m.slotProvisioner.ReleaseSlot(ctx, slot.ID); err != nil {
		return err
	}

	// Release the slot in the store (makes it available for new assignments)
	if err := m.store.ReleaseSlot(ctx, slot.ProjectID); err != nil {
		return err
	}

	log.Printf("✅ Paused workspace %s in slot %s", slot.ProjectID, slot.ID)
	return nil
}

// queueProcessor processes the queue when slots become available
func (m *Manager) queueProcessor(ctx context.Context) {
	ticker := time.NewTicker(m.queueCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.processQueue(ctx)
		}
	}
}

// processQueue assigns queued requests to available slots
func (m *Manager) processQueue(ctx context.Context) {
	// Check if there are free slots
	freeSlot, err := m.store.FindFreeSlot(ctx)
	if err != nil {
		// No free slots available
		return
	}

	// Get next request from queue
	queuedRequest, err := m.store.GetNextFromQueue(ctx)
	if err != nil || queuedRequest == nil {
		// Queue is empty or error occurred
		return
	}

	log.Printf("Processing queued request for project %s (user %s)", queuedRequest.ProjectID, queuedRequest.UserID)

	// Assign the slot
	if err := m.store.AssignSlot(ctx, freeSlot.ID, queuedRequest.ProjectID); err != nil {
		log.Printf("Error assigning slot %s to project %s: %v", freeSlot.ID, queuedRequest.ProjectID, err)
		return
	}

	// Provision the workspace
	assignment := &models.SlotAssignment{
		ProjectID:    queuedRequest.ProjectID,
		SessionID:    queuedRequest.SessionID,
		GitRepoURL:   queuedRequest.GitRepoURL,
		GitHubToken:  queuedRequest.GitHubToken,
		RabbitMQURL:  "", // TODO: Pass this from manager
		TargetBranch: queuedRequest.TargetBranch,
	}
	if assignment.TargetBranch == "" {
		assignment.TargetBranch = "main"
	}

	if err := m.slotProvisioner.AssignSlotToProject(ctx, freeSlot.ID, assignment); err != nil {
		log.Printf("Error provisioning workspace for project %s: %v", queuedRequest.ProjectID, err)
		// Release the slot on error
		m.store.ReleaseSlot(ctx, queuedRequest.ProjectID)
		return
	}

	log.Printf("✅ Assigned queued project %s to slot %s", queuedRequest.ProjectID, freeSlot.ID)
}
