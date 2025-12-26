package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

type AppConfig struct {
	CallbackURL   string
	CallbackToken string
	AtlasID       string
	RepoURL       string
	GitToken      string
	GitUser       string
	GitEmail      string
}

var (
	cfg      AppConfig
	ready    bool
	mu       sync.RWMutex
	cloneLog bytes.Buffer
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return false // No origin header, reject
			}

			// Allow localhost origins for development
			if strings.HasPrefix(origin, "http://localhost:") ||
				strings.HasPrefix(origin, "https://localhost:") ||
				strings.HasPrefix(origin, "http://127.0.0.1:") ||
				strings.HasPrefix(origin, "https://127.0.0.1:") {
				return true
			}

			// In production, check against allowed origins
			allowedOrigins := []string{
				"http://localhost:3000", // Gateway default
				"https://localhost:3000",
			}

			for _, allowed := range allowedOrigins {
				if origin == allowed {
					return true
				}
			}

			return false
		},
	}
	// Rate limiting: 100 requests per minute per IP
	rateLimiter = make(map[string]*rateLimitInfo)
	rateLimitMu sync.Mutex
	// Port watcher synchronization
	portWatcherMu sync.Mutex
	knownPorts    = map[int]bool{}
)

type rateLimitInfo struct {
	count     int64
	lastReset int64
}

// Request size limiting middleware
func limitBodySizeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Limit request body to 10MB
		r.Body = http.MaxBytesReader(w, r.Body, 10<<20) // 10MB
		next.ServeHTTP(w, r)
	})
}

// Request ID middleware for correlation tracking
func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := generateRequestID()
		w.Header().Set("X-Request-ID", requestID)
		r = r.WithContext(context.WithValue(r.Context(), "requestID", requestID))
		next.ServeHTTP(w, r)
	})
}

// Security headers middleware
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'")
		next.ServeHTTP(w, r)
	})
}

// Rate limiting middleware
func rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := getClientIP(r)
		if !checkRateLimit(clientIP) {
			logWithRequestID(r, "Rate limit exceeded for IP: %s", clientIP)
			http.Error(w, "Too many requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func generateRequestID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func getClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.Split(xff, ",")[0]
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	return strings.Split(r.RemoteAddr, ":")[0]
}

func checkRateLimit(clientIP string) bool {
	now := time.Now().Unix()
	rateLimitMu.Lock()
	defer rateLimitMu.Unlock()

	info, exists := rateLimiter[clientIP]
	if !exists || now-info.lastReset > 60 {
		rateLimiter[clientIP] = &rateLimitInfo{count: 1, lastReset: now}
		return true
	}

	if info.count >= 100 { // 100 requests per minute
		return false
	}

	atomic.AddInt64(&info.count, 1)
	return true
}

func logWithRequestID(r *http.Request, format string, args ...interface{}) {
	requestID, _ := r.Context().Value("requestID").(string)
	if requestID != "" {
		args = append([]interface{}{requestID}, args...)
		log.Printf("[req:%s] "+format, args...)
	} else {
		log.Printf(format, args...)
	}
}

func main() {
	cfg = loadConfig()
	log.Printf("Starting agent with config: AtlasID=%s, User=%s, Email=%s", cfg.AtlasID, cfg.GitUser, cfg.GitEmail)

	go backgroundClone()
	go portWatcher()

	// Apply middleware chain
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/terminal", terminalHandler)
	mux.HandleFunc("/files", fileListHandler)
	mux.HandleFunc("/files/content", fileContentHandler)
	mux.HandleFunc("/files/save", fileSaveHandler)
	mux.HandleFunc("/files/", fileHandler)

	handler := securityHeadersMiddleware(requestIDMiddleware(rateLimitMiddleware(limitBodySizeMiddleware(mux))))

	go autoCommitLoop()

	server := &http.Server{
		Addr:           ":9000",
		Handler:        handler,
		ReadTimeout:    15 * time.Second,
		WriteTimeout:   15 * time.Second,
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1MB max header size
	}
	go func() {
		log.Println("agent listening on :9000")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("agent server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Println("shutting down agent...")
	gracefulShutdown()
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	if ready {
		fmt.Fprint(w, "ready")
		return
	}
	fmt.Fprint(w, "cloning")
}

func loadConfig() AppConfig {
	return AppConfig{
		CallbackURL:   getenv("AGENT_CALLBACK_URL", ""),
		CallbackToken: getenv("AGENT_CALLBACK_TOKEN", ""),
		AtlasID:       getenv("ATLAS_ID", ""),
		RepoURL:       getenv("GIT_REPO", ""),
		GitToken:      getenv("GIT_TOKEN", ""),
		GitUser:       getenv("GIT_USER_NAME", "workspace"),
		GitEmail:      getenv("GIT_USER_EMAIL", "workspace@example.com"),
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func backgroundClone() {
	log.Printf("Starting background clone with user: %s, email: %s", cfg.GitUser, cfg.GitEmail)

	if err := exec.Command("git", "config", "--global", "user.name", cfg.GitUser).Run(); err != nil {
		log.Printf("Failed to set git user.name: %v", err)
	}
	if err := exec.Command("git", "config", "--global", "user.email", cfg.GitEmail).Run(); err != nil {
		log.Printf("Failed to set git user.email: %v", err)
	}

	cloneURL := cfg.RepoURL
	if strings.HasPrefix(cfg.RepoURL, "https://") && cfg.GitToken != "" {
		cloneURL = strings.Replace(cfg.RepoURL, "https://", fmt.Sprintf("https://x-access-token:%s@", cfg.GitToken), 1)
	}
	log.Printf("Cloning repository from: %s", maskToken(cloneURL))
	cmd := exec.Command("git", "clone", cloneURL, "/workspace")
	out, err := cmd.CombinedOutput()
	cloneLog.Write(out)
	if err != nil {
		log.Printf("Git clone failed: %v, output: %s", err, string(out))
		notifyCallback("ERROR")
		return
	}
	log.Printf("Repository cloned successfully")
	setReady()
	notifyCallback("READY")
}

func maskToken(url string) string {
	if strings.Contains(url, "x-access-token:") {
		return strings.Replace(url, "x-access-token:", "x-access-token:***", 1)
	}
	return url
}

func notifyCallback(status string) {
	if cfg.CallbackURL == "" || cfg.CallbackToken == "" {
		return
	}
	body, err := json.Marshal(map[string]string{
		"atlas_id": cfg.AtlasID,
		"status":   status,
	})
	if err != nil {
		log.Printf("Failed to marshal callback body: %v", err)
		return
	}
	req, _ := http.NewRequest(http.MethodPost, cfg.CallbackURL, bytes.NewReader(body))
	req.Header.Set("Authorization", cfg.CallbackToken)
	req.Header.Set("Content-Type", "application/json")
	_, _ = http.DefaultClient.Do(req)
}

func terminalHandler(w http.ResponseWriter, r *http.Request) {
	logWithRequestID(r, "Terminal connection attempt from %s", r.RemoteAddr)

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logWithRequestID(r, "WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()
	logWithRequestID(r, "WebSocket connection established")

	for {
		if isReady() {
			logWithRequestID(r, "Workspace ready, starting terminal")
			break
		}
		streamLogs(conn)
		time.Sleep(time.Second)
	}

	cmd := exec.Command("/bin/bash")
	ptmx, err := pty.Start(cmd)
	if err != nil {
		logWithRequestID(r, "Failed to start PTY: %v", err)
		_ = conn.WriteMessage(websocket.TextMessage, []byte("failed to start pty"))
		return
	}
	defer ptmx.Close()
	logWithRequestID(r, "PTY started successfully")

	go func() {
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				logWithRequestID(r, "WebSocket read error: %v", err)
				return
			}
			_, _ = ptmx.Write(msg)
		}
	}()

	buf := make([]byte, 1024)
	for {
		n, err := ptmx.Read(buf)
		if err != nil {
			logWithRequestID(r, "PTY read error: %v", err)
			return
		}
		_ = conn.WriteMessage(websocket.TextMessage, buf[:n])
	}
}

func streamLogs(conn *websocket.Conn) {
	mu.RLock()
	defer mu.RUnlock()
	scan := bufio.NewScanner(bytes.NewReader(cloneLog.Bytes()))
	for scan.Scan() {
		_ = conn.WriteMessage(websocket.TextMessage, scan.Bytes())
	}
}

func fileHandler(w http.ResponseWriter, r *http.Request) {
	if !isReady() {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}
	fs := http.FileServer(http.Dir("/workspace"))
	// strip /files prefix
	http.StripPrefix("/files", fs).ServeHTTP(w, r)
}

func fileContentHandler(w http.ResponseWriter, r *http.Request) {
	if !isReady() {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path required", 400)
		return
	}

	// Validate and sanitize path
	if !isValidPath(path) {
		logWithRequestID(r, "Invalid path attempted: %s", path)
		http.Error(w, "invalid path", 400)
		return
	}

	fullPath := filepath.Join("/workspace", path)
	if !strings.HasPrefix(fullPath, "/workspace") {
		logWithRequestID(r, "Path traversal attempt: %s", path)
		http.Error(w, "invalid path", 400)
		return
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			logWithRequestID(r, "File not found: %s", path)
			http.Error(w, "file not found", 404)
		} else {
			logWithRequestID(r, "File stat error for %s: %v", path, err)
			http.Error(w, err.Error(), 500)
		}
		return
	}

	if info.IsDir() {
		logWithRequestID(r, "Directory access attempted for content endpoint: %s", path)
		http.Error(w, "path is a directory", 400)
		return
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		logWithRequestID(r, "Failed to read file %s: %v", path, err)
		http.Error(w, err.Error(), 500)
		return
	}

	// Return file content with metadata
	response := map[string]interface{}{
		"content": string(data),
		"size":    info.Size(),
		"modTime": info.ModTime().Unix(),
		"path":    path,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logWithRequestID(r, "Failed to encode file content response: %v", err)
		http.Error(w, "failed to encode response", 500)
	} else {
		logWithRequestID(r, "Successfully served file content: %s (%d bytes)", path, len(data))
	}
}

func isValidPath(path string) bool {
	// Basic path validation
	if path == "" || strings.Contains(path, "..") {
		return false
	}

	// Check for absolute paths - only allow relative paths within workspace
	if filepath.IsAbs(path) {
		return false
	}

	// Check for suspicious characters
	suspiciousChars := []string{"\x00", "<", ">", "|", ";", "&", "$", "`", "(", ")", "{", "}", "[", "]"}
	for _, char := range suspiciousChars {
		if strings.Contains(path, char) {
			return false
		}
	}

	// Normalize the path and ensure it stays within workspace
	cleanPath := filepath.Clean(path)
	if strings.HasPrefix(cleanPath, "..") {
		return false
	}

	// Ensure path doesn't start with a dot (hidden files)
	if strings.HasPrefix(filepath.Base(cleanPath), ".") {
		return false
	}

	return true
}

func getMimeType(filePath string) string {
	// First try using the standard library
	mimeType := mime.TypeByExtension(filepath.Ext(filePath))
	if mimeType != "" {
		return mimeType
	}

	// If that fails, try reading the file header
	file, err := os.Open(filePath)
	if err != nil {
		return "application/octet-stream"
	}
	defer file.Close()

	// Read first 512 bytes for MIME type detection
	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil {
		return "application/octet-stream"
	}

	// Use http.DetectContentType
	return http.DetectContentType(buffer[:n])
}

func fileListHandler(w http.ResponseWriter, r *http.Request) {
	if !isReady() {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}

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

	rootPath := "/workspace"
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

			// Get file permissions
			perms := ""
			if info != nil {
				perms = info.Mode().String()
			}

			// Get file extension
			ext := filepath.Ext(entry.Name())

			// Get MIME type for files
			mimeType := ""
			if !entry.IsDir() && info != nil {
				mimeType = getMimeType(fullPath)
			}

			node := &FileNode{
				Name:        entry.Name(),
				IsDir:       entry.IsDir(),
				Path:        strings.TrimPrefix(fullPath, rootPath),
				Size:        info.Size(),
				ModTime:     info.ModTime(),
				Permissions: perms,
				Extension:   ext,
				MimeType:    mimeType,
			}
			if node.IsDir {
				node.Nodes, _ = walk(fullPath)
			}
			nodes = append(nodes, node)
		}
		return nodes, nil
	}

	nodes, err := walk(rootPath)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	json.NewEncoder(w).Encode(nodes)
}

func fileSaveHandler(w http.ResponseWriter, r *http.Request) {
	if !isReady() {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		http.Error(w, "method not allowed", 405)
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path required", 400)
		return
	}

	// Validate and sanitize path
	if !isValidPath(path) {
		http.Error(w, "invalid path", 400)
		return
	}

	fullPath := filepath.Join("/workspace", path)
	if !strings.HasPrefix(fullPath, "/workspace") {
		http.Error(w, "invalid path", 400)
		return
	}

	// Check content length limit
	const maxFileSize = 10 * 1024 * 1024 // 10MB
	if r.ContentLength > maxFileSize {
		http.Error(w, "file too large", 413)
		return
	}

	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	f, err := os.Create(fullPath)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer f.Close()

	// Limited reader to prevent excessive data
	limitedReader := io.LimitReader(r.Body, maxFileSize)
	if _, err := io.Copy(f, limitedReader); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	logWithRequestID(r, "Successfully saved file: %s (%d bytes)", path, r.ContentLength)
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "saved")
}

func autoCommitLoop() {
	for {
		time.Sleep(5 * time.Minute)
		if !isReady() {
			continue
		}
		_ = commitAll("Auto-save")
	}
}

func commitAll(msg string) error {
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = "/workspace"
	if err := cmd.Run(); err != nil {
		return err
	}
	cmd = exec.Command("git", "commit", "-m", msg)
	cmd.Dir = "/workspace"
	return cmd.Run()
}

func gracefulShutdown() {
	if !isReady() {
		return
	}
	log.Println("performing final sync...")
	_ = commitAll("Session end sync")
	cmd := exec.Command("git", "push", "origin", "main")
	cmd.Dir = "/workspace"
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("final push failed: %v, out: %s", err, string(out))
	} else {
		log.Println("final sync successful")
	}
}

func portWatcher() {
	// Common development ports to watch
	watchPorts := []int{
		3000, 3001, 3002, 3003, 3004, 3005, // Common dev servers
		4000, 4001, 4002, 4003, 4004, 4005, // Alternative dev ports
		5000, 5001, 5002, 5003, 5004, 5005, // More dev ports
		6000, 6001, 6002, 6003, 6004, 6005, // Even more dev ports
		7000, 7001, 7002, 7003, 7004, 7005, // Additional dev ports
		8000, 8001, 8002, 8003, 8004, 8005, // Common dev ports
		8080, 8081, 8082, 8083, 8084, 8085, // HTTP alternatives
		9000, 9001, 9002, 9003, 9004, 9005, // More dev ports
	}

	for {
		time.Sleep(5 * time.Second) // Increased from 2s to 5s

		// Batch scan ports for efficiency
		for _, port := range watchPorts {
			portWatcherMu.Lock()
			if knownPorts[port] {
				portWatcherMu.Unlock()
				continue
			}
			portWatcherMu.Unlock()

			conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 100*time.Millisecond)
			if err != nil {
				continue
			}
			conn.Close()

			portWatcherMu.Lock()
			knownPorts[port] = true
			portWatcherMu.Unlock()

			notifyPort(port)
		}
	}
}

func notifyPort(port int) {
	if cfg.AtlasID == "" {
		log.Printf("AtlasID not configured, skipping port notification")
		return
	}

	body := map[string]interface{}{
		"port":   port,
		"public": false,
	}
	data, err := json.Marshal(body)
	if err != nil {
		log.Printf("Failed to marshal port notification: %v", err)
		return
	}

	// Use Atlas base URL from environment or default to localhost:8080
	atlasURL := getenv("ATLAS_BASE_URL", "http://localhost:8080")
	url := fmt.Sprintf("%s/sandboxes/%s/ports", atlasURL, cfg.AtlasID)

	resp, err := http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		log.Printf("Failed to notify Atlas about port %d: %v", port, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		log.Printf("Atlas returned error status %d for port %d notification", resp.StatusCode, port)
		return
	}

	log.Printf("Successfully notified Atlas about port %d", port)
}

func setReady() {
	mu.Lock()
	defer mu.Unlock()
	ready = true
}

func isReady() bool {
	mu.RLock()
	defer mu.RUnlock()
	return ready
}
