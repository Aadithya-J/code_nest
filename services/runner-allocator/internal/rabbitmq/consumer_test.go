package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Aadithya-J/code_nest/services/runner-allocator/internal/models"
	"github.com/Aadithya-J/code_nest/services/runner-allocator/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type assignCall struct {
	slotID    string
	projectID string
	gitRepo   string
	token     string
}

type mockSlotProvisioner struct {
	assignCalls  []assignCall
	releaseCalls []string
	provisionErr error
	assignErr    error
	releaseErr   error
}

func (m *mockSlotProvisioner) ProvisionWorkspace(ctx context.Context, projectID, gitRepoURL string) error {
	return m.provisionErr
}

func (m *mockSlotProvisioner) DeprovisionWorkspace(ctx context.Context, projectID string) error {
	return nil
}

func (m *mockSlotProvisioner) AssignSlotToProject(ctx context.Context, slotID string, assignment *models.SlotAssignment) error {
	m.assignCalls = append(m.assignCalls, assignCall{
		slotID:    slotID,
		projectID: assignment.ProjectID,
		gitRepo:   assignment.GitRepoURL,
		token:     assignment.GitHubToken,
	})
	return m.assignErr
}

func (m *mockSlotProvisioner) ReleaseSlot(ctx context.Context, slotID string) error {
	m.releaseCalls = append(m.releaseCalls, slotID)
	return m.releaseErr
}

type statusMessage struct {
	sessionID string
	projectID string
	status    string
	message   string
}

type mockTokenProvider struct {
	token string
	err   error
	calls []string
}

func (m *mockTokenProvider) GetGitHubToken(ctx context.Context, userID string) (string, error) {
	m.calls = append(m.calls, userID)
	return m.token, m.err
}

func newTestConsumer(store store.Store, prov Provisioner, auth TokenProvider, statuses *[]statusMessage) *Consumer {
	return &Consumer{
		store:       store,
		provisioner: prov,
		authClient:  auth,
		statusPublisher: func(ctx context.Context, sessionID, projectID, status, message string) {
			*statuses = append(*statuses, statusMessage{sessionID: sessionID, projectID: projectID, status: status, message: message})
		},
	}
}

func TestHandleCreateRequestAssignsSlot(t *testing.T) {
	ctx := context.Background()
	memStore := store.NewInMemoryStore(2, 5)
	prov := &mockSlotProvisioner{}
	var statuses []statusMessage
	consumer := newTestConsumer(memStore, prov, nil, &statuses)

	event := &WorkspaceEvent{
		EventType: "WORKSPACE_CREATE_REQUESTED",
		Timestamp: time.Now(),
		Payload: WorkspacePayload{
			ProjectID:  "project-1",
			SessionID:  "session-1",
			GitRepoURL: "https://github.com/example/repo.git",
		},
	}

	err := consumer.handleCreateRequest(ctx, event)
	require.NoError(t, err)

	slot, err := memStore.GetSlotByProjectID(ctx, "project-1")
	require.NoError(t, err)
	assert.True(t, slot.IsBusy)
	assert.Equal(t, "project-1", slot.ProjectID)

	require.Len(t, prov.assignCalls, 1)
	assert.Equal(t, slot.ID, prov.assignCalls[0].slotID)
	assert.Equal(t, "project-1", prov.assignCalls[0].projectID)

	require.Len(t, statuses, 1)
	assert.Equal(t, "session-1", statuses[0].sessionID)
	assert.Equal(t, "project-1", statuses[0].projectID)
	assert.Equal(t, "RUNNING", statuses[0].status)
}

func TestHandleCreateRequestQueuesWhenFull(t *testing.T) {
	ctx := context.Background()
	memStore := store.NewInMemoryStore(1, 5)
	prov := &mockSlotProvisioner{}
	var statuses []statusMessage
	consumer := newTestConsumer(memStore, prov, nil, &statuses)

	slot, err := memStore.FindFreeSlot(ctx)
	require.NoError(t, err)
	require.NoError(t, memStore.AssignSlot(ctx, slot.ID, "existing-project"))

	event := &WorkspaceEvent{
		EventType: "WORKSPACE_CREATE_REQUESTED",
		Timestamp: time.Now(),
		Payload: WorkspacePayload{
			ProjectID: "queued-project",
			SessionID: "session-queued",
		},
	}

	err = consumer.handleCreateRequest(ctx, event)
	require.NoError(t, err)

	queueLen, err := memStore.GetQueueLength(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, queueLen)

	require.Len(t, statuses, 1)
	assert.Equal(t, "QUEUED", statuses[0].status)
	assert.Equal(t, "queued-project", statuses[0].projectID)
}

func TestHandleCreateRequestProvisionerFailureReleasesSlot(t *testing.T) {
	ctx := context.Background()
	memStore := store.NewInMemoryStore(1, 5)
	prov := &mockSlotProvisioner{assignErr: errors.New("provision failure")}
	var statuses []statusMessage
	consumer := newTestConsumer(memStore, prov, nil, &statuses)

	event := &WorkspaceEvent{
		EventType: "WORKSPACE_CREATE_REQUESTED",
		Timestamp: time.Now(),
		Payload: WorkspacePayload{
			ProjectID: "failing-project",
			SessionID: "session-fail",
		},
	}

	err := consumer.handleCreateRequest(ctx, event)
	require.Error(t, err)

	_, err = memStore.GetSlotByProjectID(ctx, "failing-project")
	assert.ErrorIs(t, err, store.ErrNotFound)
	assert.Empty(t, statuses)
}

func TestHandleReleaseRequestProcessesQueue(t *testing.T) {
	ctx := context.Background()
	memStore := store.NewInMemoryStore(1, 5)
	prov := &mockSlotProvisioner{}
	var statuses []statusMessage
	consumer := newTestConsumer(memStore, prov, nil, &statuses)

	createActive := &WorkspaceEvent{
		EventType: "WORKSPACE_CREATE_REQUESTED",
		Timestamp: time.Now(),
		Payload: WorkspacePayload{
			ProjectID:  "project-active",
			SessionID:  "session-active",
			GitRepoURL: "https://github.com/example/repo.git",
		},
	}
	require.NoError(t, consumer.handleCreateRequest(ctx, createActive))

	createQueued := &WorkspaceEvent{
		EventType: "WORKSPACE_CREATE_REQUESTED",
		Timestamp: time.Now(),
		Payload: WorkspacePayload{
			ProjectID: "project-queued",
			SessionID: "session-queued",
		},
	}
	require.NoError(t, consumer.handleCreateRequest(ctx, createQueued))

	releaseEvent := &WorkspaceEvent{
		EventType: "WORKSPACE_RELEASE_REQUESTED",
		Timestamp: time.Now(),
		Payload: WorkspacePayload{
			ProjectID: "project-active",
			SessionID: "session-active",
		},
	}
	require.NoError(t, consumer.handleReleaseRequest(ctx, releaseEvent))

	_, err := memStore.GetSlotByProjectID(ctx, "project-active")
	assert.ErrorIs(t, err, store.ErrNotFound)

	slot, err := memStore.GetSlotByProjectID(ctx, "project-queued")
	require.NoError(t, err)
	assert.Equal(t, "project-queued", slot.ProjectID)

	require.Len(t, prov.releaseCalls, 1)
	require.Len(t, prov.assignCalls, 2)

	var releaseStatus, runningStatus bool
	for _, status := range statuses {
		if status.projectID == "project-active" && status.status == "RELEASED" {
			releaseStatus = true
		}
		if status.projectID == "project-queued" && status.status == "RUNNING" {
			runningStatus = true
		}
	}

	assert.True(t, releaseStatus, "expected release status message")
	assert.True(t, runningStatus, "expected running status message for queued project")
}

func TestHandleMessageInvalidJSON(t *testing.T) {
	consumer := &Consumer{}
	err := consumer.handleMessage(context.Background(), []byte("not-json"))
	assert.Error(t, err)
}

func TestHandleMessageUnknownEvent(t *testing.T) {
	consumer := &Consumer{}
	event := &WorkspaceEvent{EventType: "UNKNOWN_EVENT"}
	payload, err := json.Marshal(event)
	require.NoError(t, err)

	err = consumer.handleMessage(context.Background(), payload)
	assert.NoError(t, err)
}

func TestHandleMessageRoutesCreate(t *testing.T) {
	ctx := context.Background()
	memStore := store.NewInMemoryStore(1, 5)
	prov := &mockSlotProvisioner{}
	var statuses []statusMessage
	consumer := newTestConsumer(memStore, prov, nil, &statuses)

	event := &WorkspaceEvent{
		EventType: "WORKSPACE_CREATE_REQUESTED",
		Timestamp: time.Now(),
		Payload: WorkspacePayload{
			ProjectID:  "project-handle",
			SessionID:  "session-handle",
			GitRepoURL: "https://github.com/example/repo.git",
		},
	}

	payload, err := json.Marshal(event)
	require.NoError(t, err)

	err = consumer.handleMessage(ctx, payload)
	require.NoError(t, err)

	require.Len(t, prov.assignCalls, 1)
	assert.Equal(t, "project-handle", prov.assignCalls[0].projectID)
}

func TestHandleCreateRequestFetchesToken(t *testing.T) {
	ctx := context.Background()
	memStore := store.NewInMemoryStore(1, 5)
	prov := &mockSlotProvisioner{}
	mockAuth := &mockTokenProvider{token: "fetched-token"}
	var statuses []statusMessage
	consumer := newTestConsumer(memStore, prov, mockAuth, &statuses)

	event := &WorkspaceEvent{
		EventType: "WORKSPACE_CREATE_REQUESTED",
		Timestamp: time.Now(),
		Payload: WorkspacePayload{
			ProjectID:   "project-token",
			SessionID:   "session-token",
			UserID:      "user-1",
			GitRepoURL:  "https://github.com/example/repo.git",
			GitHubToken: "", // Empty token to trigger fetch
		},
	}

	err := consumer.handleCreateRequest(ctx, event)
	require.NoError(t, err)

	require.Len(t, mockAuth.calls, 1)
	assert.Equal(t, "user-1", mockAuth.calls[0])

	require.Len(t, prov.assignCalls, 1)
	assert.Equal(t, "fetched-token", prov.assignCalls[0].token)
}
