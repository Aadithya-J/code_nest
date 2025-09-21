package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Aadithya-J/code_nest/tests/common"
	"github.com/stretchr/testify/suite"
)

type BasicAuthTestSuite struct {
	common.BaseE2ETestSuite
	authHelpers *common.AuthHelpers
}

func (s *BasicAuthTestSuite) SetupSuite() {
	s.BaseE2ETestSuite.SetupSuite()
	s.authHelpers = common.NewAuthHelpers(&s.BaseE2ETestSuite)
}

func (s *BasicAuthTestSuite) TestSignupEndpoint() {
	email, _, err := s.authHelpers.SignupUniqueUser("signup_test")
	s.NoError(err)
	s.Contains(email, "signup_test")
}

func (s *BasicAuthTestSuite) TestLoginEndpoint() {
	// First create a user
	email, _, err := s.authHelpers.SignupUniqueUser("login_test")
	s.NoError(err)

	// Then login with the same credentials
	token, err := s.authHelpers.LoginUser(email, common.TestPassword)
	s.NoError(err)
	s.NotEmpty(token, "Should receive JWT token on login")
}

func (s *BasicAuthTestSuite) TestLoginFailure() {
	loginData := map[string]string{
		"email":    "nonexistent@example.com",
		"password": "wrongpassword",
	}
	loginBody, _ := json.Marshal(loginData)

	resp, err := s.Client.Post(common.APIGatewayURL+"/auth/login", "application/json", bytes.NewBuffer(loginBody))
	s.NoError(err)
	defer resp.Body.Close()

	s.Equal(http.StatusUnauthorized, resp.StatusCode, "Should reject invalid credentials")
}

func (s *BasicAuthTestSuite) TestInvalidToken() {
	// Test 1: Missing Authorization header
	req1, err := http.NewRequest("GET", common.APIGatewayURL+"/workspace/projects", nil)
	s.NoError(err)

	resp1, err := s.Client.Do(req1)
	s.NoError(err)
	defer resp1.Body.Close()

	s.Equal(http.StatusUnauthorized, resp1.StatusCode, "Should reject request without Authorization header")

	// Test 2: Invalid token format (missing "Bearer ")
	req2, err := http.NewRequest("GET", common.APIGatewayURL+"/workspace/projects", nil)
	s.NoError(err)
	req2.Header.Set("Authorization", "invalid-token-format")

	resp2, err := s.Client.Do(req2)
	s.NoError(err)
	defer resp2.Body.Close()

	s.Equal(http.StatusUnauthorized, resp2.StatusCode, "Should reject request with invalid token format")

	// Test 3: Invalid JWT token
	req3, err := http.NewRequest("GET", common.APIGatewayURL+"/workspace/projects", nil)
	s.NoError(err)
	req3.Header.Set("Authorization", "Bearer invalid.jwt.token")

	resp3, err := s.Client.Do(req3)
	s.NoError(err)
	defer resp3.Body.Close()

	s.Equal(http.StatusUnauthorized, resp3.StatusCode, "Should reject request with invalid JWT token")
}

func (s *BasicAuthTestSuite) TestProtectedRoute() {
	// Create user and get token
	_, token, err := s.authHelpers.SignupUniqueUser("protected_test")
	s.NoError(err)

	// Test protected route with valid token
	req, err := s.authHelpers.AuthenticatedRequest("GET", common.APIGatewayURL+"/protected", token, nil)
	s.NoError(err)

	resp, err := s.Client.Do(req)
	s.NoError(err)
	defer resp.Body.Close()

	s.Equal(http.StatusOK, resp.StatusCode, "Should allow access with valid token")
}

func TestBasicAuthSuite(t *testing.T) {
	suite.Run(t, new(BasicAuthTestSuite))
}
