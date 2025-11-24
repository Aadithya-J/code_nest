package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Aadithya-J/code_nest/proto"
	"github.com/Aadithya-J/code_nest/services/api-gateway/internal/config"
	"github.com/Aadithya-J/code_nest/services/api-gateway/internal/rabbitmq"
	"github.com/Aadithya-J/code_nest/services/api-gateway/internal/websocket"
	"github.com/MicahParks/keyfunc"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	cfg := config.LoadConfig()

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	authConn, err := grpc.Dial(cfg.AuthSvcUrl, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("failed to dial auth service: %v", err)
	}
	defer authConn.Close()
	authClient := proto.NewAuthServiceClient(authConn)

	workspaceConn, err := grpc.Dial(cfg.WorkspaceSvcUrl, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("failed to dial workspace service: %v", err)
	}
	defer workspaceConn.Close()
	workspaceClient := proto.NewWorkspaceServiceClient(workspaceConn)
	sessionClient := proto.NewSessionServiceClient(workspaceConn)

	jwksURL := cfg.AuthSvcJWKSUrl
	options := keyfunc.Options{
		RefreshInterval: time.Hour,
		RefreshTimeout:  10 * time.Second,
	}
	var jwks *keyfunc.JWKS

	maxWait := 15 * time.Second
	checkInterval := 1 * time.Second
	startTime := time.Now()

	for {
		jwks, err = keyfunc.Get(jwksURL, options)
		if err == nil {
			log.Println("Successfully fetched JWKS.")
			break
		}

		if time.Since(startTime) > maxWait {
			log.Fatalf("Failed to create JWKS from resource at %s after %s: %s", jwksURL, maxWait, err)
		}

		log.Printf("Waiting for auth service JWKS... retrying in %s", checkInterval)
		time.Sleep(checkInterval)
	}

	// Initialize WebSocket Hub
	wsHub := websocket.NewHub()

	// Initialize RabbitMQ Consumer
	consumer, err := rabbitmq.NewConsumer(cfg.RabbitMQURL, wsHub)
	if err != nil {
		log.Fatalf("Failed to create RabbitMQ consumer: %v", err)
	}
	defer consumer.Close()

	// Start consumer in background
	go func() {
		if err := consumer.Start(context.Background()); err != nil {
			log.Printf("RabbitMQ consumer error: %v", err)
		}
	}()

	authMiddleware := NewAuthMiddleware(jwks)

	auth := r.Group("/auth")
	{
		auth.POST("/signup", func(c *gin.Context) {
			var req proto.SignupRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			// Validate required fields
			if req.Email == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "email is required"})
				return
			}
			if req.Password == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "password is required"})
				return
			}
			if len(req.Password) < 6 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "password must be at least 6 characters"})
				return
			}

			resp, err := authClient.Signup(context.Background(), &req)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, resp)
		})
		auth.POST("/login", func(c *gin.Context) {
			var req proto.LoginRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			// Validate required fields
			if req.Email == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "email is required"})
				return
			}
			if req.Password == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "password is required"})
				return
			}

			resp, err := authClient.Login(context.Background(), &req)
			if err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, resp)
		})
		auth.GET("/google/login", func(c *gin.Context) {
			// Generate secure random state for CSRF protection
			state := generateSecureState()
			resp, err := authClient.GetGoogleAuthURL(context.Background(), &proto.GetGoogleAuthURLRequest{State: state})
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.Redirect(http.StatusTemporaryRedirect, resp.Url)
		})
		auth.GET("/google/callback", func(c *gin.Context) {
			code := c.Query("code")
			if code == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "code query param required"})
				return
			}
			resp, err := authClient.HandleGoogleCallback(context.Background(), &proto.HandleGoogleCallbackRequest{Code: code})
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, resp)
		})

		// GitHub OAuth routes
		auth.GET("/github/login", func(c *gin.Context) {
			resp, err := authClient.GetGitHubAuthURL(context.Background(), &proto.GetGitHubAuthURLRequest{})
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.Redirect(http.StatusTemporaryRedirect, resp.Url)
		})

		auth.GET("/github/callback", authMiddleware.Authorize, func(c *gin.Context) {
			installationID := c.Query("installation_id")
			setupAction := c.Query("setup_action")
			if installationID == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "installation_id query param required"})
				return
			}

			// Convert installation_id to int64
			var instID int64
			if _, err := fmt.Sscanf(installationID, "%d", &instID); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid installation_id"})
				return
			}

			// Get authenticated user from JWT
			userID := c.GetString("user_id")

			resp, err := authClient.HandleGitHubCallback(context.Background(), &proto.HandleGitHubCallbackRequest{
				InstallationId: instID,
				SetupAction:    setupAction,
				UserId:         userID, // Pass the authenticated user
			})
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, resp)
		})

		// GitHub status endpoint - only available in development
		if os.Getenv("APP_ENV") == "development" || os.Getenv("APP_ENV") == "" {
			auth.GET("/github/status", authMiddleware.Authorize, func(c *gin.Context) {
				userID := c.GetString("user_id")

				// Check if user has GitHub linked by trying to get a token
				tokenResp, err := authClient.GetGitHubAccessToken(context.Background(), &proto.GetGitHubAccessTokenRequest{
					UserId: userID,
				})

				githubLinked := false
				var message string

				if err != nil {
					// User doesn't have GitHub linked or token generation failed
					message = "GitHub not linked or token unavailable"
				} else if tokenResp.Token != "" {
					// User has GitHub linked and token is available
					githubLinked = true
					message = "GitHub linked successfully"
				}

				c.JSON(http.StatusOK, gin.H{
					"user_id":       userID,
					"github_linked": githubLinked,
					"message":       message + " (dev only)",
				})
			})
		}
	}

	workspace := r.Group("/workspace", authMiddleware.Authorize)
	{
		workspace.POST("/projects", func(c *gin.Context) {
			var req proto.CreateProjectRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			req.UserId = c.GetString("user_id")
			resp, err := workspaceClient.CreateProject(context.Background(), &req)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, resp)
		})

		workspace.GET("/projects", func(c *gin.Context) {
			req := proto.GetProjectsRequest{UserId: c.GetString("user_id")}
			resp, err := workspaceClient.GetProjects(context.Background(), &req)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, resp)
		})

		workspace.PUT("/projects/:id", func(c *gin.Context) {
			var req proto.UpdateProjectRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			req.Id = c.Param("id")
			req.UserId = c.GetString("user_id")
			resp, err := workspaceClient.UpdateProject(context.Background(), &req)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, resp)
		})

		workspace.POST("/files", func(c *gin.Context) {
			var req proto.SaveFileRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			req.UserId = c.GetString("user_id")
			resp, err := workspaceClient.SaveFile(context.Background(), &req)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, resp)
		})

		workspace.GET("/file", func(c *gin.Context) {
			projectID := c.Query("projectId")
			path := c.Query("path")
			if projectID == "" || path == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "projectId and path required"})
				return
			}
			req := proto.GetFileRequest{ProjectId: projectID, Path: path, UserId: c.GetString("user_id")}
			resp, err := workspaceClient.GetFile(context.Background(), &req)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, resp)
		})

		workspace.GET("/files", func(c *gin.Context) {
			projectID := c.Query("projectId")
			if projectID == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "projectId required"})
				return
			}
			req := proto.ListFilesRequest{ProjectId: projectID, UserId: c.GetString("user_id")}
			resp, err := workspaceClient.ListFiles(context.Background(), &req)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, resp)
		})

		workspace.DELETE("/projects/:id", func(c *gin.Context) {
			req := proto.DeleteProjectRequest{
				Id:     c.Param("id"),
				UserId: c.GetString("user_id"),
			}
			resp, err := workspaceClient.DeleteProject(context.Background(), &req)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, resp)
		})

		// Session endpoints
		workspace.POST("/sessions", func(c *gin.Context) {
			var req proto.CreateWorkspaceSessionRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			req.UserId = c.GetString("user_id")
			resp, err := sessionClient.CreateWorkspaceSession(context.Background(), &req)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusAccepted, resp)
		})

		workspace.DELETE("/sessions/:id", func(c *gin.Context) {
			projectID := c.Query("projectId")
			if projectID == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "projectId query parameter required"})
				return
			}
			req := proto.ReleaseWorkspaceSessionRequest{
				SessionId: c.Param("id"),
				ProjectId: projectID,
				UserId:    c.GetString("user_id"),
			}
			resp, err := sessionClient.ReleaseWorkspaceSession(context.Background(), &req)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, resp)
		})

		// RESTful file routes under projects
		workspace.GET("/projects/:projectId/files/tree", func(c *gin.Context) {
			projectID := c.Param("projectId")
			req := proto.GetFileTreeRequest{
				ProjectId: projectID,
				UserId:    c.GetString("user_id"),
			}
			resp, err := workspaceClient.GetFileTree(context.Background(), &req)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, resp)
		})
	}

	r.GET("/protected", authMiddleware.Authorize, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "Hello " + c.GetString("user_id")})
	})

	// WebSocket endpoint for status updates
	r.GET("/ws/status", func(c *gin.Context) {
		wsHub.HandleConnection(c)
	})

	// Development-only cleanup endpoint
	if os.Getenv("APP_ENV") == "development" || os.Getenv("APP_ENV") == "" {
		r.POST("/dev/cleanup", func(c *gin.Context) {
			log.Println("🧹 [DEV] Cleaning up all workspace sessions...")
			
			// Get all active sessions
			sessionsResp, err := sessionClient.GetAllActiveSessions(context.Background(), &proto.GetAllActiveSessionsRequest{})
			if err != nil {
				log.Printf("Failed to get active sessions: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": "Failed to get active sessions",
				})
				return
			}

			released := 0
			failed := 0

			// Release each session
			for _, session := range sessionsResp.Sessions {
				_, err := sessionClient.ReleaseWorkspaceSession(context.Background(), &proto.ReleaseWorkspaceSessionRequest{
					SessionId: session.SessionId,
					ProjectId: session.ProjectId,
					UserId:    session.UserId,
				})
				if err != nil {
					log.Printf("Failed to release session %s: %v", session.SessionId, err)
					failed++
				} else {
					released++
				}
			}

			log.Printf("✅ [DEV] Cleanup complete: %d released, %d failed", released, failed)
			c.JSON(http.StatusOK, gin.H{
				"message":  "Cleanup complete",
				"released": released,
				"failed":   failed,
			})
		})
	}


	log.Printf("Starting API Gateway on :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}

type AuthMiddleware struct {
	jwks *keyfunc.JWKS
}

func NewAuthMiddleware(jwks *keyfunc.JWKS) *AuthMiddleware {
	return &AuthMiddleware{jwks: jwks}
}

func (am *AuthMiddleware) Authorize(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
		return
	}

	tokenString := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenString == authHeader {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token format"})
		return
	}

	token, err := jwt.Parse(tokenString, am.jwks.Keyfunc)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "token validation failed: " + err.Error()})
		return
	}

	if !token.Valid {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "failed to parse claims"})
		return
	}

	sub, ok := claims["sub"].(string)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "'sub' claim is missing or invalid"})
		return
	}

	c.Set("user_id", sub)
	c.Next()
}

// generateSecureState creates a cryptographically secure random state for OAuth
func generateSecureState() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}
