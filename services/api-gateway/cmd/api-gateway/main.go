package main

import (
	"context"
	"log"
	"net/http"

	"github.com/Aadithya-J/code_nest/proto"
	"github.com/Aadithya-J/code_nest/services/api-gateway/internal/config"
	"github.com/gin-gonic/gin"
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

	// Auth middleware
	authMiddleware := func(c *gin.Context) {
		token := c.GetHeader("Authorization")
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			return
		}

		resp, err := authClient.ValidateToken(context.Background(), &proto.ValidateTokenRequest{Token: token})
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication failed: " + err.Error()})
			return
		}
		if !resp.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": resp.Error})
			return
		}

		c.Set("user_id", resp.UserId)
		c.Next()
	}

	auth := r.Group("/auth")
	{
		auth.POST("/signup", func(c *gin.Context) {
			var req proto.SignupRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
			resp, err := authClient.Login(context.Background(), &req)
			if err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, resp)
		})
		auth.GET("/google/login", func(c *gin.Context) {
			resp, err := authClient.GetGoogleAuthURL(context.Background(), &proto.GetGoogleAuthURLRequest{State: "state"})
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
	}

	workspace := r.Group("/workspace", authMiddleware)
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
	}

	r.GET("/protected", authMiddleware, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "Hello " + c.GetString("user_id")})
	})

	log.Printf("Starting API Gateway on :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}
