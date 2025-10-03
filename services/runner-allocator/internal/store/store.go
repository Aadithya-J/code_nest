package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrQueueFull      = errors.New("queue is full")
	ErrNoSlotsAvailable = errors.New("no slots available")
	ErrNotFound       = errors.New("not found")
)

// Slot represents a workspace slot.
type Slot struct {
	ID           string
	ProjectID    string
	IsBusy       bool
	Status       string    // RUNNING, PAUSED, etc.
	LastActivity time.Time // For idle detection
}

// QueuedRequest represents a complete workspace request waiting in queue
type QueuedRequest struct {
	ProjectID  string    `json:"project_id"`
	UserID     string    `json:"user_id"`
	GitRepoURL string    `json:"git_repo_url"`
	SessionID  string    `json:"session_id"`
	QueuedAt   time.Time `json:"queued_at"`
}

// Store defines the interface for managing workspace slots and queue.
type Store interface {
	FindFreeSlot(ctx context.Context) (*Slot, error)
	AssignSlot(ctx context.Context, slotID, projectID string) error
	ReleaseSlot(ctx context.Context, projectID string) error
	AddToQueue(ctx context.Context, request *QueuedRequest) (int, error)
	GetNextFromQueue(ctx context.Context) (*QueuedRequest, error)
	GetAllSlots(ctx context.Context) ([]*Slot, error)
	UpdateSlotStatus(ctx context.Context, slotID, status string) error
	UpdateSlotActivity(ctx context.Context, slotID string) error
}

// InMemoryStore is an in-memory implementation of the Store interface.
// It is thread-safe.
type InMemoryStore struct {
	mu        sync.Mutex
	slots     map[string]*Slot
	queue     []*QueuedRequest
	queueSize int
}

// NewInMemoryStore creates a new thread-safe, in-memory store.
func NewInMemoryStore(maxSlots, queueSize int) *InMemoryStore {
	s := &InMemoryStore{
		slots:     make(map[string]*Slot),
		queue:     make([]*QueuedRequest, 0, queueSize),
		queueSize: queueSize,
	}
	for i := 1; i <= maxSlots; i++ {
		slotID := fmt.Sprintf("%d", i)
		s.slots[slotID] = &Slot{ID: slotID}
	}
	return s
}

// FindFreeSlot finds an available slot.
func (s *InMemoryStore) FindFreeSlot(ctx context.Context) (*Slot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, slot := range s.slots {
		if !slot.IsBusy {
			return slot, nil
		}
	}
	return nil, ErrNoSlotsAvailable
}

// AssignSlot marks a slot as busy with a given project.
func (s *InMemoryStore) AssignSlot(ctx context.Context, slotID, projectID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if slot, ok := s.slots[slotID]; ok {
		if !slot.IsBusy {
			slot.IsBusy = true
			slot.ProjectID = projectID
			slot.Status = "RUNNING"
			slot.LastActivity = time.Now()
			return nil
		}
	}
	return ErrNotFound
}

// ReleaseSlot marks a slot as free.
func (s *InMemoryStore) ReleaseSlot(ctx context.Context, projectID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, slot := range s.slots {
		if slot.ProjectID == projectID {
			slot.ProjectID = ""
			slot.IsBusy = false
			slot.Status = ""
			slot.LastActivity = time.Time{}
			return nil
		}
	}
	return ErrNotFound
}

// GetSlotByProjectID finds a slot by the project ID it is running.
func (s *InMemoryStore) GetSlotByProjectID(ctx context.Context, projectID string) (*Slot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, slot := range s.slots {
		if slot.IsBusy && slot.ProjectID == projectID {
			return slot, nil
		}
	}
	return nil, ErrNotFound
}

// AddToQueue adds a workspace request to the queue.
func (s *InMemoryStore) AddToQueue(ctx context.Context, request *QueuedRequest) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.queue) >= s.queueSize {
		return 0, ErrQueueFull
	}
	request.QueuedAt = time.Now()
	s.queue = append(s.queue, request)
	return len(s.queue), nil
}

// GetNextInQueue retrieves and removes the next workspace request from the queue.
func (s *InMemoryStore) GetNextInQueue(ctx context.Context) (*QueuedRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.queue) == 0 {
		return nil, nil // No error, just empty
	}

	request := s.queue[0]
	s.queue = s.queue[1:]
	return request, nil
}

// GetNextFromQueue gets the next request from queue (alias for GetNextInQueue)
func (s *InMemoryStore) GetNextFromQueue(ctx context.Context) (*QueuedRequest, error) {
	return s.GetNextInQueue(ctx)
}

// GetAllSlots returns all slots
func (s *InMemoryStore) GetAllSlots(ctx context.Context) ([]*Slot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	slots := make([]*Slot, 0, len(s.slots))
	for _, slot := range s.slots {
		slots = append(slots, slot)
	}
	return slots, nil
}

// UpdateSlotStatus updates the status of a slot
func (s *InMemoryStore) UpdateSlotStatus(ctx context.Context, slotID, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	slot, exists := s.slots[slotID]
	if !exists {
		return ErrNotFound
	}
	
	slot.Status = status
	return nil
}

// UpdateSlotActivity updates the last activity time of a slot
func (s *InMemoryStore) UpdateSlotActivity(ctx context.Context, slotID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	slot, exists := s.slots[slotID]
	if !exists {
		return ErrNotFound
	}
	
	slot.LastActivity = time.Now()
	return nil
}

// GetQueueLength returns the current number of items in the queue.
func (s *InMemoryStore) GetQueueLength(ctx context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.queue), nil
}
