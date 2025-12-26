//go:build integration
// +build integration

package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/suite"
)

// AgentIntegrationTestSuite tests agent functionality
type AgentIntegrationTestSuite struct {
	suite.Suite
	agentServer   *httptest.Server
	testWorkspace string
	cleanup       []func()
}

func TestAgentIntegrationSuite(t *testing.T) {
	suite.Run(t, new(AgentIntegrationTestSuite))
}

func (suite *AgentIntegrationTestSuite) SetupSuite() {
	// Create temporary workspace for testing
	tempDir, err := os.MkdirTemp("", "agent-test-workspace")
	suite.Require().NoError(err)
	suite.testWorkspace = tempDir

	// Create test files
	suite.createTestFiles()

	// Setup agent server
	suite.setupAgentServer()
}

func (suite *AgentIntegrationTestSuite) TearDownSuite() {
	// Cleanup
	for _, cleanup := range suite.cleanup {
		cleanup()
	}

	// Remove test workspace
	os.RemoveAll(suite.testWorkspace)
}

func (suite *AgentIntegrationTestSuite) createTestFiles() {
	// Create test directory structure
	dirs := []string{
		"src",
		"src/components",
		"src/utils",
		"docs",
		"tests",
	}

	for _, dir := range dirs {
		err := os.MkdirAll(filepath.Join(suite.testWorkspace, dir), 0755)
		suite.Require().NoError(err)
	}

	// Create test files
	files := map[string]string{
		"README.md":             "# Test Project\n\nThis is a test project.",
		"package.json":          `{"name": "test-project", "version": "1.0.0"}`,
		"src/main.js":           "console.log('Hello World');",
		"src/components/App.js": "export default function App() { return <div>Hello</div>; }",
		"src/utils/helper.js":   "export function helper() { return true; }",
		"docs/api.md":           "# API Documentation",
		"tests/unit.test.js":    "test('unit test', () => { expect(true).toBe(true); });",
	}

	for filePath, content := range files {
		fullPath := filepath.Join(suite.testWorkspace, filePath)
		err := os.WriteFile(fullPath, []byte(content), 0644)
		suite.Require().NoError(err)
	}
}

func (suite *AgentIntegrationTestSuite) setupAgentServer() {
	// In a real implementation, this would start the actual agent
	// For testing, we'll mock the agent endpoints

	mux := http.NewServeMux()

	// Mock agent endpoints
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
	})

	mux.HandleFunc("/files", func(w http.ResponseWriter, r *http.Request) {
		suite.mockFileList(w, r, suite.testWorkspace)
	})

	mux.HandleFunc("/files/content", func(w http.ResponseWriter, r *http.Request) {
		suite.mockFileContent(w, r, suite.testWorkspace)
	})

	mux.HandleFunc("/terminal", func(w http.ResponseWriter, r *http.Request) {
		suite.mockTerminal(w, r)
	})

	suite.agentServer = httptest.NewServer(mux)

	suite.cleanup = append(suite.cleanup, func() {
		suite.agentServer.Close()
	})
}

func (suite *AgentIntegrationTestSuite) mockFileList(w http.ResponseWriter, r *http.Request, workspace string) {
	type FileNode struct {
		Name        string      `json:"name"`
		IsDir       bool        `json:"isDir"`
		Path        string      `json:"path"`
		Size        int64       `json:"size"`
		ModTime     time.Time   `json:"modTime"`
		Permissions string      `json:"permissions"`
		Extension   string      `json:"extension,omitempty"`
		MimeType    string      `json:"mimeType,omitempty"`
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
				Path:        strings.TrimPrefix(fullPath, workspace),
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

	nodes, err := walk(workspace)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(nodes)
}

func (suite *AgentIntegrationTestSuite) mockFileContent(w http.ResponseWriter, r *http.Request, workspace string) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path required", 400)
		return
	}

	fullPath := filepath.Join(workspace, path)

	// Security check
	if !strings.HasPrefix(filepath.Clean(fullPath), workspace) {
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
}

func (suite *AgentIntegrationTestSuite) mockTerminal(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins in tests
		},
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// Mock terminal behavior
	for {
		messageType, p, err := conn.ReadMessage()
		if err != nil {
			break
		}

		// Echo back the message for testing
		err = conn.WriteMessage(messageType, p)
		if err != nil {
			break
		}
	}
}

// Test 1: Agent Health Check
func (suite *AgentIntegrationTestSuite) TestAgentHealthCheck() {
	resp, err := http.Get(suite.agentServer.URL + "/health")
	suite.Require().NoError(err)
	defer resp.Body.Close()

	suite.Equal(200, resp.StatusCode)

	var response map[string]string
	err = json.NewDecoder(resp.Body).Decode(&response)
	suite.Require().NoError(err)

	suite.Equal("ready", response["status"])
}

// Test 2: File List Integration
func (suite *AgentIntegrationTestSuite) TestFileListIntegration() {
	resp, err := http.Get(suite.agentServer.URL + "/files")
	suite.Require().NoError(err)
	defer resp.Body.Close()

	suite.Equal(200, resp.StatusCode)

	var files []map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&files)
	suite.Require().NoError(err)

	suite.Greater(len(files), 0)

	// Check for expected files
	var foundReadme, foundPackageJson bool
	for _, file := range files {
		name, ok := file["name"].(string)
		if !ok {
			continue
		}

		if name == "README.md" {
			foundReadme = true
		}
		if name == "package.json" {
			foundPackageJson = true
		}
	}

	suite.True(foundReadme, "README.md should be found")
	suite.True(foundPackageJson, "package.json should be found")
}

// Test 3: File Content Integration
func (suite *AgentIntegrationTestSuite) TestFileContentIntegration() {
	resp, err := http.Get(suite.agentServer.URL + "/files/content?path=README.md")
	suite.Require().NoError(err)
	defer resp.Body.Close()

	suite.Equal(200, resp.StatusCode)

	content, err := io.ReadAll(resp.Body)
	suite.Require().NoError(err)

	suite.Contains(string(content), "# Test Project")
	suite.Contains(string(content), "This is a test project.")
}

// Test 4: File Metadata Integration
func (suite *AgentIntegrationTestSuite) TestFileMetadataIntegration() {
	resp, err := http.Get(suite.agentServer.URL + "/files")
	suite.Require().NoError(err)
	defer resp.Body.Close()

	suite.Equal(200, resp.StatusCode)

	var files []map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&files)
	suite.Require().NoError(err)

	// Find README.md and check metadata
	var readme map[string]interface{}
	for _, file := range files {
		if name, ok := file["name"].(string); ok && name == "README.md" {
			readme = file
			break
		}
	}

	suite.NotEmpty(readme)

	// Check metadata fields
	suite.Contains(readme, "name")
	suite.Contains(readme, "isDir")
	suite.Contains(readme, "path")
	suite.Contains(readme, "size")
	suite.Contains(readme, "modTime")
	suite.Contains(readme, "permissions")
	suite.Contains(readme, "extension")

	suite.Equal("README.md", readme["name"])
	suite.Equal(false, readme["isDir"])
	suite.Equal(".md", readme["extension"])
}

// Test 5: WebSocket Terminal Integration
func (suite *AgentIntegrationTestSuite) TestWebSocketTerminalIntegration() {
	// Convert http:// to ws://
	wsURL := "ws" + strings.TrimPrefix(suite.agentServer.URL, "http") + "/terminal"

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	suite.Require().NoError(err)
	defer conn.Close()

	// Test message sending
	testMessage := "echo test"
	err = conn.WriteMessage(websocket.TextMessage, []byte(testMessage))
	suite.Require().NoError(err)

	// Read response
	messageType, response, err := conn.ReadMessage()
	suite.Require().NoError(err)

	suite.Equal(websocket.TextMessage, messageType)
	suite.Equal(testMessage, string(response))
}

// Test 6: File Security Integration
func (suite *AgentIntegrationTestSuite) TestFileSecurityIntegration() {
	// Test path traversal attempt
	resp, err := http.Get(suite.agentServer.URL + "/files/content?path=../../../etc/passwd")
	suite.Require().NoError(err)
	defer resp.Body.Close()

	suite.Equal(400, resp.StatusCode)

	// Test hidden file access
	resp, err = http.Get(suite.agentServer.URL + "/files/content?path=.env")
	suite.Require().NoError(err)
	defer resp.Body.Close()

	suite.Equal(400, resp.StatusCode)
}

// Test 7: File Upload Integration
func (suite *AgentIntegrationTestSuite) TestFileUploadIntegration() {
	// Test file save functionality
	testContent := "This is a test file upload"

	resp, err := http.Post(
		suite.agentServer.URL+"/files/save?path=test-upload.txt",
		"text/plain",
		bytes.NewBufferString(testContent),
	)
	suite.Require().NoError(err)
	defer resp.Body.Close()

	suite.Equal(200, resp.StatusCode)

	// Verify file was created
	resp, err = http.Get(suite.agentServer.URL + "/files/content?path=test-upload.txt")
	suite.Require().NoError(err)
	defer resp.Body.Close()

	suite.Equal(200, resp.StatusCode)

	content, err := io.ReadAll(resp.Body)
	suite.Require().NoError(err)

	suite.Equal(testContent, string(content))
}

// Test 8: Concurrent File Operations
func (suite *AgentIntegrationTestSuite) TestConcurrentFileOperations() {
	const numRequests = 50

	results := make(chan int, numRequests)

	for i := 0; i < numRequests; i++ {
		go func(id int) {
			resp, err := http.Get(suite.agentServer.URL + "/files")
			if err != nil {
				results <- -1
				return
			}
			defer resp.Body.Close()
			results <- resp.StatusCode
		}(i)
	}

	successCount := 0
	for i := 0; i < numRequests; i++ {
		statusCode := <-results
		if statusCode == 200 {
			successCount++
		}
	}

	// Most requests should succeed
	suite.Greater(successCount, numRequests/2)
}

// Test 9: Large File Handling
func (suite *AgentIntegrationTestSuite) TestLargeFileHandling() {
	// Create a large test file
	largeContent := strings.Repeat("x", 1024*1024) // 1MB

	resp, err := http.Post(
		suite.agentServer.URL+"/files/save?path=large-file.txt",
		"text/plain",
		bytes.NewBufferString(largeContent),
	)
	suite.Require().NoError(err)
	defer resp.Body.Close()

	suite.Equal(200, resp.StatusCode)

	// Verify large file can be read
	resp, err = http.Get(suite.agentServer.URL + "/files/content?path=large-file.txt")
	suite.Require().NoError(err)
	defer resp.Body.Close()

	suite.Equal(200, resp.StatusCode)

	content, err := io.ReadAll(resp.Body)
	suite.Require().NoError(err)

	suite.Equal(len(largeContent), len(content))
}

// Test 10: Error Recovery
func (suite *AgentIntegrationTestSuite) TestErrorRecovery() {
	// Test invalid file path
	resp, err := http.Get(suite.agentServer.URL + "/files/content?path=/nonexistent/file.txt")
	suite.Require().NoError(err)
	defer resp.Body.Close()

	suite.Equal(404, resp.StatusCode)

	// Test malformed request
	resp, err = http.Post(
		suite.agentServer.URL+"/files/save",
		"application/json",
		bytes.NewBufferString("invalid"),
	)
	suite.Require().NoError(err)
	defer resp.Body.Close()

	// Should handle gracefully
	suite.True(resp.StatusCode >= 400 && resp.StatusCode < 500)
}
