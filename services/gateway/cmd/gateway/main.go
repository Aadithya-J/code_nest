package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Aadithya-J/code_nest/services/gateway/internal/config"
	"github.com/Aadithya-J/code_nest/services/gateway/internal/handler"
	"github.com/Aadithya-J/code_nest/services/gateway/pkg/rpc"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg := config.Load()

	authClient, err := rpc.NewAuthClient(cfg.AuthRPCURL)
	if err != nil {
		log.Fatalf("failed to connect to auth-service: %v", err)
	}
	defer authClient.Close()

	projectClient, err := rpc.NewProjectClient(cfg.ProjectRPC)
	if err != nil {
		log.Fatalf("failed to connect to project-service: %v", err)
	}
	defer projectClient.Close()

	// Gateway doesn't need database connection - it only uses RPC clients
	redisClient := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})

	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())

	// Request ID middleware
	router.Use(func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = fmt.Sprintf("%.8s", uuid.New().String())
		}
		c.Header("X-Request-ID", requestID)
		c.Set("requestID", requestID)

		// Add request ID to context for downstream calls
		ctx := context.WithValue(c.Request.Context(), "requestID", requestID)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	})

	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{cfg.AllowOrigins},
		AllowCredentials: true,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Authorization", "Content-Type"},
	}))

	h := handler.New(authClient, projectClient, nil, redisClient)
	h.Register(router)

	server := &http.Server{
		Addr:           ":" + cfg.Port,
		Handler:        router,
		ReadTimeout:    15 * time.Second,
		WriteTimeout:   15 * time.Second,
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1MB max header size
	}

	go func() {
		log.Printf("gateway listening on :%s", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("gateway server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Println("shutting down gateway...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}
