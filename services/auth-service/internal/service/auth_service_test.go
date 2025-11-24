package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/Aadithya-J/code_nest/proto"
	"github.com/Aadithya-J/code_nest/services/auth-service/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockUserRepository
type MockUserRepository struct {
	mock.Mock
}

func (m *MockUserRepository) Create(user *repository.User) error {
	args := m.Called(user)
	return args.Error(0)
}

func (m *MockUserRepository) FindByEmail(email string) (*repository.User, error) {
	args := m.Called(email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*repository.User), args.Error(1)
}

func (m *MockUserRepository) UpdateGitHubInfo(userID uint, installationID int64, username string) error {
	args := m.Called(userID, installationID, username)
	return args.Error(0)
}

// MockHTTPClient
type MockHTTPClient struct {
	mock.Mock
}

func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	args := m.Called(req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*http.Response), args.Error(1)
}

func (m *MockHTTPClient) Get(url string) (*http.Response, error) {
	args := m.Called(url)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*http.Response), args.Error(1)
}

func generatePEM() []byte {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	keyBytes := x509.MarshalPKCS1PrivateKey(key)
	pemBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: keyBytes,
	}
	return pem.EncodeToMemory(pemBlock)
}

func TestGetGitHubAccessToken(t *testing.T) {
	mockRepo := new(MockUserRepository)
	mockHTTP := new(MockHTTPClient)
	
	githubPEM := generatePEM()
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)

	authService := &AuthService{
		repo:             mockRepo,
		httpClient:       mockHTTP,
		githubAppID:      12345,
		githubPrivateKey: githubPEM,
		privateKey:       privateKey,
	}

	t.Run("Success", func(t *testing.T) {
		user := &repository.User{
			Email:                "test@example.com",
			GitHubInstallationID: 98765,
		}
		mockRepo.On("FindByEmail", "test@example.com").Return(user, nil).Once()

		// Mock GitHub API response
		respBody := map[string]interface{}{
			"token":      "ghs_testtoken",
			"expires_at": time.Now().Add(time.Hour).Format(time.RFC3339),
		}
		jsonBody, _ := json.Marshal(respBody)
		
		mockHTTP.On("Do", mock.MatchedBy(func(req *http.Request) bool {
			return req.URL.String() == "https://api.github.com/app/installations/98765/access_tokens" &&
				req.Method == "POST"
		})).Return(&http.Response{
			StatusCode: http.StatusCreated,
			Body:       io.NopCloser(bytes.NewReader(jsonBody)),
		}, nil).Once()

		resp, err := authService.GetGitHubAccessToken(context.Background(), &proto.GetGitHubAccessTokenRequest{
			UserId: "test@example.com",
		})

		assert.NoError(t, err)
		assert.Equal(t, "ghs_testtoken", resp.Token)
		assert.NotZero(t, resp.ExpiresAt)
	})

	t.Run("UserNotFound", func(t *testing.T) {
		mockRepo.On("FindByEmail", "unknown@example.com").Return(nil, errors.New("not found")).Once()

		resp, err := authService.GetGitHubAccessToken(context.Background(), &proto.GetGitHubAccessTokenRequest{
			UserId: "unknown@example.com",
		})

		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "user not found")
	})

	t.Run("UserNotLinked", func(t *testing.T) {
		user := &repository.User{
			Email:                "unlinked@example.com",
			GitHubInstallationID: 0,
		}
		mockRepo.On("FindByEmail", "unlinked@example.com").Return(user, nil).Once()

		resp, err := authService.GetGitHubAccessToken(context.Background(), &proto.GetGitHubAccessTokenRequest{
			UserId: "unlinked@example.com",
		})

		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "user has not linked GitHub")
	})

	t.Run("GitHubAPIError", func(t *testing.T) {
		user := &repository.User{
			Email:                "error@example.com",
			GitHubInstallationID: 98765,
		}
		mockRepo.On("FindByEmail", "error@example.com").Return(user, nil).Once()

		mockHTTP.On("Do", mock.Anything).Return(&http.Response{
			StatusCode: http.StatusUnauthorized,
			Body:       io.NopCloser(bytes.NewReader([]byte("Unauthorized"))),
		}, nil).Once()

		resp, err := authService.GetGitHubAccessToken(context.Background(), &proto.GetGitHubAccessTokenRequest{
			UserId: "error@example.com",
		})

		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "GitHub API error")
	})
}
