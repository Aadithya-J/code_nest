package helpers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

const (
	APIGatewayBaseURL = "http://localhost:8080"
	DefaultTimeout    = 30 * time.Second
)

// HTTPClient wraps HTTP operations for testing
type HTTPClient struct {
	BaseURL    string
	HTTPClient *http.Client
	AuthToken  string
}

// NewHTTPClient creates a new HTTP test client
func NewHTTPClient() *HTTPClient {
	return &HTTPClient{
		BaseURL: APIGatewayBaseURL,
		HTTPClient: &http.Client{
			Timeout: DefaultTimeout,
		},
	}
}

// SetAuthToken sets the JWT token for authenticated requests
func (c *HTTPClient) SetAuthToken(token string) {
	c.AuthToken = token
}

// POST makes a POST request
func (c *HTTPClient) POST(t *testing.T, path string, body interface{}) (*http.Response, []byte) {
	t.Helper()

	jsonBody, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Failed to marshal request body: %v", err)
	}

	req, err := http.NewRequest("POST", c.BaseURL+path, bytes.NewBuffer(jsonBody))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.AuthToken)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	respBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	return resp, respBody
}

// GET makes a GET request
func (c *HTTPClient) GET(t *testing.T, path string) (*http.Response, []byte) {
	t.Helper()

	req, err := http.NewRequest("GET", c.BaseURL+path, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	if c.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.AuthToken)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	respBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	return resp, respBody
}

// PUT makes a PUT request
func (c *HTTPClient) PUT(t *testing.T, path string, body interface{}) (*http.Response, []byte) {
	t.Helper()

	jsonBody, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Failed to marshal request body: %v", err)
	}

	req, err := http.NewRequest("PUT", c.BaseURL+path, bytes.NewBuffer(jsonBody))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.AuthToken)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	respBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	return resp, respBody
}

// DELETE makes a DELETE request
func (c *HTTPClient) DELETE(t *testing.T, path string) (*http.Response, []byte) {
	t.Helper()

	req, err := http.NewRequest("DELETE", c.BaseURL+path, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	if c.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.AuthToken)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	respBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	return resp, respBody
}

// ParseJSON unmarshals JSON response
func (c *HTTPClient) ParseJSON(t *testing.T, data []byte, v interface{}) {
	t.Helper()

	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("Failed to parse JSON: %v\nBody: %s", err, string(data))
	}
}

// WaitForService waits for a service to be available
func WaitForService(t *testing.T, url string, maxWait time.Duration) error {
	t.Helper()

	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(maxWait)

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode < 500 {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("service at %s not available after %v", url, maxWait)
}

// WaitForSessionReady polls the workspace session status until it's RUNNING or times out
// This is more reliable than using fixed sleep times
func (c *HTTPClient) WaitForSessionReady(t *testing.T, projectID string, maxWait time.Duration) error {
	t.Helper()

	deadline := time.Now().Add(maxWait)
	pollInterval := 2 * time.Second

	t.Logf("Waiting for workspace session to be ready (max %v)...", maxWait)

	var lastBody []byte

	for time.Now().Before(deadline) {
		// Try to perform a simple file operation to check if workspace is ready
		// We'll try to get the file tree, which requires an active session
		url := fmt.Sprintf("/workspace/projects/%s/files/tree", projectID)
		resp, body := c.GET(t, url)
		lastBody = body

		if resp.StatusCode == 200 {
			t.Logf("✅ Workspace session is ready!")
			return nil
		} else {
			// Only log every 10th attempt or if it's a 404 to avoid spam, but for now log all 404s
			if resp.StatusCode == 404 {
				t.Logf("⚠️ WaitForSessionReady: %s returned %d. Body: %s", url, resp.StatusCode, string(body))
			}
		}

		// Parse error to see if it's a "no active session" error
		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err == nil {
			if errorMsg, ok := result["error"].(string); ok {
				t.Logf("Waiting... (status: %s). Response Body: %s", errorMsg, string(body))
			} else {
				t.Logf("Waiting... (unknown response format). Body: %s", string(body))
			}
		} else {
			t.Logf("Waiting... (json parse error). Body: %s", string(body))
		}

		time.Sleep(pollInterval)
	}

	return fmt.Errorf("workspace session not ready after %v. Last status: %s", maxWait, string(lastBody))
}
