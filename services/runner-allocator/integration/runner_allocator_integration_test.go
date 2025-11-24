//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/Aadithya-J/code_nest/services/runner-allocator/internal/models"
	runnerconsumer "github.com/Aadithya-J/code_nest/services/runner-allocator/internal/rabbitmq"
	"github.com/Aadithya-J/code_nest/services/runner-allocator/internal/store"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	rabbitExchange      = runnerconsumer.ExchangeName
	rabbitExchangeType  = runnerconsumer.ExchangeType
	rabbitRequestsQueue = runnerconsumer.QueueRequests
)

type statusEvent struct {
	sessionID string
	projectID string
	status    string
	message   string
}

type provisionCall struct {
	slotID    string
	projectID string
	gitRepo   string
}

type mockSlotProvisioner struct {
	provisionCalls   []provisionCall
	releaseCalls     []string
	shouldFailAssign bool
}

func (m *mockSlotProvisioner) ProvisionWorkspace(ctx context.Context, projectID, gitRepoURL string) error {
	return nil
}

func (m *mockSlotProvisioner) DeprovisionWorkspace(ctx context.Context, projectID string) error {
	return nil
}

func (m *mockSlotProvisioner) AssignSlotToProject(ctx context.Context, slotID string, assignment *models.SlotAssignment) error {
	if m.shouldFailAssign {
		return assert.AnError
	}
	m.provisionCalls = append(m.provisionCalls, provisionCall{
		slotID:    slotID,
		projectID: assignment.ProjectID,
		gitRepo:   assignment.GitRepoURL,
	})
	return nil
}

func (m *mockSlotProvisioner) ReleaseSlot(ctx context.Context, slotID string) error {
	m.releaseCalls = append(m.releaseCalls, slotID)
	return nil
}

func getRabbitURL() string {
	if url := os.Getenv("TEST_RABBITMQ_URL"); url != "" {
		return url
	}
	return os.Getenv("RABBITMQ_URL")
}

func setupRabbit(t *testing.T) (*amqp.Connection, *amqp.Channel) {
	t.Helper()

	url := getRabbitURL()
	if url == "" {
		t.Skip("RabbitMQ URL not configured; set TEST_RABBITMQ_URL or RABBITMQ_URL")
	}

	conn, err := amqp.Dial(url)
	if err != nil {
		t.Skipf("RabbitMQ not available at %s: %v", url, err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		require.NoError(t, err)
	}

	require.NoError(t, ch.ExchangeDeclare(
		rabbitExchange,
		rabbitExchangeType,
		true,
		false,
		false,
		false,
		nil,
	))

	if _, err := ch.QueueDeclare(
		rabbitRequestsQueue,
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		require.NoError(t, err)
	}

	require.NoError(t, ch.QueueBind(
		rabbitRequestsQueue,
		"*.requested",
		rabbitExchange,
		false,
		nil,
	))

	return conn, ch
}

func purgeQueues(t *testing.T, ch *amqp.Channel) {
	t.Helper()
	if _, err := ch.QueuePurge(rabbitRequestsQueue, false); err != nil {
		require.NoError(t, err)
	}
}

func publishWorkspaceEvent(t *testing.T, ch *amqp.Channel, routingKey string, event map[string]interface{}) {
	t.Helper()
	payload, err := json.Marshal(event)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, ch.PublishWithContext(ctx,
		rabbitExchange,
		routingKey,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         payload,
			DeliveryMode: amqp.Persistent,
		},
	))
}

func startTestConsumer(t *testing.T, storeInstance store.Store, prov runnerconsumer.Provisioner, statuses chan<- statusEvent) (*runnerconsumer.Consumer, context.CancelFunc) {
	url := getRabbitURL()
	consumer, err := runnerconsumer.NewConsumer(url, storeInstance, prov, nil)
	require.NoError(t, err)

	consumer.SetStatusPublisher(func(ctx context.Context, sessionID, projectID, status, message string) {
		select {
		case statuses <- statusEvent{sessionID: sessionID, projectID: projectID, status: status, message: message}:
		default:
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		err := consumer.Start(ctx)
		if err != nil && ctx.Err() == nil {
			t.Errorf("consumer start error: %v", err)
		}
	}()

	return consumer, cancel
}

func waitForStatus(t *testing.T, ch <-chan statusEvent, predicate func(statusEvent) bool) statusEvent {
	t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		select {
		case evt := <-ch:
			if predicate(evt) {
				return evt
			}
		case <-timeout:
			t.Fatal("timeout waiting for status event")
		}
	}
}

func TestRunnerAllocatorProcessesCreateRequest(t *testing.T) {
	conn, ch := setupRabbit(t)
	defer conn.Close()
	defer ch.Close()

	purgeQueues(t, ch)

	storeInstance := store.NewInMemoryStore(1, 10)
	prov := &mockSlotProvisioner{}
	statusCh := make(chan statusEvent, 4)

	consumer, cancel := startTestConsumer(t, storeInstance, prov, statusCh)
	defer func() {
		cancel()
		time.Sleep(200 * time.Millisecond)
		consumer.Close()
	}()

	time.Sleep(150 * time.Millisecond)
	sessionID := "session-create-" + time.Now().Format("150405")
	projectID := "project-create"

	event := map[string]interface{}{
		"event_type": "WORKSPACE_CREATE_REQUESTED",
		"timestamp":  time.Now().UTC(),
		"payload": map[string]interface{}{
			"project_id":   projectID,
			"user_id":      "user-create",
			"git_repo_url": "https://github.com/test/repo.git",
			"session_id":   sessionID,
		},
	}

	publishWorkspaceEvent(t, ch, "create.requested", event)

	status := waitForStatus(t, statusCh, func(evt statusEvent) bool {
		return evt.projectID == projectID && evt.sessionID == sessionID
	})

	assert.Equal(t, "RUNNING", status.status)
	assert.NotEmpty(t, prov.provisionCalls)
	assert.Equal(t, projectID, prov.provisionCalls[0].projectID)
}

func TestRunnerAllocatorQueuesWhenNoSlots(t *testing.T) {
	conn, ch := setupRabbit(t)
	defer conn.Close()
	defer ch.Close()

	purgeQueues(t, ch)

	storeInstance := store.NewInMemoryStore(1, 10)
	slot, err := storeInstance.FindFreeSlot(context.Background())
	require.NoError(t, err)
	require.NoError(t, storeInstance.AssignSlot(context.Background(), slot.ID, "occupied-project"))

	prov := &mockSlotProvisioner{}
	statusCh := make(chan statusEvent, 4)

	consumer, cancel := startTestConsumer(t, storeInstance, prov, statusCh)
	defer func() {
		cancel()
		time.Sleep(200 * time.Millisecond)
		consumer.Close()
	}()
	time.Sleep(150 * time.Millisecond)

	sessionID := "session-queued-" + time.Now().Format("150405")
	projectID := "project-queued"
	event := map[string]interface{}{
		"event_type": "WORKSPACE_CREATE_REQUESTED",
		"timestamp":  time.Now().UTC(),
		"payload": map[string]interface{}{
			"project_id": projectID,
			"user_id":    "user-queued",
			"session_id": sessionID,
		},
	}

	publishWorkspaceEvent(t, ch, "create.requested", event)

	status := waitForStatus(t, statusCh, func(evt statusEvent) bool {
		return evt.projectID == projectID
	})

	assert.Equal(t, "QUEUED", status.status)
	queueLen, err := storeInstance.GetQueueLength(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, queueLen)
	assert.Empty(t, prov.provisionCalls)
}

func TestRunnerAllocatorReleaseProcessesQueue(t *testing.T) {
	conn, ch := setupRabbit(t)
	defer conn.Close()
	defer ch.Close()

	purgeQueues(t, ch)

	storeInstance := store.NewInMemoryStore(1, 10)
	prov := &mockSlotProvisioner{}
	statusCh := make(chan statusEvent, 8)

	consumer, cancel := startTestConsumer(t, storeInstance, prov, statusCh)
	defer func() {
		cancel()
		time.Sleep(200 * time.Millisecond)
		consumer.Close()
	}()
	time.Sleep(150 * time.Millisecond)

	activeSession := "session-active-" + time.Now().Format("150405")
	queuedSession := "session-queued-" + time.Now().Format("150405")

	createActive := map[string]interface{}{
		"event_type": "WORKSPACE_CREATE_REQUESTED",
		"timestamp":  time.Now().UTC(),
		"payload": map[string]interface{}{
			"project_id":   "project-active",
			"user_id":      "user",
			"git_repo_url": "",
			"session_id":   activeSession,
		},
	}
	publishWorkspaceEvent(t, ch, "create.requested", createActive)
	waitForStatus(t, statusCh, func(evt statusEvent) bool { return evt.projectID == "project-active" && evt.status == "RUNNING" })

	createQueued := map[string]interface{}{
		"event_type": "WORKSPACE_CREATE_REQUESTED",
		"timestamp":  time.Now().UTC(),
		"payload": map[string]interface{}{
			"project_id": "project-queued",
			"user_id":    "user",
			"session_id": queuedSession,
		},
	}
	publishWorkspaceEvent(t, ch, "create.requested", createQueued)
	waitForStatus(t, statusCh, func(evt statusEvent) bool { return evt.projectID == "project-queued" && evt.status == "QUEUED" })

	releaseEvent := map[string]interface{}{
		"event_type": "WORKSPACE_RELEASE_REQUESTED",
		"timestamp":  time.Now().UTC(),
		"payload": map[string]interface{}{
			"project_id": "project-active",
			"user_id":    "user",
			"session_id": activeSession,
		},
	}
	publishWorkspaceEvent(t, ch, "release.requested", releaseEvent)

	releaseSeen := false
	runningSeen := false
	var runningQueued statusEvent
	deadline := time.After(10 * time.Second)

	for !(releaseSeen && runningSeen) {
		select {
		case evt := <-statusCh:
			if evt.projectID == "project-active" && evt.status == "RELEASED" {
				releaseSeen = true
			}
			if evt.projectID == "project-queued" && evt.status == "RUNNING" {
				runningQueued = evt
				runningSeen = true
			}
		case <-deadline:
			t.Fatal("timeout waiting for queued project to start after release")
		}
	}

	assert.Equal(t, queuedSession, runningQueued.sessionID)

	require.Len(t, prov.releaseCalls, 1)
	require.GreaterOrEqual(t, len(prov.provisionCalls), 2)
}
