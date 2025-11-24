//go:build e2e

package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/Aadithya-J/code_nest/tests/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSessionCreate tests workspace session creation
func TestSessionCreate(t *testing.T) {
	client, projectID := setupProjectForFileTests(t, "session-create")
	defer client.DELETE(t, "/workspace/projects/"+projectID)

	tests := []struct {
		name           string
		projectID      string
		gitRepoUrl     string
		expectedStatus int
		shouldHaveID   bool
	}{
		{
			name:           "Valid session creation",
			projectID:      projectID,
			gitRepoUrl:     "https://github.com/octocat/Hello-World.git",
			expectedStatus: 202,
			shouldHaveID:   true,
		},
		{
			name:           "Session without git URL",
			projectID:      projectID,
			gitRepoUrl:     "",
			expectedStatus: 202,
			shouldHaveID:   true,
		},
		{
			name:           "Session with non-GitHub URL",
			projectID:      projectID,
			gitRepoUrl:     "https://gitlab.com/user/project.git",
			expectedStatus: 202,
			shouldHaveID:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, body := client.POST(t, "/workspace/sessions", map[string]interface{}{
				"projectId":  tt.projectID,
				"gitRepoUrl": tt.gitRepoUrl,
			})

			assert.Equal(t, tt.expectedStatus, resp.StatusCode, "Response body: %s", string(body))

			if tt.shouldHaveID {
				var result map[string]interface{}
				client.ParseJSON(t, body, &result)

				assert.NotEmpty(t, result["sessionId"], "Should return session ID")
				assert.Equal(t, "CREATING", result["status"], "Status should be CREATING")
				assert.Contains(t, result["message"], "requested", "Should have creation message")
			}
		})
	}
}

// TestSessionLifecycle tests complete session lifecycle
func TestSessionLifecycle(t *testing.T) {
	client, projectID := setupProjectForFileTests(t, "session-lifecycle")
	defer client.DELETE(t, "/workspace/projects/"+projectID)

	var sessionID string

	t.Run("Create workspace session", func(t *testing.T) {
		resp, body := client.POST(t, "/workspace/sessions", map[string]interface{}{
			"projectId":  projectID,
			"gitRepoUrl": "https://github.com/octocat/Hello-World.git",
		})

		require.Equal(t, 202, resp.StatusCode, "Session creation should return 202 Accepted, body: %s", string(body))

		var result map[string]interface{}
		client.ParseJSON(t, body, &result)

		sessionID = result["sessionId"].(string)
		assert.NotEmpty(t, sessionID, "Should return session ID")
		assert.Equal(t, "CREATING", result["status"])
		assert.Contains(t, result["message"], "Workspace session creation requested")
	})

	t.Run("Wait for session to be processed", func(t *testing.T) {
		// In a real scenario, session would be provisioned by runner-allocator
		// Here we just wait a bit to simulate async processing
		time.Sleep(3 * time.Second)
	})

	t.Run("Release workspace session", func(t *testing.T) {
		resp, body := client.DELETE(t, fmt.Sprintf("/workspace/sessions/%s?projectId=%s", sessionID, projectID))

		require.Equal(t, 200, resp.StatusCode, "Session release should succeed, body: %s", string(body))

		var result map[string]interface{}
		client.ParseJSON(t, body, &result)

		assert.Equal(t, sessionID, result["sessionId"])
		assert.Equal(t, "RELEASING", result["status"])
		assert.Contains(t, result["message"], "release requested")
	})
}

// TestSessionWithoutProjectID tests session operations require projectId
func TestSessionWithoutProjectID(t *testing.T) {
	client, projectID := setupProjectForFileTests(t, "session-validation")
	defer client.DELETE(t, "/workspace/projects/"+projectID)

	// Create a session first
	resp, body := client.POST(t, "/workspace/sessions", map[string]interface{}{
		"projectId":  projectID,
		"gitRepoUrl": "https://github.com/octocat/Hello-World.git",
	})
	require.Equal(t, 202, resp.StatusCode)

	var result map[string]interface{}
	client.ParseJSON(t, body, &result)
	sessionID := result["sessionId"].(string)

	t.Run("Release without projectId parameter", func(t *testing.T) {
		resp, _ := client.DELETE(t, fmt.Sprintf("/workspace/sessions/%s", sessionID))

		assert.Equal(t, 400, resp.StatusCode, "Should require projectId parameter")
	})
}

// TestSessionMultipleCreation tests creating multiple sessions
func TestSessionMultipleCreation(t *testing.T) {
	client, projectID := setupProjectForFileTests(t, "session-multiple")
	defer client.DELETE(t, "/workspace/projects/"+projectID)

	sessionIDs := []string{}

	t.Run("Create multiple sessions", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			resp, body := client.POST(t, "/workspace/sessions", map[string]interface{}{
				"projectId":  projectID,
				"gitRepoUrl": fmt.Sprintf("https://github.com/Aadithya-J/tictactoe%d.git", i),
			})

			require.Equal(t, 202, resp.StatusCode, "Session %d creation failed: %s", i, string(body))

			var result map[string]interface{}
			client.ParseJSON(t, body, &result)

			sessionID := result["sessionId"].(string)
			sessionIDs = append(sessionIDs, sessionID)

			assert.NotEmpty(t, sessionID)
			assert.Contains(t, sessionID, projectID, "Session ID should contain project ID")
		}
	})

	t.Run("All session IDs are unique", func(t *testing.T) {
		seen := make(map[string]bool)
		for _, id := range sessionIDs {
			assert.False(t, seen[id], "Session ID %s should be unique", id)
			seen[id] = true
		}
	})

	// Cleanup
	t.Cleanup(func() {
		time.Sleep(2 * time.Second)
		for _, sessionID := range sessionIDs {
			client.DELETE(t, fmt.Sprintf("/workspace/sessions/%s?projectId=%s", sessionID, projectID))
		}
	})
}

// TestSessionUnauthorizedAccess tests authorization on session operations
func TestSessionUnauthorizedAccess(t *testing.T) {
	client1, projectID := setupProjectForFileTests(t, "session-auth1")
	defer client1.DELETE(t, "/workspace/projects/"+projectID)

	client2, _ := setupAuthenticatedClient(t, "session-auth2")

	// User 1 creates a session
	resp, body := client1.POST(t, "/workspace/sessions", map[string]interface{}{
		"projectId":  projectID,
		"gitRepoUrl": "https://github.com/octocat/Hello-World.git",
	})
	require.Equal(t, 202, resp.StatusCode)

	var result map[string]interface{}
	client1.ParseJSON(t, body, &result)
	sessionID := result["sessionId"].(string)

	t.Run("User2 cannot release User1's session", func(t *testing.T) {
		resp, _ := client2.DELETE(t, fmt.Sprintf("/workspace/sessions/%s?projectId=%s", sessionID, projectID))

		// Should fail because user2 doesn't own the project
		assert.NotEqual(t, 200, resp.StatusCode, "Should not allow unauthorized session release")
	})

	// Cleanup
	t.Cleanup(func() {
		time.Sleep(2 * time.Second)
		client1.DELETE(t, fmt.Sprintf("/workspace/sessions/%s?projectId=%s", sessionID, projectID))
	})
}

// TestSessionWithoutAuth tests that session operations require authentication
func TestSessionWithoutAuth(t *testing.T) {
	client := helpers.NewHTTPClient()
	// Don't set auth token

	t.Run("Create session without auth", func(t *testing.T) {
		resp, _ := client.POST(t, "/workspace/sessions", map[string]interface{}{
			"projectId":  "some-id",
			"gitRepoUrl": "https://github.com/octocat/Hello-World.git",
		})
		assert.Equal(t, 401, resp.StatusCode)
	})

	t.Run("Release session without auth", func(t *testing.T) {
		resp, _ := client.DELETE(t, "/workspace/sessions/some-session?projectId=some-id")
		assert.Equal(t, 401, resp.StatusCode)
	})
}

// TestSessionErrorCases tests various error scenarios
func TestSessionErrorCases(t *testing.T) {
	client, projectID := setupProjectForFileTests(t, "session-errors")
	defer client.DELETE(t, "/workspace/projects/"+projectID)

	t.Run("Create session without projectId", func(t *testing.T) {
		resp, _ := client.POST(t, "/workspace/sessions", map[string]interface{}{
			"gitRepoUrl": "https://github.com/octocat/Hello-World.git",
		})

		assert.NotEqual(t, 202, resp.StatusCode, "Should require projectId")
	})

	t.Run("Create session with invalid projectId", func(t *testing.T) {
		resp, _ := client.POST(t, "/workspace/sessions", map[string]interface{}{
			"projectId":  "invalid-non-existent-id",
			"gitRepoUrl": "https://github.com/octocat/Hello-World.git",
		})

		assert.NotEqual(t, 202, resp.StatusCode, "Should fail for invalid projectId")
	})

	t.Run("Release non-existent session", func(t *testing.T) {
		resp, _ := client.DELETE(t, fmt.Sprintf("/workspace/sessions/non-existent?projectId=%s", projectID))

		// This might succeed or fail depending on implementation
		// The important part is it doesn't crash
		assert.NotEqual(t, 500, resp.StatusCode, "Should handle gracefully")
	})
}
