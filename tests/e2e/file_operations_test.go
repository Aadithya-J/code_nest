//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/Aadithya-J/code_nest/proto"
	"github.com/Aadithya-J/code_nest/tests/helpers"
	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TestFileOperationsWithKubectlExec tests the complete kubectl exec-based file workflow
func TestFileOperationsWithKubectlExec(t *testing.T) {
	// Setup: Create project and session
	client, projectID := setupProjectForFileTests(t, "file-ops-kubectl")
	defer client.DELETE(t, "/workspace/projects/"+projectID)

	// Step 1: Create workspace session with Hello-World repo
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
		require.NotEmpty(t, sessionID, "Should return session ID")
		assert.Equal(t, "CREATING", result["status"])
	})

	// Step 2: Wait for session to be RUNNING
	t.Run("Wait for session to be RUNNING", func(t *testing.T) {
		timeout := time.After(120 * time.Second)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		sessionRunning := false
		for !sessionRunning {
			select {
			case <-timeout:
				t.Fatal("Timeout waiting for session to be RUNNING")
			case <-ticker.C:
				// Check session status via workspace-service gRPC
				conn, err := grpc.Dial("localhost:50052", grpc.WithTransportCredentials(insecure.NewCredentials()))
				if err != nil {
					t.Logf("Failed to connect to workspace-service: %v", err)
					continue
				}
				defer conn.Close()

				// Wait for session to be ready
				if err := client.WaitForSessionReady(t, projectID, 120*time.Second); err != nil {
					t.Logf("Workspace not ready: %v", err)
					continue
				}
				sessionRunning = true
			}
		}

		t.Log("Session is now RUNNING")
	})

	// Step 3: Test GetFileTree
	var fileTree map[string]interface{}
	t.Run("Get initial file tree", func(t *testing.T) {
		resp, body := client.GET(t, fmt.Sprintf("/workspace/projects/%s/files/tree", projectID))

		require.Equal(t, 200, resp.StatusCode, "GetFileTree should return 200, body: %s", string(body))

		client.ParseJSON(t, body, &fileTree)
		assert.NotNil(t, fileTree["nodes"], "Should have nodes in tree")

		nodes, ok := fileTree["nodes"].([]interface{})
		require.True(t, ok, "nodes should be an array")
		assert.Greater(t, len(nodes), 0, "Should have at least one file/directory")

		t.Logf("File tree: %+v", fileTree)
	})

	// Step 4: Test SaveFile - create a new file
	t.Run("Save new file", func(t *testing.T) {
		resp, body := client.POST(t, fmt.Sprintf("/workspace/projects/%s/files", projectID), map[string]interface{}{
			"path":    "test-file.txt",
			"content": "Hello from CodeNest E2E test!",
		})

		require.Equal(t, 200, resp.StatusCode, "SaveFile should return 200, body: %s", string(body))

		var result map[string]interface{}
		client.ParseJSON(t, body, &result)

		file := result["file"].(map[string]interface{})
		assert.Equal(t, "test-file.txt", file["path"])
		assert.Equal(t, projectID, file["projectId"])
	})

	// Step 5: Wait for inotify to commit the change
	t.Run("Wait for auto-commit", func(t *testing.T) {
		time.Sleep(5 * time.Second)
		t.Log("Waited for inotify auto-commit")
	})

	// Step 6: Test GetFile - read the file back
	t.Run("Get saved file", func(t *testing.T) {
		resp, body := client.GET(t, fmt.Sprintf("/workspace/projects/%s/files?path=test-file.txt", projectID))

		require.Equal(t, 200, resp.StatusCode, "GetFile should return 200, body: %s", string(body))

		var result map[string]interface{}
		client.ParseJSON(t, body, &result)

		file := result["file"].(map[string]interface{})
		assert.Equal(t, "test-file.txt", file["path"])
		assert.Equal(t, "Hello from CodeNest E2E test!", file["content"])
	})

	// Step 7: Test SaveFile - update existing file
	t.Run("Update existing file", func(t *testing.T) {
		resp, body := client.POST(t, fmt.Sprintf("/workspace/projects/%s/files", projectID), map[string]interface{}{
			"path":    "test-file.txt",
			"content": "Updated content from E2E test!",
		})

		require.Equal(t, 200, resp.StatusCode, "SaveFile should return 200, body: %s", string(body))
	})

	// Step 8: Verify update
	t.Run("Verify file update", func(t *testing.T) {
		resp, body := client.GET(t, fmt.Sprintf("/workspace/projects/%s/files?path=test-file.txt", projectID))

		require.Equal(t, 200, resp.StatusCode, "GetFile should return 200, body: %s", string(body))

		var result map[string]interface{}
		client.ParseJSON(t, body, &result)

		file := result["file"].(map[string]interface{})
		assert.Equal(t, "Updated content from E2E test!", file["content"])
	})

	// Step 9: Test RenameFile
	t.Run("Rename file", func(t *testing.T) {
		resp, body := client.PUT(t, fmt.Sprintf("/workspace/projects/%s/files/rename", projectID), map[string]interface{}{
			"oldPath": "test-file.txt",
			"newPath": "renamed-file.txt",
		})

		require.Equal(t, 200, resp.StatusCode, "RenameFile should return 200, body: %s", string(body))
	})

	// Step 10: Verify rename
	t.Run("Verify file rename", func(t *testing.T) {
		// Old path should not exist
		resp, _ := client.GET(t, fmt.Sprintf("/workspace/projects/%s/files?path=test-file.txt", projectID))
		assert.Equal(t, 404, resp.StatusCode, "Old path should not exist")

		// New path should exist with same content
		resp, body := client.GET(t, fmt.Sprintf("/workspace/projects/%s/files?path=renamed-file.txt", projectID))
		require.Equal(t, 200, resp.StatusCode, "GetFile with new path should return 200, body: %s", string(body))

		var result map[string]interface{}
		client.ParseJSON(t, body, &result)

		file := result["file"].(map[string]interface{})
		assert.Equal(t, "Updated content from E2E test!", file["content"])
	})

	// Step 11: Test DeleteFile
	t.Run("Delete file", func(t *testing.T) {
		resp, body := client.DELETE(t, fmt.Sprintf("/workspace/projects/%s/files?path=renamed-file.txt", projectID))

		require.Equal(t, 200, resp.StatusCode, "DeleteFile should return 200, body: %s", string(body))

		var result map[string]interface{}
		client.ParseJSON(t, body, &result)
		assert.True(t, result["success"].(bool))
	})

	// Step 12: Verify deletion
	t.Run("Verify file deletion", func(t *testing.T) {
		resp, _ := client.GET(t, fmt.Sprintf("/workspace/projects/%s/files?path=renamed-file.txt", projectID))
		assert.Equal(t, 404, resp.StatusCode, "Deleted file should not exist")
	})

	// Step 13: Verify commits on GitHub temp branch
	t.Run("Verify GitHub commits", func(t *testing.T) {
		// This would require GitHub API integration to verify
		// For now, we'll just log that we should check manually
		t.Logf("TODO: Verify commits on branch codenest-session-%s", sessionID)
		t.Log("Manual verification required: Check GitHub for auto-commits")
	})

	// Step 14: Release session
	t.Run("Release session", func(t *testing.T) {
		resp, body := client.DELETE(t, fmt.Sprintf("/workspace/sessions/%s?projectId=%s", sessionID, projectID))

		require.Equal(t, 200, resp.StatusCode, "ReleaseSession should return 200, body: %s", string(body))

		var result map[string]interface{}
		client.ParseJSON(t, body, &result)
		assert.Equal(t, sessionID, result["sessionId"])
		assert.Contains(t, []string{"RELEASED", "RELEASING"}, result["status"])
	})

	// Step 15: Wait for session cleanup and merge
	t.Run("Wait for session cleanup", func(t *testing.T) {
		time.Sleep(10 * time.Second)
		t.Log("TODO: Verify merge to main branch on GitHub")
		t.Log("Manual verification required: Check GitHub for squashed merge to main")
	})
}

// TestFileTreeStructure tests the file tree structure parsing
func TestFileTreeStructure(t *testing.T) {
	client, projectID := setupProjectForFileTests(t, "file-tree")
	defer client.DELETE(t, "/workspace/projects/"+projectID)

	// Create session
	resp, body := client.POST(t, "/workspace/sessions", map[string]interface{}{
		"projectId":  projectID,
		"gitRepoUrl": "https://github.com/octocat/Hello-World.git",
	})
	require.Equal(t, 202, resp.StatusCode)

	var sessionResult map[string]interface{}
	client.ParseJSON(t, body, &sessionResult)
	sessionID := sessionResult["sessionId"].(string)
	defer client.DELETE(t, fmt.Sprintf("/workspace/sessions/%s?projectId=%s", sessionID, projectID))

	// Wait for session to be ready
	if err := client.WaitForSessionReady(t, projectID, 120*time.Second); err != nil {
		t.Fatalf("Workspace not ready: %v", err)
	}

	// Get file tree
	resp, body = client.GET(t, fmt.Sprintf("/workspace/projects/%s/files/tree", projectID))
	require.Equal(t, 200, resp.StatusCode, "body: %s", string(body))

	var result map[string]interface{}
	client.ParseJSON(t, body, &result)

	nodes, ok := result["nodes"].([]interface{})
	require.True(t, ok, "nodes should be an array")
	assert.Greater(t, len(nodes), 0, "Should have files in tree")

	// Verify tree structure
	for _, nodeInterface := range nodes {
		node := nodeInterface.(map[string]interface{})
		assert.NotEmpty(t, node["name"], "Node should have a name")
		assert.NotEmpty(t, node["path"], "Node should have a path")
		assert.NotNil(t, node["isDirectory"], "Node should have isDirectory flag")

		// If directory, should have children array (may be empty)
		if node["isDirectory"].(bool) {
			_, hasChildren := node["children"]
			assert.True(t, hasChildren, "Directory should have children array")
		}

		t.Logf("Node: name=%s, path=%s, isDir=%v", node["name"], node["path"], node["isDirectory"])
	}
}

// TestConcurrentFileOperations tests multiple file operations in parallel
func TestConcurrentFileOperations(t *testing.T) {
	client, projectID := setupProjectForFileTests(t, "concurrent-files")
	defer client.DELETE(t, "/workspace/projects/"+projectID)

	// Create session
	resp, body := client.POST(t, "/workspace/sessions", map[string]interface{}{
		"projectId":  projectID,
		"gitRepoUrl": "https://github.com/octocat/Hello-World.git",
	})
	require.Equal(t, 202, resp.StatusCode)

	var sessionResult map[string]interface{}
	client.ParseJSON(t, body, &sessionResult)
	sessionID := sessionResult["sessionId"].(string)
	defer client.DELETE(t, fmt.Sprintf("/workspace/sessions/%s?projectId=%s", sessionID, projectID))

	// Wait for session
	if err := client.WaitForSessionReady(t, projectID, 120*time.Second); err != nil {
		t.Fatalf("Workspace not ready: %v", err)
	}

	// Create multiple files concurrently
	t.Run("Create multiple files", func(t *testing.T) {
		done := make(chan bool, 3)

		for i := 1; i <= 3; i++ {
			go func(idx int) {
				defer func() { done <- true }()

				resp, body := client.POST(t, fmt.Sprintf("/workspace/projects/%s/files", projectID), map[string]interface{}{
					"path":    fmt.Sprintf("concurrent-file-%d.txt", idx),
					"content": fmt.Sprintf("Content for file %d", idx),
				})

				assert.Equal(t, 200, resp.StatusCode, "SaveFile should succeed, body: %s", string(body))
			}(i)
		}

		// Wait for all goroutines
		for i := 0; i < 3; i++ {
			<-done
		}
	})

	// Verify all files exist
	t.Run("Verify all files", func(t *testing.T) {
		for i := 1; i <= 3; i++ {
			resp, body := client.GET(t, fmt.Sprintf("/workspace/projects/%s/files?path=concurrent-file-%d.txt", projectID, i))
			require.Equal(t, 200, resp.StatusCode, "File %d should exist, body: %s", i, string(body))

			var result map[string]interface{}
			client.ParseJSON(t, body, &result)

			file := result["file"].(map[string]interface{})
			assert.Equal(t, fmt.Sprintf("Content for file %d", i), file["content"])
		}
	})
}

// TestDirectGRPCFileOperations tests file operations directly via gRPC
func TestDirectGRPCFileOperations(t *testing.T) {
	// This test directly calls the workspace-service gRPC API
	// instead of going through the API gateway

	// Connect to workspace-service
	conn, err := grpc.Dial("localhost:50052", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	wsClient := proto.NewWorkspaceServiceClient(conn)
	ctx := context.Background()

	// Create session via API gateway (for simplicity)
	client := helpers.NewHTTPClient()

	// Create a user for this test to get a valid token and user ID
	// We need a unique email
	grpcEmail := fmt.Sprintf("test-user-grpc-%d@example.com", time.Now().UnixNano())
	resp, body := client.POST(t, "/auth/signup", map[string]string{
		"email":    grpcEmail,
		"password": "testpassword123",
	})
	require.Equal(t, 200, resp.StatusCode)
	var authResult map[string]interface{}
	client.ParseJSON(t, body, &authResult)
	client.SetAuthToken(authResult["token"].(string))

	// Extract UserID from auth response (JWT)
	tokenString := authResult["token"].(string)
	token, _ := jwt.Parse(tokenString, nil)
	claims := token.Claims.(jwt.MapClaims)
	userID := claims["sub"].(string)

	// Create project via gRPC with the correct UserID
	createResp, err := wsClient.CreateProject(ctx, &proto.CreateProjectRequest{
		Name:        "gRPC Test Project",
		Description: "Testing direct gRPC calls",
		UserId:      userID,
	})
	require.NoError(t, err)
	projectID := createResp.Project.Id

	resp, body = client.POST(t, "/workspace/sessions", map[string]interface{}{
		"projectId":  projectID,
		"gitRepoUrl": "https://github.com/octocat/Hello-World.git",
	})
	require.Equal(t, 202, resp.StatusCode)

	var sessionResult map[string]interface{}
	json.Unmarshal(body, &sessionResult)
	sessionID := sessionResult["sessionId"].(string)

	// Wait for session to be ready
	if err := client.WaitForSessionReady(t, projectID, 120*time.Second); err != nil {
		t.Fatalf("Workspace not ready: %v", err)
	}


	// Test SaveFile via gRPC
	t.Run("SaveFile via gRPC", func(t *testing.T) {
		_, err := wsClient.SaveFile(ctx, &proto.SaveFileRequest{
			ProjectId: projectID,
			Path:      "grpc-test.txt",
			Content:   "Content via gRPC",
			UserId:    userID,
		})
		require.NoError(t, err, "SaveFile should succeed")
	})

	// Test GetFile via gRPC
	t.Run("GetFile via gRPC", func(t *testing.T) {
		getResp, err := wsClient.GetFile(ctx, &proto.GetFileRequest{
			ProjectId: projectID,
			Path:      "grpc-test.txt",
			UserId:    "test-user-grpc",
		})
		require.NoError(t, err, "GetFile should succeed")
		assert.Equal(t, "Content via gRPC", getResp.File.Content)
	})

	// Test GetFileTree via gRPC
	t.Run("GetFileTree via gRPC", func(t *testing.T) {
		treeResp, err := wsClient.GetFileTree(ctx, &proto.GetFileTreeRequest{
			ProjectId: projectID,
			UserId:    userID,
		})
		require.NoError(t, err, "GetFileTree should succeed")
		assert.Greater(t, len(treeResp.Nodes), 0, "Should have nodes in tree")

		t.Logf("File tree has %d root nodes", len(treeResp.Nodes))
	})

	// Cleanup
	client.DELETE(t, fmt.Sprintf("/workspace/sessions/%s?projectId=%s", sessionID, projectID))
	wsClient.DeleteProject(ctx, &proto.DeleteProjectRequest{
		Id:     projectID,
		UserId: userID,
	})
}
