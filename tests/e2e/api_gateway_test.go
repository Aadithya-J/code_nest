//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Aadithya-J/code_nest/tests/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMain ensures services are running before tests
func TestMain(m *testing.M) {
	// Wait for services to be available
	services := map[string]string{
		"API Gateway":       "http://localhost:8080/protected",
		"Auth Service JWKS": "http://localhost:8081/.well-known/jwks.json",
	}

	for name, url := range services {
		if err := helpers.WaitForService(&testing.T{}, url, 30*time.Second); err != nil {
			fmt.Printf("⚠️  Warning: %s not available: %v\n", name, err)
		}
	}

	m.Run()
}

// TestAuthFlow tests complete authentication flow
func TestAuthFlow(t *testing.T) {
	client := helpers.NewHTTPClient()

	// Generate unique email for this test
	timestamp := time.Now().UnixNano()
	email := fmt.Sprintf("test%d@example.com", timestamp)
	password := "testpassword123"

	t.Run("Signup", func(t *testing.T) {
		resp, body := client.POST(t, "/auth/signup", map[string]string{
			"email":    email,
			"password": password,
		})

		require.Equal(t, 200, resp.StatusCode, "Signup should succeed, body: %s", string(body))

		var result map[string]interface{}
		client.ParseJSON(t, body, &result)

		require.NotEmpty(t, result["token"], "Should return a token")
		client.SetAuthToken(result["token"].(string))
	})

	t.Run("Login", func(t *testing.T) {
		resp, body := client.POST(t, "/auth/login", map[string]string{
			"email":    email,
			"password": password,
		})

		require.Equal(t, 200, resp.StatusCode, "Login should succeed, body: %s", string(body))

		var result map[string]interface{}
		client.ParseJSON(t, body, &result)

		require.NotEmpty(t, result["token"], "Should return a token")
		client.SetAuthToken(result["token"].(string))
	})

	t.Run("Login with wrong password", func(t *testing.T) {
		resp, _ := client.POST(t, "/auth/login", map[string]string{
			"email":    email,
			"password": "wrongpassword",
		})

		assert.Equal(t, 401, resp.StatusCode, "Should return unauthorized")
	})

	t.Run("Access protected route", func(t *testing.T) {
		resp, body := client.GET(t, "/protected")

		require.Equal(t, 200, resp.StatusCode, "Protected route should be accessible, body: %s", string(body))

		var result map[string]interface{}
		client.ParseJSON(t, body, &result)

		assert.Contains(t, result["message"], email, "Should contain user email in message")
	})
}

// TestProjectManagement tests complete project CRUD operations
func TestProjectManagement(t *testing.T) {
	client := helpers.NewHTTPClient()

	// Setup: Create a test user and login
	timestamp := time.Now().UnixNano()
	email := fmt.Sprintf("project-test%d@example.com", timestamp)
	password := "testpassword123"

	resp, body := client.POST(t, "/auth/signup", map[string]string{
		"email":    email,
		"password": password,
	})
	require.Equal(t, 200, resp.StatusCode)

	var authResult map[string]interface{}
	client.ParseJSON(t, body, &authResult)
	client.SetAuthToken(authResult["token"].(string))

	var projectID string

	t.Run("Create project", func(t *testing.T) {
		resp, body := client.POST(t, "/workspace/projects", map[string]interface{}{
			"name":        "Test Project",
			"description": "A test project for E2E testing",
		})

		require.Equal(t, 200, resp.StatusCode, "Project creation should succeed, body: %s", string(body))

		var result map[string]interface{}
		client.ParseJSON(t, body, &result)

		project := result["project"].(map[string]interface{})
		projectID = project["id"].(string)

		assert.NotEmpty(t, projectID, "Should return project ID")
		assert.Equal(t, "Test Project", project["name"])
		assert.Equal(t, "A test project for E2E testing", project["description"])
	})

	t.Run("Get all projects", func(t *testing.T) {
		resp, body := client.GET(t, "/workspace/projects")

		require.Equal(t, 200, resp.StatusCode, "Get projects should succeed, body: %s", string(body))

		var result map[string]interface{}
		client.ParseJSON(t, body, &result)

		projects := result["projects"].([]interface{})
		assert.GreaterOrEqual(t, len(projects), 1, "Should have at least one project")
	})

	t.Run("Update project", func(t *testing.T) {
		resp, body := client.PUT(t, "/workspace/projects/"+projectID, map[string]interface{}{
			"name":        "Updated Test Project",
			"description": "Updated description",
		})

		require.Equal(t, 200, resp.StatusCode, "Project update should succeed, body: %s", string(body))

		var result map[string]interface{}
		client.ParseJSON(t, body, &result)

		project := result["project"].(map[string]interface{})
		assert.Equal(t, "Updated Test Project", project["name"])
		assert.Equal(t, "Updated description", project["description"])
	})

	t.Run("Delete project", func(t *testing.T) {
		resp, body := client.DELETE(t, "/workspace/projects/"+projectID)

		require.Equal(t, 200, resp.StatusCode, "Project deletion should succeed, body: %s", string(body))

		var result map[string]interface{}
		client.ParseJSON(t, body, &result)

		assert.True(t, result["success"].(bool), "Deletion should be successful")
	})
}

// TestFileManagement tests file operations within a project
func TestFileManagement(t *testing.T) {
	client := helpers.NewHTTPClient()

	// Setup: Create user, login, and create project
	// DEV cleanup to ensure no leftover active sessions from previous tests
	// This keeps tests idempotent without requiring external scripts.
	cleanupResp, cleanupBody := client.POST(t, "/dev/cleanup", nil)
	t.Logf("DEV cleanup before TestFileManagement: status=%d body=%s", cleanupResp.StatusCode, string(cleanupBody))
	timestamp := time.Now().UnixNano()
	email := fmt.Sprintf("file-test%d@example.com", timestamp)
	password := "testpassword123"

	resp, body := client.POST(t, "/auth/signup", map[string]string{
		"email":    email,
		"password": password,
	})
	require.Equal(t, 200, resp.StatusCode)

	var authResult map[string]interface{}
	client.ParseJSON(t, body, &authResult)
	client.SetAuthToken(authResult["token"].(string))

	// Create a project
	resp, body = client.POST(t, "/workspace/projects", map[string]interface{}{
		"name":        "File Test Project",
		"description": "Project for file testing",
	})
	require.Equal(t, 200, resp.StatusCode)

	var projectResult map[string]interface{}
	client.ParseJSON(t, body, &projectResult)
	project := projectResult["project"].(map[string]interface{})
	projectID := project["id"].(string)

	// Create a workspace session (required for file operations)
	resp, body = client.POST(t, "/workspace/sessions", map[string]interface{}{
		"projectId":  projectID,
		"gitRepoUrl": "https://github.com/octocat/Hello-World.git",
	})
	require.Equal(t, 202, resp.StatusCode, "Session creation should be accepted")
	
	// Wait for session to be ready
	if err := client.WaitForSessionReady(t, projectID, 120*time.Second); err != nil {
		t.Fatalf("Workspace not ready: %v", err)
	}


	t.Run("Save file", func(t *testing.T) {
		resp, body := client.POST(t, "/workspace/files", map[string]interface{}{
			"projectId": projectID,
			"path":      "/main.go",
			"content":   "package main\n\nfunc main() {\n\tprintln(\"Hello World\")\n}",
		})

		require.Equal(t, 200, resp.StatusCode, "File save should succeed, body: %s", string(body))

		var result map[string]interface{}
		client.ParseJSON(t, body, &result)

		file := result["file"].(map[string]interface{})
		assert.Equal(t, "/main.go", file["path"])
	})

	t.Run("Get file", func(t *testing.T) {
		resp, body := client.GET(t, fmt.Sprintf("/workspace/file?projectId=%s&path=/main.go", projectID))

		require.Equal(t, 200, resp.StatusCode, "Get file should succeed, body: %s", string(body))

		var result map[string]interface{}
		client.ParseJSON(t, body, &result)

		file := result["file"].(map[string]interface{})
		assert.Equal(t, "/main.go", file["path"])
		assert.Contains(t, file["content"], "package main")
	})

	t.Run("List files", func(t *testing.T) {
		// Save another file
		client.POST(t, "/workspace/files", map[string]interface{}{
			"projectId": projectID,
			"path":      "/README.md",
			"content":   "# Test Project",
		})

		resp, body := client.GET(t, fmt.Sprintf("/workspace/files?projectId=%s", projectID))

		require.Equal(t, 200, resp.StatusCode, "List files should succeed, body: %s", string(body))

		var result map[string]interface{}
		client.ParseJSON(t, body, &result)

		if result["files"] == nil {
			t.Fatal("files array is nil")
		}
		files := result["files"].([]interface{})
		assert.GreaterOrEqual(t, len(files), 2, "Should have at least 2 files")
	})

	// Cleanup
	t.Cleanup(func() {
		client.DELETE(t, "/workspace/projects/"+projectID)
	})
}

// TestWorkspaceSession tests workspace session creation and release
func TestWorkspaceSession(t *testing.T) {
	client := helpers.NewHTTPClient()

	// Setup: Create user, login, and create project
	timestamp := time.Now().UnixNano()
	email := fmt.Sprintf("session-test%d@example.com", timestamp)
	password := "testpassword123"

	resp, body := client.POST(t, "/auth/signup", map[string]string{
		"email":    email,
		"password": password,
	})
	require.Equal(t, 200, resp.StatusCode)

	var authResult map[string]interface{}
	client.ParseJSON(t, body, &authResult)
	client.SetAuthToken(authResult["token"].(string))

	// Create a project
	resp, body = client.POST(t, "/workspace/projects", map[string]interface{}{
		"name":        "Session Test Project",
		"description": "Project for session testing",
	})
	require.Equal(t, 200, resp.StatusCode)

	var projectResult map[string]interface{}
	client.ParseJSON(t, body, &projectResult)
	project := projectResult["project"].(map[string]interface{})
	projectID := project["id"].(string)

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
		assert.Equal(t, "CREATING", result["status"], "Status should be CREATING")
	})

	t.Run("Release workspace session", func(t *testing.T) {
		// Wait a bit for session to be processed
		time.Sleep(2 * time.Second)

		resp, body := client.DELETE(t, fmt.Sprintf("/workspace/sessions/%s?projectId=%s", sessionID, projectID))

		require.Equal(t, 200, resp.StatusCode, "Session release should succeed, body: %s", string(body))

		var result map[string]interface{}
		client.ParseJSON(t, body, &result)

		assert.Equal(t, sessionID, result["sessionId"])
		assert.Equal(t, "RELEASING", result["status"])
	})

	// Cleanup
	t.Cleanup(func() {
		client.DELETE(t, "/workspace/projects/"+projectID)
	})
}

// TestUnauthorizedAccess tests that protected routes require authentication
func TestUnauthorizedAccess(t *testing.T) {
	client := helpers.NewHTTPClient()
	// Don't set auth token

	protectedRoutes := []struct {
		method string
		path   string
	}{
		{"GET", "/protected"},
		{"POST", "/workspace/projects"},
		{"GET", "/workspace/projects"},
		{"POST", "/workspace/files"},
		{"GET", "/workspace/files?projectId=test"},
		{"POST", "/workspace/sessions"},
	}

	for _, route := range protectedRoutes {
		t.Run(fmt.Sprintf("%s %s without auth", route.method, route.path), func(t *testing.T) {
			var resp *http.Response
			switch route.method {
			case "GET":
				resp, _ = client.GET(t, route.path)
			case "POST":
				resp, _ = client.POST(t, route.path, map[string]string{})
			}

			assert.Equal(t, 401, resp.StatusCode, "Should return 401 Unauthorized")
		})
	}
}

// TestCompleteUserWorkflow tests a complete user workflow from signup to workspace
func TestCompleteUserWorkflow(t *testing.T) {
	client := helpers.NewHTTPClient()
	timestamp := time.Now().UnixNano()
	email := fmt.Sprintf("workflow-test%d@example.com", timestamp)
	password := "testpassword123"

	// Step 1: Signup
	t.Log("Step 1: User signs up")
	resp, body := client.POST(t, "/auth/signup", map[string]string{
		"email":    email,
		"password": password,
	})
	require.Equal(t, 200, resp.StatusCode)

	var authResult map[string]interface{}
	client.ParseJSON(t, body, &authResult)
	client.SetAuthToken(authResult["token"].(string))

	// Step 2: Create a project
	t.Log("Step 2: User creates a project")
	resp, body = client.POST(t, "/workspace/projects", map[string]interface{}{
		"name":        "My Awesome App",
		"description": "Building something cool",
	})
	require.Equal(t, 200, resp.StatusCode)

	var projectResult map[string]interface{}
	client.ParseJSON(t, body, &projectResult)
	project := projectResult["project"].(map[string]interface{})
	projectID := project["id"].(string)

	// Step 3: Add files to the project
	t.Log("Step 3: User adds files to the project")
	files := []map[string]string{
		{
			"path":    "/index.html",
			"content": "<html><body><h1>Hello World</h1></body></html>",
		},
		{
			"path":    "/app.js",
			"content": "console.log('Hello from JavaScript');",
		},
		{
			"path":    "/styles.css",
			"content": "body { font-family: Arial; }",
		},
	}

	for _, file := range files {
		resp, _ = client.POST(t, "/workspace/files", map[string]interface{}{
			"projectId": projectID,
			"path":      file["path"],
			"content":   file["content"],
		})
		require.Equal(t, 200, resp.StatusCode)
	}

	// Step 4: List all files
	t.Log("Step 4: User lists all files")
	resp, body = client.GET(t, fmt.Sprintf("/workspace/files?projectId=%s", projectID))
	require.Equal(t, 200, resp.StatusCode)

	var filesResult map[string]interface{}
	client.ParseJSON(t, body, &filesResult)
	filesList := filesResult["files"].([]interface{})
	assert.Equal(t, 3, len(filesList), "Should have 3 files")

	// Step 5: Request a workspace session
	t.Log("Step 5: User requests a live workspace session")
	resp, body = client.POST(t, "/workspace/sessions", map[string]interface{}{
		"projectId":  projectID,
		"gitRepoUrl": "https://github.com/octocat/Hello-World.git",
	})
	require.Equal(t, 202, resp.StatusCode)

	var sessionResult map[string]interface{}
	client.ParseJSON(t, body, &sessionResult)
	sessionID := sessionResult["sessionId"].(string)
	assert.NotEmpty(t, sessionID)

	// Step 6: Update project details
	t.Log("Step 6: User updates project details")
	resp, _ = client.PUT(t, "/workspace/projects/"+projectID, map[string]interface{}{
		"name":        "My Awesome App v2",
		"description": "Updated with new features",
	})
	require.Equal(t, 200, resp.StatusCode)

	// Step 7: Clean up - Release session
	t.Log("Step 7: User releases the workspace session")
	time.Sleep(2 * time.Second)
	resp, _ = client.DELETE(t, fmt.Sprintf("/workspace/sessions/%s?projectId=%s", sessionID, projectID))
	assert.Equal(t, 200, resp.StatusCode)

	// Step 8: Delete project
	t.Log("Step 8: User deletes the project")
	resp, _ = client.DELETE(t, "/workspace/projects/"+projectID)
	require.Equal(t, 200, resp.StatusCode)

	t.Log("✅ Complete workflow test passed!")
}
