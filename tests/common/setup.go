package common

import (
	"log"
	"net/http"
	"time"

	"github.com/stretchr/testify/suite"
)

const (
	APIGatewayURL = "http://localhost:8080"
	TestEmail     = "testuser@example.com"
	TestPassword  = "password123"
)

// BaseE2ETestSuite provides common setup for all E2E tests
type BaseE2ETestSuite struct {
	suite.Suite
	Client *http.Client
}

// SetupSuite runs once before all tests in the suite
func (s *BaseE2ETestSuite) SetupSuite() {
	s.Client = &http.Client{Timeout: 10 * time.Second}

	// Wait for the API Gateway to be ready before running tests
	maxWait := 15 * time.Second
	checkInterval := 1 * time.Second
	startTime := time.Now()

	for {
		// Health check using a known endpoint
		resp, err := http.Get(APIGatewayURL + "/auth/google/login")
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

// NoRedirectClient returns an HTTP client that doesn't follow redirects
func (s *BaseE2ETestSuite) NoRedirectClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
