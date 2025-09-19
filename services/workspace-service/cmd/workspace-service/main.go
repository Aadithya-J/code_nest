package main

import (
	"fmt"
	"log"
	"net"

	"github.com/Aadithya-J/code_nest/proto"
	"github.com/Aadithya-J/code_nest/services/workspace-service/internal/config"
	"github.com/Aadithya-J/code_nest/services/workspace-service/internal/db"
	"github.com/Aadithya-J/code_nest/services/workspace-service/internal/kafka"
	"github.com/Aadithya-J/code_nest/services/workspace-service/internal/models"
	"github.com/Aadithya-J/code_nest/services/workspace-service/internal/repository"
	"github.com/Aadithya-J/code_nest/services/workspace-service/internal/service"
	"google.golang.org/grpc"
)

func main() {
	cfg := config.LoadConfig()

	gormDB := db.Init(cfg.DatabaseURL)

	createSchema := fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", cfg.DBSchema)
	gormDB.Exec(createSchema)
	gormDB.Exec(fmt.Sprintf("SET search_path TO %s", cfg.DBSchema))

	log.Println("Running migrations...")
	if err := gormDB.AutoMigrate(&models.Project{}); err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}
	log.Println("Migrations completed.")

	kafkaProducer := kafka.NewProducer(cfg.KafkaBrokerURL, cfg.KafkaTopicCmd)
	defer kafkaProducer.Close()
	log.Println("Kafka producer initialized.")

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.ServicePort))
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	projectRepo := repository.NewProjectRepository(gormDB)
	workspaceSvc := service.NewWorkspaceService(projectRepo, kafkaProducer)

	s := grpc.NewServer()
	proto.RegisterWorkspaceServiceServer(s, workspaceSvc)

	log.Printf("Workspace service listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
