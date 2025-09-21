package auth

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Aadithya-J/code_nest/tests/common"
	"github.com/stretchr/testify/suite"
)

type OAuthTestSuite struct {
	common.BaseE2ETestSuite
	authHelpers *common.AuthHelpers
}

func (s *OAuthTestSuite) SetupSuite() {
	s.BaseE2ETestSuite.SetupSuite()
	s.authHelpers = common.NewAuthHelpers(&s.BaseE2ETestSuite)
}

func (s *OAuthTestSuite) TestGoogleOAuthFlow() {
	// Test Google login endpoint should redirect to Google
	client := s.NoRedirectClient()

	resp, err := client.Get(common.APIGatewayURL + "/auth/google/login")
	s.NoError(err)
	defer resp.Body.Close()

	// Should get a redirect response (307 or 302)
	s.True(resp.StatusCode == http.StatusTemporaryRedirect || resp.StatusCode == 307,
		"Should redirect to Google, got status: %d", resp.StatusCode)

	// Check that the Location header contains Google OAuth URL
	location := resp.Header.Get("Location")
	s.Contains(location, "accounts.google.com/o/oauth2/auth", "Should redirect to Google OAuth")
	s.Contains(location, "client_id=", "Should include client_id parameter")
	s.Contains(location, "redirect_uri=", "Should include redirect_uri parameter")
	s.Contains(location, "state=", "Should include state parameter for CSRF protection")
	s.Contains(location, "scope=openid+email+profile", "Should request proper scopes")
}

func (s *OAuthTestSuite) TestGoogleCallbackValidation() {
	// Test 1: Missing code should return 400
	resp1, err := s.Client.Get(common.APIGatewayURL + "/auth/google/callback")
	s.NoError(err)
	defer resp1.Body.Close()

	s.Equal(http.StatusBadRequest, resp1.StatusCode, "Should reject callback without code")

	// Test 2: With code but invalid (will fail on Google token exchange - expected)
	resp2, err := s.Client.Get(common.APIGatewayURL + "/auth/google/callback?code=invalid_code&state=test_state")
	s.NoError(err)
	defer resp2.Body.Close()

	// This will return 500 because Google will reject the invalid code
	s.Equal(http.StatusInternalServerError, resp2.StatusCode, "Should fail with invalid Google code")
}

func (s *OAuthTestSuite) TestGitHubAuthFlow() {
	// Test GitHub login endpoint should redirect to GitHub
	client := s.NoRedirectClient()

	resp, err := client.Get(common.APIGatewayURL + "/auth/github/login")
	s.NoError(err)
	defer resp.Body.Close()

	// Should get a redirect response (307 or 302)
	s.True(resp.StatusCode == http.StatusTemporaryRedirect || resp.StatusCode == 307,
		"Should redirect to GitHub, got status: %d", resp.StatusCode)

	// Check that the Location header contains GitHub URL
	location := resp.Header.Get("Location")
	s.Contains(location, "github.com/apps/", "Should redirect to GitHub App installation")
	s.Contains(location, "installations/new", "Should be installation URL")
	s.Contains(location, "redirect_url=", "Should include callback URL")
}

func (s *OAuthTestSuite) TestGitHubCallbackValidation() {
	// Test 1: Without authentication should return 401
	resp1, err := s.Client.Get(common.APIGatewayURL + "/auth/github/callback")
	s.NoError(err)
	defer resp1.Body.Close()

	s.Equal(http.StatusUnauthorized, resp1.StatusCode, "Should require authentication")

	// Test 2: With authentication but missing installation_id should return 400
	_, token, err := s.authHelpers.SignupUniqueUser("github_callback_test")
	s.NoError(err)

	// Test with missing installation_id
	req2, err := s.authHelpers.AuthenticatedRequest("GET", common.APIGatewayURL+"/auth/github/callback", token, nil)
	s.NoError(err)

	resp2, err := s.Client.Do(req2)
	s.NoError(err)
	defer resp2.Body.Close()

	s.Equal(http.StatusBadRequest, resp2.StatusCode, "Should reject callback without installation_id")

	// Test 3: With authentication and valid installation_id
	req3, err := s.authHelpers.AuthenticatedRequest("GET", common.APIGatewayURL+"/auth/github/callback?installation_id=123456&setup_action=install", token, nil)
	s.NoError(err)

	resp3, err := s.Client.Do(req3)
	s.NoError(err)
	defer resp3.Body.Close()

	// This should now work since we have a real authenticated user
	s.Equal(http.StatusOK, resp3.StatusCode, "Should process authenticated callback")

	var response map[string]interface{}
	json.NewDecoder(resp3.Body).Decode(&response)
	// Should either succeed or fail with a GitHub API error (since we don't have real installation)
	s.True(response["token"] != nil || response["error"] != nil, "Should return either token or error")
}

func (s *OAuthTestSuite) TestGitHubStatusEndpoint() {
	// Test 1: Without authentication should return 401
	resp1, err := s.Client.Get(common.APIGatewayURL + "/auth/github/status")
	s.NoError(err)
	defer resp1.Body.Close()

	s.Equal(http.StatusUnauthorized, resp1.StatusCode, "Should require authentication")

	// Test 2: With valid JWT should return status (safe, no tokens exposed)
	_, token, err := s.authHelpers.SignupUniqueUser("github_status_test")
	s.NoError(err)

	// Check GitHub status (safe endpoint)
	req, err := s.authHelpers.AuthenticatedRequest("GET", common.APIGatewayURL+"/auth/github/status", token, nil)
	s.NoError(err)

	resp2, err := s.Client.Do(req)
	s.NoError(err)
	defer resp2.Body.Close()

	s.Equal(http.StatusOK, resp2.StatusCode, "Should return status safely")

	var statusResponse map[string]interface{}
	json.NewDecoder(resp2.Body).Decode(&statusResponse)
	s.Contains(statusResponse["user_id"], "github_status_test", "Should return user ID")
	s.Equal(false, statusResponse["github_linked"], "Should show GitHub not linked")
	s.Contains(statusResponse["message"], "not linked", "Should indicate GitHub not linked")
	s.Contains(statusResponse["message"], "dev only", "Should indicate dev-only endpoint")
}

func TestOAuthSuite(t *testing.T) {
	suite.Run(t, new(OAuthTestSuite))
}
