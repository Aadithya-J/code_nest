package handler

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Aadithya-J/code_nest/proto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type AuthClient interface {
	Signup(context.Context, *proto.SignupRequest) (*proto.AuthResponse, error)
	Login(context.Context, *proto.LoginRequest) (*proto.AuthResponse, error)
	GetGoogleAuthURL(context.Context, *proto.GetGoogleAuthURLRequest) (*proto.GetGoogleAuthURLResponse, error)
	HandleGoogleCallback(context.Context, *proto.HandleGoogleCallbackRequest) (*proto.AuthResponse, error)
	GetGitHubAuthURL(context.Context, *proto.GetGitHubAuthURLRequest) (*proto.GetGitHubAuthURLResponse, error)
	HandleGitHubCallback(context.Context, *proto.HandleGitHubCallbackRequest) (*proto.AuthResponse, error)
	ValidateToken(context.Context, string) (*proto.ValidateTokenResponse, error)
	GetGitHubAccessToken(ctx context.Context, req *proto.GetGitHubAccessTokenRequest) (*proto.GetGitHubAccessTokenResponse, error)
}

type ProjectClient interface {
	CreateProject(ctx context.Context, req *proto.CreateProjectRequest) (*proto.CreateProjectResponse, error)
	StartWorkspace(ctx context.Context, req *proto.StartWorkspaceRequest) (*proto.StartWorkspaceResponse, error)
	VerifyAndComplete(ctx context.Context, req *proto.VerifyAndCompleteRequest) (*proto.VerifyAndCompleteResponse, error)
	IsOwner(ctx context.Context, req *proto.IsOwnerRequest) (*proto.IsOwnerResponse, error)
}

type Handler struct {
	auth    AuthClient
	project ProjectClient
	db      *gorm.DB
	redis   *redis.Client
}

func New(auth AuthClient, project ProjectClient, db *gorm.DB, redis *redis.Client) *Handler {
	return &Handler{auth: auth, project: project, db: db, redis: redis}
}

func (h *Handler) errorResponse(c *gin.Context, statusCode int, message string, err error) {
	requestID := c.GetHeader("X-Request-ID")
	if requestID == "" {
		requestID = fmt.Sprintf("%.8s", uuid.New().String())
	}

	errorResponse := gin.H{
		"error":      message,
		"request_id": requestID,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"endpoint":   c.Request.URL.Path,
	}

	if err != nil {
		errorResponse["details"] = err.Error()
	}

	c.JSON(statusCode, errorResponse)
}

func (h *Handler) Register(r *gin.Engine) {
	api := r.Group("/api")
	{
		api.GET("/health", h.Health)
		api.POST("/auth/signup", h.Signup)
		api.POST("/auth/login", h.Login)
		api.GET("/auth/google/url", h.GetGoogleAuthURL)
		api.GET("/auth/google/callback", h.HandleGoogleCallback)
		api.GET("/auth/github/url", h.GetGitHubAuthURL)
		api.GET("/auth/github/callback", h.HandleGitHubCallback)
		api.POST("/projects", h.CreateProject)
		api.POST("/projects/:id/start", h.StartWorkspace)
		api.POST("/internal/webhook", h.HandleWebhookInternal)
	}

	r.GET("/auth/verify", h.VerifyRequest)
}

func (h *Handler) Health(c *gin.Context) {
	c.JSON(200, gin.H{"status": "ok"})
}

func (h *Handler) Signup(c *gin.Context) {
	var body struct {
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required,min=6"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		h.errorResponse(c, 400, "Invalid request format", err)
		return
	}

	resp, err := h.auth.Signup(c.Request.Context(), &proto.SignupRequest{
		Email:    body.Email,
		Password: body.Password,
	})
	if err != nil {
		h.errorResponse(c, 500, "Internal server error", err)
		return
	}
	c.JSON(200, gin.H{"token": resp.GetToken()})
}

// CreateProject inserts a project row (STOPPED).
func (h *Handler) CreateProject(c *gin.Context) {
	if h.project == nil {
		h.errorResponse(c, 500, "Service unavailable", nil)
		return
	}
	token := bearer(c.GetHeader("Authorization"))
	if token == "" {
		c.Status(401)
		return
	}
	authResp, err := h.auth.ValidateToken(c.Request.Context(), token)
	if err != nil || !authResp.GetValid() {
		c.Status(401)
		return
	}

	var body struct {
		Name    string `json:"name" binding:"required"`
		RepoURL string `json:"repoUrl" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		h.errorResponse(c, 400, "Invalid request format", err)
		return
	}

	resp, err := h.project.CreateProject(c.Request.Context(), &proto.CreateProjectRequest{
		UserId:  authResp.GetUserId(),
		Name:    body.Name,
		RepoUrl: body.RepoURL,
	})
	if err != nil {
		h.errorResponse(c, 500, "Internal server error", err)
		return
	}
	c.JSON(200, gin.H{"projectId": resp.GetProjectId()})
}

// StartWorkspace is a convenience HTTP entrypoint that forwards to project-service.StartWorkspace.
func (h *Handler) StartWorkspace(c *gin.Context) {
	if h.project == nil {
		h.errorResponse(c, 500, "Service unavailable", nil)
		return
	}
	token := bearer(c.GetHeader("Authorization"))
	if token == "" {
		c.Status(401)
		return
	}

	authResp, err := h.auth.ValidateToken(c.Request.Context(), token)
	if err != nil || !authResp.GetValid() {
		c.Status(401)
		return
	}
	userID := authResp.GetUserId()

	projectID := c.Param("id")
	if projectID == "" {
		h.errorResponse(c, 400, "Project ID required", nil)
		return
	}

	resp, err := h.project.StartWorkspace(c.Request.Context(), &proto.StartWorkspaceRequest{
		ProjectId: projectID,
		UserId:    userID,
	})
	if err != nil {
		h.errorResponse(c, 500, "Internal server error", err)
		return
	}
	if !resp.GetOk() {
		h.errorResponse(c, 400, "Failed to start workspace", nil)
		return
	}

	c.JSON(200, gin.H{
		"ok":      true,
		"status":  resp.GetStatus().String(),
		"atlasId": resp.GetAtlasId(),
	})
}

func (h *Handler) Login(c *gin.Context) {
	var body struct {
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		h.errorResponse(c, 400, "Invalid request format", err)
		return
	}

	resp, err := h.auth.Login(c.Request.Context(), &proto.LoginRequest{
		Email:    body.Email,
		Password: body.Password,
	})
	if err != nil {
		h.errorResponse(c, 401, "Authentication failed", err)
		return
	}
	c.JSON(200, gin.H{"token": resp.GetToken()})
}

func (h *Handler) GetGoogleAuthURL(c *gin.Context) {
	state := c.Query("state")
	resp, err := h.auth.GetGoogleAuthURL(c.Request.Context(), &proto.GetGoogleAuthURLRequest{
		State: state,
	})
	if err != nil {
		h.errorResponse(c, 500, "Internal server error", err)
		return
	}
	c.JSON(200, gin.H{"url": resp.GetUrl()})
}

func (h *Handler) HandleGoogleCallback(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		h.errorResponse(c, 400, "Code parameter required", nil)
		return
	}
	resp, err := h.auth.HandleGoogleCallback(c.Request.Context(), &proto.HandleGoogleCallbackRequest{
		Code: code,
	})
	if err != nil {
		h.errorResponse(c, 500, "Internal server error", err)
		return
	}
	c.JSON(200, gin.H{"token": resp.GetToken()})
}

func (h *Handler) GetGitHubAuthURL(c *gin.Context) {
	state := c.Query("state")
	if state == "" {
		token := bearer(c.GetHeader("Authorization"))
		if token != "" {
			if authResp, err := h.auth.ValidateToken(c.Request.Context(), token); err == nil && authResp.GetValid() {
				state = authResp.GetUserId()
			}
		}
	}

	resp, err := h.auth.GetGitHubAuthURL(c.Request.Context(), &proto.GetGitHubAuthURLRequest{})
	if err != nil {
		h.errorResponse(c, 500, "Internal server error", err)
		return
	}

	urlStr := resp.GetUrl()
	if state != "" {
		if parsed, err := url.Parse(urlStr); err == nil {
			q := parsed.Query()
			q.Set("state", state)
			parsed.RawQuery = q.Encode()
			urlStr = parsed.String()
		}
	}

	c.JSON(200, gin.H{"url": urlStr})
}

func (h *Handler) HandleGitHubCallback(c *gin.Context) {
	installationStr := c.Query("installation_id")
	state := c.Query("state")
	if state == "" {
		c.JSON(400, gin.H{"error": "state required"})
		return
	}

	var installationID int64
	if installationStr != "" {
		parsed, err := strconv.ParseInt(installationStr, 10, 64)
		if err != nil {
			c.JSON(400, gin.H{"error": "invalid installation_id"})
			return
		}
		installationID = parsed
	}

	resp, err := h.auth.HandleGitHubCallback(c.Request.Context(), &proto.HandleGitHubCallbackRequest{
		UserId:         state,
		InstallationId: installationID,
	})
	if err != nil {
		h.errorResponse(c, 500, "Internal server error", err)
		return
	}
	if resp.GetError() != "" {
		h.errorResponse(c, 400, resp.GetError(), nil)
		return
	}

	redirect := c.Query("redirect")
	if redirect != "" {
		if target, err := url.Parse(redirect); err == nil {
			// Use fragment instead of query to avoid token exposure in logs/history
			fragment := url.Values{}
			fragment.Set("token", resp.GetToken())
			target.Fragment = fragment.Encode()
			c.Redirect(302, target.String())
			return
		}
	}

	c.JSON(200, gin.H{"token": resp.GetToken()})
}

// HandleWebhookInternal proxies agent callbacks to project-service for validation and status update.
func (h *Handler) HandleWebhookInternal(c *gin.Context) {
	if h.project == nil {
		h.errorResponse(c, 500, "Service unavailable", nil)
		return
	}
	token := c.GetHeader("Authorization")
	var body struct {
		ID     string `json:"id" binding:"required"`     // atlas id
		Status string `json:"status" binding:"required"` // READY or ERROR
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		h.errorResponse(c, 400, "Invalid request format", err)
		return
	}
	if token == "" {
		h.errorResponse(c, 400, "Authorization required", nil)
		return
	}
	resp, err := h.project.VerifyAndComplete(c.Request.Context(), &proto.VerifyAndCompleteRequest{
		AtlasId:       body.ID,
		CallbackToken: token,
		Status:        body.Status,
	})
	if err != nil || !resp.GetOk() {
		h.errorResponse(c, 403, "Forbidden", err)
		return
	}
	c.JSON(200, gin.H{"ok": true, "status": resp.GetStatus().String()})
}

func (h *Handler) VerifyRequest(c *gin.Context) {
	token := bearer(c.GetHeader("Authorization"))
	if token == "" {
		token = c.GetHeader("X-Forwarded-Access-Token")
	}
	if token == "" {
		if cookie, err := c.Cookie("auth_token"); err == nil {
			token = cookie
		}
	}
	if token == "" {
		c.Status(401)
		return
	}

	resp, err := h.auth.ValidateToken(c.Request.Context(), token)
	if err != nil || !resp.GetValid() {
		c.Status(401)
		return
	}

	userID := resp.GetUserId()

	host := c.GetHeader("X-Forwarded-Host")
	atlasID := parseAtlasID(host)
	if atlasID == "" {
		c.Status(403)
		return
	}

	cacheKey := "auth_decision:" + userID + ":" + atlasID
	if h.redis != nil {
		if allowed, _ := h.redis.Get(c.Request.Context(), cacheKey).Result(); allowed == "1" {
			c.Header("X-User-Id", userID)
			c.Status(200)
			return
		}
	}

	if h.project == nil {
		c.Status(500)
		return
	}
	own, err := h.project.IsOwner(c.Request.Context(), &proto.IsOwnerRequest{
		UserId:    userID,
		ProjectId: atlasID,
	})
	if err != nil || !own.GetIsOwner() {
		c.Status(403)
		return
	}

	if h.redis != nil {
		h.redis.Set(c.Request.Context(), cacheKey, "1", 30*time.Second)
	}

	c.Header("X-User-Id", userID)
	c.Status(200)
}

func parseAtlasID(host string) string {
	parts := strings.Split(host, ".")
	if len(parts) == 0 {
		return ""
	}
	sub := parts[0]

	// Handle standard ws- prefix
	if strings.HasPrefix(sub, "ws-") {
		return sub
	}

	// Handle legacy 3000-ws- prefix (for backward compatibility)
	if strings.HasPrefix(sub, "3000-ws-") {
		return "ws-" + strings.TrimPrefix(sub, "3000-ws-")
	}

	// Try to extract any ws- pattern from the subdomain
	if idx := strings.Index(sub, "ws-"); idx != -1 {
		return sub[idx:]
	}

	return ""
}

func bearer(h string) string {
	if h == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}
