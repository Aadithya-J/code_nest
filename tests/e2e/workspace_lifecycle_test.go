//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/Aadithya-J/code_nest/tests/helpers"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkspaceLifecycle verifies the full asynchronous lifecycle of a workspace
// including creation, status updates via WebSocket, and release.
func TestWorkspaceLifecycle(t *testing.T) {
	client := helpers.NewHTTPClient()

	// 1. Setup: Create user, login, and create project
	timestamp := time.Now().UnixNano()
	email := fmt.Sprintf("lifecycle-test%d@example.com", timestamp)
	password := "testpassword123"

	t.Logf("Creating user %s", email)
	resp, body := client.POST(t, "/auth/signup", map[string]string{
		"email":    email,
		"password": password,
	})
	require.Equal(t, 200, resp.StatusCode)

	var authResult map[string]interface{}
	client.ParseJSON(t, body, &authResult)
	client.SetAuthToken(authResult["token"].(string))

	t.Log("Creating project")
	resp, body = client.POST(t, "/workspace/projects", map[string]interface{}{
		"name":        "Lifecycle Test Project",
		"description": "Testing async flow",
	})
	require.Equal(t, 200, resp.StatusCode)

	var projectResult map[string]interface{}
	client.ParseJSON(t, body, &projectResult)
	project := projectResult["project"].(map[string]interface{})
	projectID := project["id"].(string)

	// 2. Request Workspace Session
	t.Log("Requesting workspace session")
	resp, body = client.POST(t, "/workspace/sessions", map[string]interface{}{
		"projectId":  projectID,
		"gitRepoUrl": "https://github.com/octocat/Hello-World.git",
	})
	require.Equal(t, 202, resp.StatusCode)

	var sessionResult map[string]interface{}
	client.ParseJSON(t, body, &sessionResult)
	sessionID := sessionResult["sessionId"].(string)
	require.NotEmpty(t, sessionID)

	// 3. Connect to WebSocket to listen for status updates
	t.Logf("Connecting to WebSocket for session %s", sessionID)
	u := url.URL{Scheme: "ws", Host: "localhost:8080", Path: "/ws/status", RawQuery: "sessionId=" + sessionID}
	
	// Retry connection a few times if needed
	var ws *websocket.Conn
	var err error
	for i := 0; i < 5; i++ {
		ws, _, err = websocket.DefaultDialer.Dial(u.String(), nil)
		if err == nil {
			break
		}
		time.Sleep(1 * time.Second)
	}
	require.NoError(t, err, "Failed to connect to WebSocket")
	defer ws.Close()

	// 4. Wait for RUNNING status
	t.Log("Waiting for RUNNING status...")
	statusChan := make(chan string)
	errChan := make(chan error)

	go func() {
		for {
			_, message, err := ws.ReadMessage()
			if err != nil {
				errChan <- err
				return
			}
			
			var update map[string]interface{}
			if err := json.Unmarshal(message, &update); err != nil {
				continue
			}

			if update["type"] == "status_update" {
				status := update["status"].(string)
				msg := update["message"].(string)
				t.Logf("Received status: %s - %s", status, msg)
				
				if status == "RUNNING" {
					statusChan <- status
					return
				}
				if status == "FAILED" {
					errChan <- fmt.Errorf("Workspace creation failed: %s", msg)
					return
				}
			}
		}
	}()

	select {
	case status := <-statusChan:
		assert.Equal(t, "RUNNING", status)
	case err := <-errChan:
		t.Fatalf("WebSocket error: %v", err)
	case <-time.After(60 * time.Second):
		t.Fatal("Timeout waiting for RUNNING status")
	}

	// 5. Release Workspace
	t.Log("Releasing workspace session")
	resp, _ = client.DELETE(t, fmt.Sprintf("/workspace/sessions/%s?projectId=%s", sessionID, projectID))
	require.Equal(t, 200, resp.StatusCode)

	// 6. Wait for RELEASED status
	t.Log("Waiting for RELEASED status...")
	
	// Re-use channels for next event
	go func() {
		for {
			_, message, err := ws.ReadMessage()
			if err != nil {
				errChan <- err
				return
			}
			
			var update map[string]interface{}
			if err := json.Unmarshal(message, &update); err != nil {
				continue
			}

			if update["type"] == "status_update" {
				status := update["status"].(string)
				t.Logf("Received status: %s", status)
				
				if status == "RELEASED" {
					statusChan <- status
					return
				}
			}
		}
	}()

	select {
	case status := <-statusChan:
		assert.Equal(t, "RELEASED", status)
	case err := <-errChan:
		// Ignore close errors if they happen during shutdown
		t.Logf("WebSocket closed: %v", err)
	case <-time.After(30 * time.Second):
		t.Fatal("Timeout waiting for RELEASED status")
	}

	// Cleanup
	client.DELETE(t, "/workspace/projects/"+projectID)
	t.Log("✅ Workspace lifecycle test passed!")
}
