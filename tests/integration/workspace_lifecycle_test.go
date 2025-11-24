//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	amqp "github.com/rabbitmq/amqp091-go"

	workspacepb "github.com/Aadithya-J/code_nest/proto"
)

const (
	workspaceServiceAddr = "localhost:50052"
	testExchange         = "workspace"
)

func rabbitURL() string {
	if url := os.Getenv("TEST_RABBITMQ_URL"); url != "" {
		return url
	}
	return os.Getenv("RABBITMQ_URL")
}

func setupRabbitTap(t *testing.T, routingKey string) (*amqp.Connection, *amqp.Channel, <-chan amqp.Delivery) {
	t.Helper()
	url := rabbitURL()
	if url == "" {
		t.Skip("RabbitMQ URL not configured; set TEST_RABBITMQ_URL or RABBITMQ_URL")
	}

	conn, err := amqp.Dial(url)
	if err != nil {
		t.Skipf("RabbitMQ not available: %v", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		require.NoError(t, err)
	}

	require.NoError(t, ch.ExchangeDeclare(
		testExchange,
		"topic",
		true,
		false,
		false,
		false,
		nil,
	))

	queue, err := ch.QueueDeclare(
		"",
		false,
		true,
		true,
		false,
		nil,
	)
	require.NoError(t, err)

	require.NoError(t, ch.QueueBind(
		queue.Name,
		routingKey,
		testExchange,
		false,
		nil,
	))

	msgs, err := ch.Consume(queue.Name, "", false, true, false, false, nil)
	require.NoError(t, err)

	return conn, ch, msgs
}

func expectMessage(t *testing.T, deliveries <-chan amqp.Delivery, matcher func(map[string]interface{}) bool) map[string]interface{} {
	t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		select {
		case msg := <-deliveries:
			require.NotNil(t, msg.Body)
			var payload map[string]interface{}
			require.NoError(t, json.Unmarshal(msg.Body, &payload))
			if matcher(payload) {
				require.NoError(t, msg.Ack(false))
				return payload
			}
			msg.Nack(false, false)
		case <-timeout:
			t.Fatal("timeout waiting for RabbitMQ message")
		}
	}
}

// TestWorkspaceSessionCreateRabbitMessage ensures session creation publishes the expected RabbitMQ message.
func TestWorkspaceSessionCreateRabbitMessage(t *testing.T) {
	conn, ch, msgs := setupRabbitTap(t, "create.requested")
	defer conn.Close()
	defer ch.Close()

	grpcConn, err := grpc.Dial(workspaceServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer grpcConn.Close()

	client := workspacepb.NewWorkspaceServiceClient(grpcConn)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	projectResp, err := client.CreateProject(ctx, &workspacepb.CreateProjectRequest{
		UserId:      "test-user-lifecycle-1",
		Name:        "lifecycle-test-project",
		Description: "Test project for lifecycle",
	})
	require.NoError(t, err)
	projectID := projectResp.Project.Id

	defer client.DeleteProject(context.Background(), &workspacepb.DeleteProjectRequest{
		Id:     projectID,
		UserId: "test-user-lifecycle-1",
	})

	sessionClient := workspacepb.NewSessionServiceClient(grpcConn)
	sessionResp, err := sessionClient.CreateWorkspaceSession(ctx, &workspacepb.CreateWorkspaceSessionRequest{
		ProjectId:  projectID,
		UserId:     "test-user-lifecycle-1",
		GitRepoUrl: "https://github.com/octocat/Hello-World.git",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, sessionResp.SessionId)

	event := expectMessage(t, msgs, func(data map[string]interface{}) bool {
		return data["event_type"] == "WORKSPACE_CREATE_REQUESTED"
	})

	payload := event["payload"].(map[string]interface{})
	assert.Equal(t, sessionResp.SessionId, payload["session_id"])
	assert.Equal(t, projectID, payload["project_id"])
	assert.Equal(t, "test-user-lifecycle-1", payload["user_id"])
	assert.Equal(t, "https://github.com/octocat/Hello-World.git", payload["git_repo_url"])
}

// TestWorkspaceSessionReleaseRabbitMessage ensures session release publishes the expected RabbitMQ message.
func TestWorkspaceSessionReleaseRabbitMessage(t *testing.T) {
	conn, ch, msgs := setupRabbitTap(t, "release.requested")
	defer conn.Close()
	defer ch.Close()

	grpcConn, err := grpc.Dial(workspaceServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer grpcConn.Close()

	client := workspacepb.NewWorkspaceServiceClient(grpcConn)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	projectResp, err := client.CreateProject(ctx, &workspacepb.CreateProjectRequest{
		UserId:      "test-user-lifecycle-2",
		Name:        "lifecycle-release-project",
		Description: "Test project for release",
	})
	require.NoError(t, err)
	projectID := projectResp.Project.Id

	defer client.DeleteProject(context.Background(), &workspacepb.DeleteProjectRequest{
		Id:     projectID,
		UserId: "test-user-lifecycle-2",
	})

	sessionClient := workspacepb.NewSessionServiceClient(grpcConn)
	sessionResp, err := sessionClient.CreateWorkspaceSession(ctx, &workspacepb.CreateWorkspaceSessionRequest{
		ProjectId: projectID,
		UserId:    "test-user-lifecycle-2",
	})
	require.NoError(t, err)
	sessionID := sessionResp.SessionId

	_, err = sessionClient.ReleaseWorkspaceSession(ctx, &workspacepb.ReleaseWorkspaceSessionRequest{
		SessionId: sessionID,
		ProjectId: projectID,
		UserId:    "test-user-lifecycle-2",
	})
	require.NoError(t, err)

	event := expectMessage(t, msgs, func(data map[string]interface{}) bool {
		return data["event_type"] == "WORKSPACE_RELEASE_REQUESTED"
	})

	payload := event["payload"].(map[string]interface{})
	assert.Equal(t, sessionID, payload["session_id"])
	assert.Equal(t, projectID, payload["project_id"])
}

func TestSessionWithInvalidProject(t *testing.T) {
	grpcConn, err := grpc.Dial(workspaceServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer grpcConn.Close()

	sessionClient := workspacepb.NewSessionServiceClient(grpcConn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = sessionClient.CreateWorkspaceSession(ctx, &workspacepb.CreateWorkspaceSessionRequest{
		ProjectId:  "non-existent-project-id",
		UserId:     "test-user-invalid",
		GitRepoUrl: "",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestReleaseNonExistentSession(t *testing.T) {
	grpcConn, err := grpc.Dial(workspaceServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer grpcConn.Close()

	client := workspacepb.NewWorkspaceServiceClient(grpcConn)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	projectResp, err := client.CreateProject(ctx, &workspacepb.CreateProjectRequest{
		UserId:      "test-user-release",
		Name:        "release-test-project",
		Description: "Test",
	})
	require.NoError(t, err)
	projectID := projectResp.Project.Id

	defer client.DeleteProject(context.Background(), &workspacepb.DeleteProjectRequest{
		Id:     projectID,
		UserId: "test-user-release",
	})

	sessionClient := workspacepb.NewSessionServiceClient(grpcConn)
	resp, err := sessionClient.ReleaseWorkspaceSession(ctx, &workspacepb.ReleaseWorkspaceSessionRequest{
		SessionId: "non-existent-session",
		ProjectId: projectID,
		UserId:    "test-user-release",
	})
	require.NoError(t, err)
	assert.Equal(t, "non-existent-session", resp.SessionId)
}
