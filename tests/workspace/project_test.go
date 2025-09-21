package workspace

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Aadithya-J/code_nest/proto"
	"github.com/Aadithya-J/code_nest/tests/common"
	"github.com/stretchr/testify/suite"
)

type ProjectTestSuite struct {
	common.BaseE2ETestSuite
	authHelpers *common.AuthHelpers
}

func (s *ProjectTestSuite) SetupSuite() {
	s.BaseE2ETestSuite.SetupSuite()
	s.authHelpers = common.NewAuthHelpers(&s.BaseE2ETestSuite)
}

func (s *ProjectTestSuite) TestProjectCRUD() {
	// Create user and get token
	_, token, err := s.authHelpers.SignupUniqueUser("project_test")
	s.NoError(err)

	// Test 1: Create Project
	createProjectData := map[string]string{
		"name":        "Test Project",
		"description": "A test project for E2E testing",
	}
	createBody, _ := json.Marshal(createProjectData)

	createReq, err := s.authHelpers.AuthenticatedRequest("POST", common.APIGatewayURL+"/workspace/projects", token, createBody)
	s.NoError(err)

	createResp, err := s.Client.Do(createReq)
	s.NoError(err)
	defer createResp.Body.Close()

	s.Equal(http.StatusOK, createResp.StatusCode, "Should create project successfully")

	var createResponse proto.ProjectResponse
	json.NewDecoder(createResp.Body).Decode(&createResponse)
	s.NotEmpty(createResponse.Project.Id, "Should return project ID")
	s.Equal("Test Project", createResponse.Project.Name, "Should return correct project name")

	projectID := createResponse.Project.Id

	// Test 2: Get Projects
	getReq, err := s.authHelpers.AuthenticatedRequest("GET", common.APIGatewayURL+"/workspace/projects", token, nil)
	s.NoError(err)

	getResp, err := s.Client.Do(getReq)
	s.NoError(err)
	defer getResp.Body.Close()

	s.Equal(http.StatusOK, getResp.StatusCode, "Should get projects successfully")

	var getResponse proto.GetProjectsResponse
	json.NewDecoder(getResp.Body).Decode(&getResponse)
	s.Len(getResponse.Projects, 1, "Should return one project")
	s.Equal(projectID, getResponse.Projects[0].Id, "Should return the created project")

	// Test 3: Update Project
	updateProjectData := map[string]string{
		"name":        "Updated Test Project",
		"description": "Updated description",
	}
	updateBody, _ := json.Marshal(updateProjectData)

	updateReq, err := s.authHelpers.AuthenticatedRequest("PUT", common.APIGatewayURL+"/workspace/projects/"+projectID, token, updateBody)
	s.NoError(err)

	updateResp, err := s.Client.Do(updateReq)
	s.NoError(err)
	defer updateResp.Body.Close()

	s.Equal(http.StatusOK, updateResp.StatusCode, "Should update project successfully")

	// Test 4: Delete Project
	deleteReq, err := s.authHelpers.AuthenticatedRequest("DELETE", common.APIGatewayURL+"/workspace/projects/"+projectID, token, nil)
	s.NoError(err)

	deleteResp, err := s.Client.Do(deleteReq)
	s.NoError(err)
	defer deleteResp.Body.Close()

	s.Equal(http.StatusOK, deleteResp.StatusCode, "Should delete project successfully")
}

func TestProjectSuite(t *testing.T) {
	suite.Run(t, new(ProjectTestSuite))
}
