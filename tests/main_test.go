package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"testing"
	"time"

	"github.com/Aadithya-J/code_nest/proto"
	"github.com/stretchr/testify/suite"
)

const (
	apiGatewayURL = "http://localhost:8080"
	// Use a unique email for each test run to avoid conflicts.
	testEmail    = "testuser@example.com"
	testPassword = "password123"
)

type E2ETestSuite struct {
	suite.Suite
	client *http.Client
}

func (s *E2ETestSuite) SetupSuite() {
	s.client = &http.Client{Timeout: 10 * time.Second}

	// Wait for the API Gateway to be ready before running tests.
	maxWait := 15 * time.Second
	checkInterval := 1 * time.Second
	startTime := time.Now()

	for {
		// Use a simple health check endpoint (e.g., /ping or any known route)
		// For now, we'll just try to connect to the base URL.
		resp, err := http.Get(apiGatewayURL + "/auth/google/login") // Using a known GET endpoint for a health check
		if err == nil && resp.StatusCode < 500 {
			log.Println("API Gateway is up!")
			resp.Body.Close()
			return
		}

		if time.Since(startTime) > maxWait {
			s.T().Fatalf("API Gateway did not become ready in %s", maxWait)
		}

		log.Printf("Waiting for API Gateway to be ready... retrying in %s", checkInterval)
		time.Sleep(checkInterval)
	}
}

// TestSignupEndpoint tests the POST /auth/signup endpoint.
func (s *E2ETestSuite) TestSignupEndpoint() {
	// Use a unique email for each test run to ensure idempotency
	uniqueEmail := fmt.Sprintf("user_%d@example.com", time.Now().UnixNano())

	signupData := map[string]string{
		"email":    uniqueEmail,
		"password": testPassword,
	}
	jsonData, err := json.Marshal(signupData)
	s.NoError(err)

	req, err := http.NewRequest("POST", apiGatewayURL+"/auth/signup", bytes.NewBuffer(jsonData))
	s.NoError(err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	s.NoError(err)
	defer resp.Body.Close()

	s.Equal(http.StatusOK, resp.StatusCode, "Expected status code 200 OK")

	var authResponse map[string]string
	err = json.NewDecoder(resp.Body).Decode(&authResponse)
	s.NoError(err)

	s.NotEmpty(authResponse["token"], "Response should contain a token")
}

// TestLoginEndpoint tests the POST /auth/login endpoint with valid credentials.
func (s *E2ETestSuite) TestLoginEndpoint() {
	// First, create a user via signup
	uniqueEmail := fmt.Sprintf("loginuser_%d@example.com", time.Now().UnixNano())
	
	signupData := map[string]string{
		"email":    uniqueEmail,
		"password": testPassword,
	}
	jsonData, err := json.Marshal(signupData)
	s.NoError(err)

	// Create the user
	req, err := http.NewRequest("POST", apiGatewayURL+"/auth/signup", bytes.NewBuffer(jsonData))
	s.NoError(err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	s.NoError(err)
	resp.Body.Close()
	s.Equal(http.StatusOK, resp.StatusCode, "Signup should succeed")

	// Now test login with the same credentials
	loginData := map[string]string{
		"email":    uniqueEmail,
		"password": testPassword,
	}
	loginJSON, err := json.Marshal(loginData)
	s.NoError(err)

	loginReq, err := http.NewRequest("POST", apiGatewayURL+"/auth/login", bytes.NewBuffer(loginJSON))
	s.NoError(err)
	loginReq.Header.Set("Content-Type", "application/json")

	loginResp, err := s.client.Do(loginReq)
	s.NoError(err)
	defer loginResp.Body.Close()

	s.Equal(http.StatusOK, loginResp.StatusCode, "Login should succeed with valid credentials")

	var loginResponse map[string]string
	err = json.NewDecoder(loginResp.Body).Decode(&loginResponse)
	s.NoError(err)

	s.NotEmpty(loginResponse["token"], "Login response should contain a token")
}

// TestLoginFailure tests the POST /auth/login endpoint with invalid credentials.
func (s *E2ETestSuite) TestLoginFailure() {
	// First, create a user via signup
	uniqueEmail := fmt.Sprintf("failuser_%d@example.com", time.Now().UnixNano())
	
	signupData := map[string]string{
		"email":    uniqueEmail,
		"password": testPassword,
	}
	jsonData, err := json.Marshal(signupData)
	s.NoError(err)

	// Create the user
	req, err := http.NewRequest("POST", apiGatewayURL+"/auth/signup", bytes.NewBuffer(jsonData))
	s.NoError(err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	s.NoError(err)
	resp.Body.Close()
	s.Equal(http.StatusOK, resp.StatusCode, "Signup should succeed")

	// Now test login with wrong password
	loginData := map[string]string{
		"email":    uniqueEmail,
		"password": "wrongpassword",
	}
	loginJSON, err := json.Marshal(loginData)
	s.NoError(err)

	loginReq, err := http.NewRequest("POST", apiGatewayURL+"/auth/login", bytes.NewBuffer(loginJSON))
	s.NoError(err)
	loginReq.Header.Set("Content-Type", "application/json")

	loginResp, err := s.client.Do(loginReq)
	s.NoError(err)
	defer loginResp.Body.Close()

	s.Equal(http.StatusUnauthorized, loginResp.StatusCode, "Login should fail with invalid credentials")
}

// TestProtectedRoute tests accessing a protected endpoint with a valid JWT token.
func (s *E2ETestSuite) TestProtectedRoute() {
	// First, create a user and get a token
	uniqueEmail := fmt.Sprintf("protecteduser_%d@example.com", time.Now().UnixNano())
	
	signupData := map[string]string{
		"email":    uniqueEmail,
		"password": testPassword,
	}
	jsonData, err := json.Marshal(signupData)
	s.NoError(err)

	// Create the user
	req, err := http.NewRequest("POST", apiGatewayURL+"/auth/signup", bytes.NewBuffer(jsonData))
	s.NoError(err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	s.NoError(err)
	defer resp.Body.Close()
	s.Equal(http.StatusOK, resp.StatusCode, "Signup should succeed")

	var authResponse map[string]string
	err = json.NewDecoder(resp.Body).Decode(&authResponse)
	s.NoError(err)
	token := authResponse["token"]
	s.NotEmpty(token, "Should receive a token")

	// Now test accessing a protected route with the token
	protectedReq, err := http.NewRequest("GET", apiGatewayURL+"/workspace/projects", nil)
	s.NoError(err)
	protectedReq.Header.Set("Authorization", "Bearer "+token)

	protectedResp, err := s.client.Do(protectedReq)
	s.NoError(err)
	defer protectedResp.Body.Close()

	s.Equal(http.StatusOK, protectedResp.StatusCode, "Protected route should be accessible with valid token")
}

// TestInvalidToken tests accessing a protected endpoint with invalid/missing token.
// TestFileManagementEndpoints tests the full lifecycle of file management.
func (s *E2ETestSuite) TestFileManagementEndpoints() {
	// 1. Signup to get a token
	uniqueEmail := fmt.Sprintf("fileuser_%d@example.com", time.Now().UnixNano())
	signupData := map[string]string{"email": uniqueEmail, "password": testPassword}
	signupBody, _ := json.Marshal(signupData)
	signupReq, _ := http.NewRequest("POST", apiGatewayURL+"/auth/signup", bytes.NewBuffer(signupBody))
	signupReq.Header.Set("Content-Type", "application/json")
	signupResp, err := s.client.Do(signupReq)
	s.NoError(err)
	defer signupResp.Body.Close()
	s.Equal(http.StatusOK, signupResp.StatusCode)
	var authResponse map[string]string
	json.NewDecoder(signupResp.Body).Decode(&authResponse)
	token := authResponse["token"]
	s.NotEmpty(token)

	// 2. Create a project
	projectData := map[string]string{"name": "Test Project for Files"}
	projectBody, _ := json.Marshal(projectData)
	createProjectReq, _ := http.NewRequest("POST", apiGatewayURL+"/workspace/projects", bytes.NewBuffer(projectBody))
	createProjectReq.Header.Set("Content-Type", "application/json")
	createProjectReq.Header.Set("Authorization", "Bearer "+token)
	createProjectResp, err := s.client.Do(createProjectReq)
	s.NoError(err)
	defer createProjectResp.Body.Close()
	s.Equal(http.StatusOK, createProjectResp.StatusCode)
	var projectResponse proto.ProjectResponse
	json.NewDecoder(createProjectResp.Body).Decode(&projectResponse)
	projectID := projectResponse.Project.Id
	s.NotEmpty(projectID)

	// 3. Save a file to the project
	fileData := map[string]string{
		"projectId": projectID,
		"path":      "main.go",
		"content":   "package main; func main() {}",
	}
	fileBody, _ := json.Marshal(fileData)
	saveFileReq, _ := http.NewRequest("POST", apiGatewayURL+"/workspace/files", bytes.NewBuffer(fileBody))
	saveFileReq.Header.Set("Content-Type", "application/json")
	saveFileReq.Header.Set("Authorization", "Bearer "+token)
	saveFileResp, err := s.client.Do(saveFileReq)
	s.NoError(err)
	defer saveFileResp.Body.Close()
	s.Equal(http.StatusOK, saveFileResp.StatusCode)

	// 4. Get the file back
	getFileReq, _ := http.NewRequest("GET", fmt.Sprintf("%s/workspace/file?projectId=%s&path=main.go", apiGatewayURL, projectID), nil)
	getFileReq.Header.Set("Authorization", "Bearer "+token)
	getFileResp, err := s.client.Do(getFileReq)
	s.NoError(err)
	defer getFileResp.Body.Close()
	s.Equal(http.StatusOK, getFileResp.StatusCode)
	var fileGetResponse proto.FileResponse
	json.NewDecoder(getFileResp.Body).Decode(&fileGetResponse)
	s.Equal("package main; func main() {}", fileGetResponse.File.Content)

	// 5. List files in the project
	listFilesReq, _ := http.NewRequest("GET", fmt.Sprintf("%s/workspace/files?projectId=%s", apiGatewayURL, projectID), nil)
	listFilesReq.Header.Set("Authorization", "Bearer "+token)
	listFilesResp, err := s.client.Do(listFilesReq)
	s.NoError(err)
	defer listFilesResp.Body.Close()
	s.Equal(http.StatusOK, listFilesResp.StatusCode)
	var listFilesResponse proto.ListFilesResponse
	json.NewDecoder(listFilesResp.Body).Decode(&listFilesResponse)
	s.Len(listFilesResponse.Files, 1)
	s.Equal("main.go", listFilesResponse.Files[0].Path)
}

func (s *E2ETestSuite) TestInvalidToken() {
	// Test 1: No Authorization header
	req1, err := http.NewRequest("GET", apiGatewayURL+"/workspace/projects", nil)
	s.NoError(err)

	resp1, err := s.client.Do(req1)
	s.NoError(err)
	defer resp1.Body.Close()

	s.Equal(http.StatusUnauthorized, resp1.StatusCode, "Should reject request without Authorization header")

	// Test 2: Invalid token format (missing "Bearer ")
	req2, err := http.NewRequest("GET", apiGatewayURL+"/workspace/projects", nil)
	s.NoError(err)
	req2.Header.Set("Authorization", "invalidtoken")

	resp2, err := s.client.Do(req2)
	s.NoError(err)
	defer resp2.Body.Close()

	s.Equal(http.StatusUnauthorized, resp2.StatusCode, "Should reject request with invalid token format")

	// Test 3: Invalid JWT token
	req3, err := http.NewRequest("GET", apiGatewayURL+"/workspace/projects", nil)
	s.NoError(err)
	req3.Header.Set("Authorization", "Bearer invalid.jwt.token")

	resp3, err := s.client.Do(req3)
	s.NoError(err)
	defer resp3.Body.Close()

	s.Equal(http.StatusUnauthorized, resp3.StatusCode, "Should reject request with invalid JWT token")
}

func TestE2ESuite(t *testing.T) {
	suite.Run(t, new(E2ETestSuite))
}
