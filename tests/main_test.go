package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"testing"
	"time"

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
