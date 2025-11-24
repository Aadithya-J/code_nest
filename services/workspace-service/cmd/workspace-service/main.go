package main

import (
	"context"
	"fmt"
	"log"
	"net"

	"github.com/Aadithya-J/code_nest/proto"
	"github.com/Aadithya-J/code_nest/services/workspace-service/internal/config"
	"github.com/Aadithya-J/code_nest/services/workspace-service/internal/db"
	"github.com/Aadithya-J/code_nest/services/workspace-service/internal/k8s"
	"github.com/Aadithya-J/code_nest/services/workspace-service/internal/models"
	"github.com/Aadithya-J/code_nest/services/workspace-service/internal/rabbitmq"
	"github.com/Aadithya-J/code_nest/services/workspace-service/internal/repository"
	"github.com/Aadithya-J/code_nest/services/workspace-service/internal/service"
	"google.golang.org/grpc"
)

func main() {
	cfg := config.LoadConfig()

	gormDB := db.Init(cfg.DatabaseURL, cfg.DBSchema)

	createSchema := fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", cfg.DBSchema)
	gormDB.Exec(createSchema)
	gormDB.Exec(fmt.Sprintf("SET search_path TO %s", cfg.DBSchema))

	log.Println("Running migrations...")
	if err := gormDB.AutoMigrate(&models.Project{}, &models.File{}, &models.WorkspaceSession{}); err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}
	log.Println("Migrations completed.")

	rabbitMQProducer, err := rabbitmq.NewProducer(cfg.RabbitMQURL)
	if err != nil {
		log.Fatalf("Failed to initialize RabbitMQ producer: %v", err)
	}
	defer rabbitMQProducer.Close()
	log.Println("RabbitMQ producer initialized.")

	// Initialize Kubernetes client
	k8sClient, err := k8s.NewClient(cfg.KubeconfigPath, cfg.WorkspaceNamespace)
	if err != nil {
		log.Fatalf("Failed to initialize Kubernetes client: %v", err)
	}
	log.Println("Kubernetes client initialized.")

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.ServicePort))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	projectRepo := repository.NewProjectRepository(gormDB)
	fileRepo := repository.NewFileRepository(gormDB)
	sessionRepo := repository.NewSessionRepository(gormDB)

	workspaceSvc := service.NewWorkspaceService(projectRepo, fileRepo, sessionRepo, rabbitMQProducer, k8sClient)
	sessionSvc := service.NewSessionService(rabbitMQProducer, projectRepo, sessionRepo)


	// Initialize and start RabbitMQ Consumer
	rabbitMQConsumer, err := rabbitmq.NewConsumer(cfg.RabbitMQURL, sessionRepo)
	if err != nil {
		log.Fatalf("Failed to initialize RabbitMQ consumer: %v", err)
	}
	defer rabbitMQConsumer.Close()

	go func() {
		if err := rabbitMQConsumer.Start(context.Background()); err != nil {
			log.Printf("RabbitMQ consumer error: %v", err)
		}
	}()

	s := grpc.NewServer()
	proto.RegisterWorkspaceServiceServer(s, workspaceSvc)
	proto.RegisterSessionServiceServer(s, sessionSvc)

	log.Printf("Workspace service listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
