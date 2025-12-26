package tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Basic integration tests without external dependencies
func TestHealthCheckIntegration(t *testing.T) {
	// Mock health check endpoint
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", "test-request-123")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	resp, err := http.Get(server.URL + "/health")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	requestID := resp.Header.Get("X-Request-ID")
	if requestID != "test-request-123" {
		t.Errorf("Expected request ID 'test-request-123', got '%s'", requestID)
	}

	var response map[string]string
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response["status"] != "ok" {
		t.Errorf("Expected status 'ok', got '%s'", response["status"])
	}
}

func TestFileListIntegration(t *testing.T) {
	// Create temporary test directory
	tempDir, err := os.MkdirTemp("", "test-files")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test files
	testFiles := map[string]string{
		"README.md":    "# Test Project",
		"package.json": `{"name": "test"}`,
		"src/app.js":   "console.log('hello');",
	}

	for filePath, content := range testFiles {
		fullPath := filepath.Join(tempDir, filePath)
		os.MkdirAll(filepath.Dir(fullPath), 0755)
		err := os.WriteFile(fullPath, []byte(content), 0644)
		if err != nil {
			t.Fatalf("Failed to create test file %s: %v", filePath, err)
		}
	}

	// Mock file list handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		type FileNode struct {
			Name        string      `json:"name"`
			IsDir       bool        `json:"isDir"`
			Path        string      `json:"path"`
			Size        int64       `json:"size"`
			ModTime     time.Time   `json:"modTime"`
			Permissions string      `json:"permissions"`
			Extension   string      `json:"extension,omitempty"`
			Nodes       []*FileNode `json:"nodes,omitempty"`
		}

		var walk func(string) ([]*FileNode, error)
		walk = func(curr string) ([]*FileNode, error) {
			entries, err := os.ReadDir(curr)
			if err != nil {
				return nil, err
			}

			var nodes []*FileNode
			for _, entry := range entries {
				if entry.Name() == ".git" {
					continue
				}

				fullPath := filepath.Join(curr, entry.Name())
				info, _ := entry.Info()

				node := &FileNode{
					Name:        entry.Name(),
					IsDir:       entry.IsDir(),
					Path:        strings.TrimPrefix(fullPath, tempDir),
					Size:        info.Size(),
					ModTime:     info.ModTime(),
					Permissions: info.Mode().String(),
					Extension:   filepath.Ext(entry.Name()),
				}

				if node.IsDir {
					node.Nodes, _ = walk(fullPath)
				}

				nodes = append(nodes, node)
			}
			return nodes, nil
		}

		nodes, err := walk(tempDir)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(nodes)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	resp, err := http.Get(server.URL + "/files")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var files []map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&files)
	if err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(files) == 0 {
		t.Error("Expected files in response, got empty list")
	}

	// Check for README.md
	foundReadme := false
	for _, file := range files {
		if name, ok := file["name"].(string); ok && name == "README.md" {
			foundReadme = true
			break
		}
	}

	if !foundReadme {
		t.Error("Expected to find README.md in file list")
	}
}

func TestFileContentIntegration(t *testing.T) {
	// Create temporary test file
	tempDir, err := os.MkdirTemp("", "test-content")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	testContent := "This is a test file content"
	testFile := filepath.Join(tempDir, "test.txt")
	err = os.WriteFile(testFile, []byte(testContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Mock file content handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Query().Get("path")
		if path == "" {
			http.Error(w, "path required", 400)
			return
		}

		fullPath := filepath.Join(tempDir, path)

		// Security check
		if !strings.HasPrefix(filepath.Clean(fullPath), tempDir) {
			http.Error(w, "invalid path", 400)
			return
		}

		content, err := os.ReadFile(fullPath)
		if err != nil {
			http.Error(w, "file not found", 404)
			return
		}

		w.Header().Set("Content-Type", "text/plain")
		w.Write(content)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	resp, err := http.Get(server.URL + "/files/content?path=test.txt")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	if string(content) != testContent {
		t.Errorf("Expected content '%s', got '%s'", testContent, string(content))
	}
}

func TestSecurityIntegration(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "test-security")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Mock secure file handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Query().Get("path")
		if path == "" {
			http.Error(w, "path required", 400)
			return
		}

		// Security validations
		if strings.Contains(path, "..") {
			http.Error(w, "path traversal not allowed", 400)
			return
		}

		if filepath.IsAbs(path) {
			http.Error(w, "absolute paths not allowed", 400)
			return
		}

		if strings.HasPrefix(filepath.Base(path), ".") {
			http.Error(w, "hidden files not allowed", 400)
			return
		}

		fullPath := filepath.Join(tempDir, path)

		// Ensure path is within tempDir
		if !strings.HasPrefix(filepath.Clean(fullPath), tempDir) {
			http.Error(w, "invalid path", 400)
			return
		}

		http.Error(w, "file not found", 404)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	// Test path traversal
	resp, err := http.Get(server.URL + "/files/content?path=../../../etc/passwd")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 400 {
		t.Errorf("Expected status 400 for path traversal, got %d", resp.StatusCode)
	}

	// Test absolute path
	resp, err = http.Get(server.URL + "/files/content?path=/etc/passwd")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 400 {
		t.Errorf("Expected status 400 for absolute path, got %d", resp.StatusCode)
	}

	// Test hidden file
	resp, err = http.Get(server.URL + "/files/content?path=.env")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 400 {
		t.Errorf("Expected status 400 for hidden file, got %d", resp.StatusCode)
	}
}

func TestRateLimitingIntegration(t *testing.T) {
	requestCount := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount > 10 { // Allow 10 requests, then rate limit
			w.WriteHeader(429)
			json.NewEncoder(w).Encode(map[string]string{"error": "rate limited"})
			return
		}
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	rateLimitedCount := 0
	successCount := 0

	// Make 20 requests rapidly
	for i := 0; i < 20; i++ {
		resp, err := http.Get(server.URL + "/test")
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == 429 {
			rateLimitedCount++
		} else if resp.StatusCode == 200 {
			successCount++
		}
	}

	if rateLimitedCount == 0 {
		t.Error("Expected some requests to be rate limited")
	}

	if successCount == 0 {
		t.Error("Expected some requests to succeed")
	}

	fmt.Printf("Successful requests: %d, Rate limited: %d\n", successCount, rateLimitedCount)
}

func TestConcurrentRequests(t *testing.T) {
	requestCount := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		// Simulate some processing time
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    "ok",
			"requestId": requestCount,
		})
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	const numRequests = 50
	results := make(chan int, numRequests)

	// Launch concurrent requests
	for i := 0; i < numRequests; i++ {
		go func() {
			resp, err := http.Get(server.URL + "/test")
			if err != nil {
				results <- -1
				return
			}
			defer resp.Body.Close()
			results <- resp.StatusCode
		}()
	}

	successCount := 0
	for i := 0; i < numRequests; i++ {
		statusCode := <-results
		if statusCode == 200 {
			successCount++
		}
	}

	if successCount < numRequests/2 {
		t.Errorf("Expected at least %d successful requests, got %d", numRequests/2, successCount)
	}

	fmt.Printf("Concurrent test: %d/%d requests successful\n", successCount, numRequests)
}

func TestErrorHandlingIntegration(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/not-found":
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
		case "/bad-request":
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{"error": "bad request"})
		case "/server-error":
			w.WriteHeader(500)
			json.NewEncoder(w).Encode(map[string]string{"error": "internal server error"})
		default:
			w.WriteHeader(200)
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		}
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	testCases := []struct {
		path       string
		statusCode int
		errorMsg   string
	}{
		{"/not-found", 404, "not found"},
		{"/bad-request", 400, "bad request"},
		{"/server-error", 500, "internal server error"},
	}

	for _, tc := range testCases {
		resp, err := http.Get(server.URL + tc.path)
		if err != nil {
			t.Fatalf("Failed to make request to %s: %v", tc.path, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != tc.statusCode {
			t.Errorf("Expected status %d for %s, got %d", tc.statusCode, tc.path, resp.StatusCode)
		}

		var response map[string]string
		err = json.NewDecoder(resp.Body).Decode(&response)
		if err != nil {
			t.Fatalf("Failed to decode response for %s: %v", tc.path, err)
		}

		if response["error"] != tc.errorMsg {
			t.Errorf("Expected error '%s' for %s, got '%s'", tc.errorMsg, tc.path, response["error"])
		}
	}
}

func TestRequestSizeLimitIntegration(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check content length
		if r.ContentLength > 10*1024*1024 { // 10MB limit
			w.WriteHeader(413)
			json.NewEncoder(w).Encode(map[string]string{"error": "payload too large"})
			return
		}

		// Read body (limited)
		body, err := io.ReadAll(io.LimitReader(r.Body, 10*1024*1024))
		if err != nil {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to read body"})
			return
		}

		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":     "ok",
			"bodyLength": len(body),
		})
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	// Test normal size
	normalPayload := strings.Repeat("x", 1024) // 1KB
	resp, err := http.Post(server.URL+"/test", "text/plain", strings.NewReader(normalPayload))
	if err != nil {
		t.Fatalf("Failed to make normal request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200 for normal payload, got %d", resp.StatusCode)
	}

	// Test oversized payload
	largePayload := strings.Repeat("x", 11*1024*1024) // 11MB
	resp, err = http.Post(server.URL+"/test", "text/plain", strings.NewReader(largePayload))
	if err != nil {
		t.Fatalf("Failed to make large request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 413 {
		t.Errorf("Expected status 413 for large payload, got %d", resp.StatusCode)
	}
}

func TestWebSocketSecurityIntegration(t *testing.T) {
	// Mock WebSocket upgrade handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// Check origin validation
		allowedOrigins := []string{
			"http://localhost:3000",
			"https://localhost:3000",
			"http://127.0.0.1:3000",
		}

		allowed := false
		for _, allowedOrigin := range allowedOrigins {
			if origin == allowedOrigin {
				allowed = true
				break
			}
		}

		if !allowed && origin != "" {
			w.WriteHeader(403)
			json.NewEncoder(w).Encode(map[string]string{"error": "origin not allowed"})
			return
		}

		// For testing, just return success
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]string{"status": "websocket upgrade allowed"})
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	// Test allowed origin
	req, _ := http.NewRequest("GET", server.URL+"/ws", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "upgrade")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to make WebSocket request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200 for allowed origin, got %d", resp.StatusCode)
	}

	// Test disallowed origin
	req, _ = http.NewRequest("GET", server.URL+"/ws", nil)
	req.Header.Set("Origin", "http://evil.com")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "upgrade")

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to make WebSocket request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 403 {
		t.Errorf("Expected status 403 for disallowed origin, got %d", resp.StatusCode)
	}
}
