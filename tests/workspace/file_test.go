package workspace

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Aadithya-J/code_nest/proto"
	"github.com/Aadithya-J/code_nest/tests/common"
	"github.com/stretchr/testify/suite"
)

type FileTestSuite struct {
	common.BaseE2ETestSuite
	authHelpers *common.AuthHelpers
}

func (s *FileTestSuite) SetupSuite() {
	s.BaseE2ETestSuite.SetupSuite()
	s.authHelpers = common.NewAuthHelpers(&s.BaseE2ETestSuite)
}

func (s *FileTestSuite) TestFileManagement() {
	// Create user and get token
	_, token, err := s.authHelpers.SignupUniqueUser("file_test")
	s.NoError(err)

	// First create a project
	createProjectData := map[string]string{
		"name":        "File Test Project",
		"description": "Project for testing file operations",
	}
	createBody, _ := json.Marshal(createProjectData)

	createReq, err := s.authHelpers.AuthenticatedRequest("POST", common.APIGatewayURL+"/workspace/projects", token, createBody)
	s.NoError(err)

	createResp, err := s.Client.Do(createReq)
	s.NoError(err)
	defer createResp.Body.Close()

	var createResponse proto.ProjectResponse
	json.NewDecoder(createResp.Body).Decode(&createResponse)
	projectID := createResponse.Project.Id

	// Test 1: Save File
	saveFileData := map[string]string{
		"projectId": projectID,
		"path":      "main.go",
		"content":   "package main\n\nfunc main() {\n\tprintln(\"Hello, World!\")\n}",
	}
	saveBody, _ := json.Marshal(saveFileData)

	saveReq, err := s.authHelpers.AuthenticatedRequest("POST", common.APIGatewayURL+"/workspace/files", token, saveBody)
	s.NoError(err)

	saveResp, err := s.Client.Do(saveReq)
	s.NoError(err)
	defer saveResp.Body.Close()

	s.Equal(http.StatusOK, saveResp.StatusCode, "Should save file successfully")

	// Test 2: Get File
	getFileReq, err := s.authHelpers.AuthenticatedRequest("GET", common.APIGatewayURL+"/workspace/file?projectId="+projectID+"&path=main.go", token, nil)
	s.NoError(err)

	getFileResp, err := s.Client.Do(getFileReq)
	s.NoError(err)
	defer getFileResp.Body.Close()

	s.Equal(http.StatusOK, getFileResp.StatusCode, "Should get file successfully")

	var getFileResponse proto.FileResponse
	json.NewDecoder(getFileResp.Body).Decode(&getFileResponse)
	s.Equal("main.go", getFileResponse.File.Path, "Should return correct file path")
	s.Contains(getFileResponse.File.Content, "Hello, World!", "Should return correct file content")

	// Test 3: List Files
	listFilesReq, err := s.authHelpers.AuthenticatedRequest("GET", common.APIGatewayURL+"/workspace/files?projectId="+projectID, token, nil)
	s.NoError(err)

	listFilesResp, err := s.Client.Do(listFilesReq)
	s.NoError(err)
	defer listFilesResp.Body.Close()

	s.Equal(http.StatusOK, listFilesResp.StatusCode, "Should list files successfully")

	var listFilesResponse proto.ListFilesResponse
	json.NewDecoder(listFilesResp.Body).Decode(&listFilesResponse)
	s.Len(listFilesResponse.Files, 1, "Should return one file")
	s.Equal("main.go", listFilesResponse.Files[0].Path, "Should return the saved file")
}

func TestFileSuite(t *testing.T) {
	suite.Run(t, new(FileTestSuite))
}
