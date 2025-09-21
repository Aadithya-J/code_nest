package common

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Aadithya-J/code_nest/proto"
)

// AuthHelpers provides common authentication utilities for tests
type AuthHelpers struct {
	suite *BaseE2ETestSuite
}

func NewAuthHelpers(suite *BaseE2ETestSuite) *AuthHelpers {
	return &AuthHelpers{suite: suite}
}

// SignupUser creates a new user and returns the JWT token
func (h *AuthHelpers) SignupUser(email, password string) (string, error) {
	signupData := map[string]string{
		"email":    email,
		"password": password,
	}
	signupBody, _ := json.Marshal(signupData)

	resp, err := h.suite.Client.Post(APIGatewayURL+"/auth/signup", "application/json", bytes.NewBuffer(signupBody))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("signup failed with status %d", resp.StatusCode)
	}

	var signupResponse proto.AuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&signupResponse); err != nil {
		return "", err
	}

	return signupResponse.Token, nil
}

// SignupUniqueUser creates a user with a unique email based on timestamp
func (h *AuthHelpers) SignupUniqueUser(prefix string) (string, string, error) {
	email := fmt.Sprintf("%s_%d@example.com", prefix, time.Now().UnixNano())
	token, err := h.SignupUser(email, TestPassword)
	return email, token, err
}

// LoginUser logs in an existing user and returns the JWT token
func (h *AuthHelpers) LoginUser(email, password string) (string, error) {
	loginData := map[string]string{
		"email":    email,
		"password": password,
	}
	loginBody, _ := json.Marshal(loginData)

	resp, err := h.suite.Client.Post(APIGatewayURL+"/auth/login", "application/json", bytes.NewBuffer(loginBody))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("login failed with status %d", resp.StatusCode)
	}

	var loginResponse proto.AuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&loginResponse); err != nil {
		return "", err
	}

	return loginResponse.Token, nil
}

// AuthenticatedRequest creates an HTTP request with JWT authorization header
func (h *AuthHelpers) AuthenticatedRequest(method, url, token string, body []byte) (*http.Request, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, err
	}
	
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	
	return req, nil
}
