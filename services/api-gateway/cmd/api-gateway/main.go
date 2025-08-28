package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/Aadithya-J/code_nest/proto"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"google.golang.org/grpc"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}
	authServiceURL := os.Getenv("AUTH_SERVICE_URL")
	if authServiceURL == "" {
		authServiceURL = "localhost:50051"
	}

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	conn, err := grpc.Dial(authServiceURL, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("failed to dial auth service: %v", err)
	}
	defer conn.Close()
	authClient := proto.NewAuthServiceClient(conn)

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

	r.GET("/protected", func(c *gin.Context) {
		token := c.GetHeader("Authorization")
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			return
		}

		// call gRPC ValidateToken
		resp, err := authClient.ValidateToken(context.Background(), &proto.ValidateTokenRequest{Token: token})
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication failed: " + err.Error()})
			return
		}
		if !resp.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": resp.Error})
			return
		}

		// inject user
		c.Set("user_id", resp.UserId)
		c.JSON(http.StatusOK, gin.H{"message": "Hello " + resp.UserId})
	})

	log.Println("Starting API Gateway on :8080")
	if err := r.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}
