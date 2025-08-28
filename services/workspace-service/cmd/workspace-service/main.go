package main

import (
	"fmt"
	"log"
	"net"

	"github.com/Aadithya-J/code_nest/proto"
	"github.com/Aadithya-J/code_nest/services/workspace-service/internal/config"
	"github.com/Aadithya-J/code_nest/services/workspace-service/internal/db"
	"github.com/Aadithya-J/code_nest/services/workspace-service/internal/models"
	"github.com/Aadithya-J/code_nest/services/workspace-service/internal/repository"
	"github.com/Aadithya-J/code_nest/services/workspace-service/internal/service"
	"google.golang.org/grpc"
)

func main() {
	// Load configuration
	cfg := config.LoadConfig()

	// Initialize database
	gormDB := db.Init(cfg.DatabaseURL)

	// Auto-migrate models
	log.Println("Running migrations...")
	if err := gormDB.AutoMigrate(&models.Project{}); err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}
	log.Println("Migrations completed.")

	// Create a listener
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.ServicePort))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	// Create repository and service
	projectRepo := repository.NewProjectRepository(gormDB)
	workspaceSvc := service.NewWorkspaceService(projectRepo)

	// Create gRPC server
	s := grpc.NewServer()
	proto.RegisterWorkspaceServiceServer(s, workspaceSvc)

	log.Printf("Workspace service listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
