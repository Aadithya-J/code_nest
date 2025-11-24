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

// setupAuthenticatedClient creates a new user and returns an authenticated client
func setupAuthenticatedClient(t *testing.T, prefix string) (*helpers.HTTPClient, string) {
	t.Helper()

	client := helpers.NewHTTPClient()
	timestamp := time.Now().UnixNano()
	email := fmt.Sprintf("%s-%d@example.com", prefix, timestamp)
	password := "testpassword123"

	resp, body := client.POST(t, "/auth/signup", map[string]string{
		"email":    email,
		"password": password,
	})
	require.Equal(t, 200, resp.StatusCode, "Signup failed: %s", string(body))

	var authResult map[string]interface{}
	client.ParseJSON(t, body, &authResult)
	client.SetAuthToken(authResult["token"].(string))

	return client, email
}

// TestProjectCreate tests project creation
func TestProjectCreate(t *testing.T) {
	client, _ := setupAuthenticatedClient(t, "project-create")

	tests := []struct {
		name           string
		projectName    string
		description    string
		expectedStatus int
		shouldReturnID bool
	}{
		{
			name:           "Valid project creation",
			projectName:    "Test Project",
			description:    "A test project",
			expectedStatus: 200,
			shouldReturnID: true,
		},
		{
			name:           "Project with empty description",
			projectName:    "Minimal Project",
			description:    "",
			expectedStatus: 200,
			shouldReturnID: true,
		},
		{
			name:           "Project with long description",
			projectName:    "Detailed Project",
			description:    "This is a very long description that contains a lot of information about the project and its goals and objectives.",
			expectedStatus: 200,
			shouldReturnID: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, body := client.POST(t, "/workspace/projects", map[string]interface{}{
				"name":        tt.projectName,
				"description": tt.description,
			})

			assert.Equal(t, tt.expectedStatus, resp.StatusCode, "Response body: %s", string(body))

			if tt.shouldReturnID {
				var result map[string]interface{}
				client.ParseJSON(t, body, &result)

				project := result["project"].(map[string]interface{})
				assert.NotEmpty(t, project["id"], "Should return project ID")
				assert.Equal(t, tt.projectName, project["name"])
				// Description might be nil for empty string, that's OK
				if tt.description != "" {
					assert.Equal(t, tt.description, project["description"])
				}
			}
		})
	}
}

// TestProjectCRUD tests complete project lifecycle
func TestProjectCRUD(t *testing.T) {
	client, userEmail := setupAuthenticatedClient(t, "project-crud")

	var projectID string

	t.Run("Create project", func(t *testing.T) {
		resp, body := client.POST(t, "/workspace/projects", map[string]interface{}{
			"name":        "CRUD Test Project",
			"description": "Testing full CRUD operations",
		})

		require.Equal(t, 200, resp.StatusCode, "Response body: %s", string(body))

		var result map[string]interface{}
		client.ParseJSON(t, body, &result)

		project := result["project"].(map[string]interface{})
		projectID = project["id"].(string)

		assert.NotEmpty(t, projectID)
		assert.Equal(t, "CRUD Test Project", project["name"])
		assert.Equal(t, userEmail, project["userId"])
	})

	t.Run("Get all projects", func(t *testing.T) {
		resp, body := client.GET(t, "/workspace/projects")

		require.Equal(t, 200, resp.StatusCode, "Response body: %s", string(body))

		var result map[string]interface{}
		client.ParseJSON(t, body, &result)

		projects := result["projects"].([]interface{})
		assert.GreaterOrEqual(t, len(projects), 1, "Should have at least one project")

		// Verify our project is in the list
		found := false
		for _, p := range projects {
			proj := p.(map[string]interface{})
			if proj["id"].(string) == projectID {
				found = true
				assert.Equal(t, "CRUD Test Project", proj["name"])
				break
			}
		}
		assert.True(t, found, "Created project should be in the list")
	})

	t.Run("Update project", func(t *testing.T) {
		resp, body := client.PUT(t, "/workspace/projects/"+projectID, map[string]interface{}{
			"name":        "Updated CRUD Project",
			"description": "Updated description",
		})

		require.Equal(t, 200, resp.StatusCode, "Response body: %s", string(body))

		var result map[string]interface{}
		client.ParseJSON(t, body, &result)

		project := result["project"].(map[string]interface{})
		assert.Equal(t, projectID, project["id"])
		assert.Equal(t, "Updated CRUD Project", project["name"])
		assert.Equal(t, "Updated description", project["description"])
	})

	t.Run("Delete project", func(t *testing.T) {
		resp, body := client.DELETE(t, "/workspace/projects/"+projectID)

		require.Equal(t, 200, resp.StatusCode, "Response body: %s", string(body))

		var result map[string]interface{}
		client.ParseJSON(t, body, &result)

		assert.True(t, result["success"].(bool), "Deletion should succeed")
	})

	t.Run("Verify deletion", func(t *testing.T) {
		resp, body := client.GET(t, "/workspace/projects")

		require.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		client.ParseJSON(t, body, &result)

		// Handle empty/nil projects list
		if result["projects"] != nil {
			projects := result["projects"].([]interface{})

			// Verify our project is NOT in the list
			for _, p := range projects {
				proj := p.(map[string]interface{})
				assert.NotEqual(t, projectID, proj["id"], "Deleted project should not be in the list")
			}
		}
	})
}

// TestProjectUnauthorizedAccess tests authorization on project operations
func TestProjectUnauthorizedAccess(t *testing.T) {
	client1, _ := setupAuthenticatedClient(t, "project-auth1")
	client2, _ := setupAuthenticatedClient(t, "project-auth2")

	// User 1 creates a project
	resp, body := client1.POST(t, "/workspace/projects", map[string]interface{}{
		"name":        "User1 Project",
		"description": "Only user1 should access",
	})
	require.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	client1.ParseJSON(t, body, &result)
	project := result["project"].(map[string]interface{})
	projectID := project["id"].(string)

	t.Run("User2 cannot update User1's project", func(t *testing.T) {
		resp, _ := client2.PUT(t, "/workspace/projects/"+projectID, map[string]interface{}{
			"name": "Hacked Name",
		})

		// Should fail with permission error
		assert.NotEqual(t, 200, resp.StatusCode, "Should not allow unauthorized update")
	})

	t.Run("User2 cannot delete User1's project", func(t *testing.T) {
		resp, _ := client2.DELETE(t, "/workspace/projects/"+projectID)

		// Should fail with permission error
		assert.NotEqual(t, 200, resp.StatusCode, "Should not allow unauthorized delete")
	})

	// Cleanup
	t.Cleanup(func() {
		client1.DELETE(t, "/workspace/projects/"+projectID)
	})
}

// TestProjectWithoutAuth tests that project routes require authentication
func TestProjectWithoutAuth(t *testing.T) {
	client := helpers.NewHTTPClient()
	// Don't set auth token

	t.Run("Create project without auth", func(t *testing.T) {
		resp, _ := client.POST(t, "/workspace/projects", map[string]interface{}{
			"name": "Should Fail",
		})
		assert.Equal(t, 401, resp.StatusCode)
	})

	t.Run("Get projects without auth", func(t *testing.T) {
		resp, _ := client.GET(t, "/workspace/projects")
		assert.Equal(t, 401, resp.StatusCode)
	})

	t.Run("Update project without auth", func(t *testing.T) {
		resp, _ := client.PUT(t, "/workspace/projects/some-id", map[string]interface{}{
			"name": "Should Fail",
		})
		assert.Equal(t, 401, resp.StatusCode)
	})

	t.Run("Delete project without auth", func(t *testing.T) {
		resp, _ := client.DELETE(t, "/workspace/projects/some-id")
		assert.Equal(t, 401, resp.StatusCode)
	})
}
