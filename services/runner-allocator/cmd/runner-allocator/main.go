package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Aadithya-J/code_nest/services/runner-allocator/internal/auth"
	"github.com/Aadithya-J/code_nest/services/runner-allocator/internal/config"
	"github.com/Aadithya-J/code_nest/services/runner-allocator/internal/lifecycle"
	"github.com/Aadithya-J/code_nest/services/runner-allocator/internal/provisioner"
	"github.com/Aadithya-J/code_nest/services/runner-allocator/internal/rabbitmq"
	"github.com/Aadithya-J/code_nest/services/runner-allocator/internal/store"
)

func main() {
	cfg := config.LoadConfig()

	storeInstance := store.NewInMemoryStore(cfg.MaxSlots, cfg.QueueSize)

	kubernetesProvisioner, err := provisioner.NewKubernetesProvisioner(cfg.WorkspaceImage, cfg.KubeconfigPath)
	if err != nil {
		log.Fatalf("Failed to create Kubernetes provisioner: %v", err)
	}

	if err := kubernetesProvisioner.InitializeSlots(context.Background()); err != nil {
		log.Fatalf("Failed to initialize slots: %v", err)
	}

	lifecycleManager := lifecycle.NewManager(storeInstance, kubernetesProvisioner)
	ctx, cancel := context.WithCancel(context.Background())
	go lifecycleManager.Start(ctx)

	
	authClient, err := auth.NewClient(cfg.AuthServiceURL)
	if err != nil {
		log.Fatalf("Failed to create auth client: %v", err)
	}
	defer authClient.Close()

	rabbitMQConsumer, err := rabbitmq.NewConsumer(cfg.RabbitMQURL, storeInstance, kubernetesProvisioner, authClient)
	if err != nil {
		log.Fatalf("Failed to create RabbitMQ consumer: %v", err)
	}
	defer rabbitMQConsumer.Close()

	go func() {
		if err := rabbitMQConsumer.Start(ctx); err != nil {
			log.Printf("RabbitMQ consumer error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	cancel()
}
