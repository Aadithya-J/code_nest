package main

import (
	"context"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"

	"github.com/aadithya/code_nest/proto"
)

func main() {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	// setup gRPC client to AuthService
	conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
	if err != nil {
		log.Fatalf("failed to dial auth service: %v", err)
	}
	defer conn.Close()
	authClient := proto.NewAuthServiceClient(conn)

	r.GET("/protected", func(c *gin.Context) {
		token := c.GetHeader("Authorization")
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			return
		}

		// call gRPC ValidateToken
		resp, err := authClient.ValidateToken(context.Background(), &proto.ValidateTokenRequest{Token: token})
		if err != nil || !resp.Valid {
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
