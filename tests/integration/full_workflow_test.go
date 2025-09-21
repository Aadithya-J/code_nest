package integration

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Aadithya-J/code_nest/proto"
	"github.com/Aadithya-J/code_nest/tests/common"
	"github.com/stretchr/testify/suite"
)

type FullWorkflowTestSuite struct {
	common.BaseE2ETestSuite
	authHelpers *common.AuthHelpers
}

func (s *FullWorkflowTestSuite) SetupSuite() {
	s.BaseE2ETestSuite.SetupSuite()
	s.authHelpers = common.NewAuthHelpers(&s.BaseE2ETestSuite)
}

func (s *FullWorkflowTestSuite) TestCompleteUserWorkflow() {
	// This test simulates a complete user workflow:
	// 1. User signs up
	// 2. Creates a project
	// 3. Saves files to the project
	// 4. Retrieves files
	// 5. Updates project
	// 6. Cleans up

	// Step 1: User Signup
	email, token, err := s.authHelpers.SignupUniqueUser("workflow_test")
	s.NoError(err)
	s.NotEmpty(token, "Should receive JWT token")
	s.Contains(email, "workflow_test", "Should create user with correct email")

	// Step 2: Create Project
	createProjectData := map[string]string{
		"name":        "My First Project",
		"description": "A complete workflow test project",
	}
	createBody, _ := json.Marshal(createProjectData)

	createReq, err := s.authHelpers.AuthenticatedRequest("POST", common.APIGatewayURL+"/workspace/projects", token, createBody)
	s.NoError(err)

	createResp, err := s.Client.Do(createReq)
	s.NoError(err)
	defer createResp.Body.Close()

	s.Equal(http.StatusOK, createResp.StatusCode, "Should create project")

	var projectResponse proto.ProjectResponse
	json.NewDecoder(createResp.Body).Decode(&projectResponse)
	projectID := projectResponse.Project.Id
	s.NotEmpty(projectID, "Should return project ID")

	// Step 3: Save Multiple Files
	files := []struct {
		path    string
		content string
	}{
		{"main.go", "package main\n\nfunc main() {\n\tprintln(\"Hello, World!\")\n}"},
		{"README.md", "# My First Project\n\nThis is a test project."},
		{"config.json", "{\"name\": \"test\", \"version\": \"1.0.0\"}"},
	}

	for _, file := range files {
		saveFileData := map[string]string{
			"projectId": projectID,
			"path":      file.path,
			"content":   file.content,
		}
		saveBody, _ := json.Marshal(saveFileData)

		saveReq, err := s.authHelpers.AuthenticatedRequest("POST", common.APIGatewayURL+"/workspace/files", token, saveBody)
		s.NoError(err)

		saveResp, err := s.Client.Do(saveReq)
		s.NoError(err)
		defer saveResp.Body.Close()

		s.Equal(http.StatusOK, saveResp.StatusCode, "Should save file: %s", file.path)
	}

	// Step 4: List All Files
	listReq, err := s.authHelpers.AuthenticatedRequest("GET", common.APIGatewayURL+"/workspace/files?projectId="+projectID, token, nil)
	s.NoError(err)

	listResp, err := s.Client.Do(listReq)
	s.NoError(err)
	defer listResp.Body.Close()

	s.Equal(http.StatusOK, listResp.StatusCode, "Should list files")

	var listResponse proto.ListFilesResponse
	json.NewDecoder(listResp.Body).Decode(&listResponse)
	s.Len(listResponse.Files, 3, "Should return all saved files")

	// Step 5: Retrieve Specific File
	getReq, err := s.authHelpers.AuthenticatedRequest("GET", common.APIGatewayURL+"/workspace/file?projectId="+projectID+"&path=main.go", token, nil)
	s.NoError(err)

	getResp, err := s.Client.Do(getReq)
	s.NoError(err)
	defer getResp.Body.Close()

	s.Equal(http.StatusOK, getResp.StatusCode, "Should get specific file")

	var fileResponse proto.FileResponse
	json.NewDecoder(getResp.Body).Decode(&fileResponse)
	s.Equal("main.go", fileResponse.File.Path, "Should return correct file")
	s.Contains(fileResponse.File.Content, "Hello, World!", "Should return correct content")

	// Step 6: Update Project
	updateData := map[string]string{
		"name":        "My Updated Project",
		"description": "Updated description after adding files",
	}
	updateBody, _ := json.Marshal(updateData)

	updateReq, err := s.authHelpers.AuthenticatedRequest("PUT", common.APIGatewayURL+"/workspace/projects/"+projectID, token, updateBody)
	s.NoError(err)

	updateResp, err := s.Client.Do(updateReq)
	s.NoError(err)
	defer updateResp.Body.Close()

	s.Equal(http.StatusOK, updateResp.StatusCode, "Should update project")

	// Step 7: Verify Updated Project
	getProjectsReq, err := s.authHelpers.AuthenticatedRequest("GET", common.APIGatewayURL+"/workspace/projects", token, nil)
	s.NoError(err)

	getProjectsResp, err := s.Client.Do(getProjectsReq)
	s.NoError(err)
	defer getProjectsResp.Body.Close()

	var projectsResponse proto.GetProjectsResponse
	json.NewDecoder(getProjectsResp.Body).Decode(&projectsResponse)
	s.Len(projectsResponse.Projects, 1, "Should have one project")
	s.Equal("My Updated Project", projectsResponse.Projects[0].Name, "Should reflect updated name")
}

func TestFullWorkflowSuite(t *testing.T) {
	suite.Run(t, new(FullWorkflowTestSuite))
}
