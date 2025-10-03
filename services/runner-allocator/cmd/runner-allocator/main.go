package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Aadithya-J/code_nest/services/runner-allocator/internal/config"
	"github.com/Aadithya-J/code_nest/services/runner-allocator/internal/consumer"
	"github.com/Aadithya-J/code_nest/services/runner-allocator/internal/lifecycle"
	"github.com/Aadithya-J/code_nest/services/runner-allocator/internal/provisioner"
	"github.com/Aadithya-J/code_nest/services/runner-allocator/internal/store"
)

func main() {
	cfg := config.LoadConfig()

	storeInstance := store.NewInMemoryStore(cfg.MaxSlots, cfg.QueueSize)

	kubernetesProvisioner, err := provisioner.NewKubernetesProvisioner("workspaces", cfg.WorkspaceImage)
	if err != nil {
		log.Fatalf("Failed to create Kubernetes provisioner: %v", err)
	}

	if err := kubernetesProvisioner.InitializeSlots(context.Background()); err != nil {
		log.Fatalf("Failed to initialize slots: %v", err)
	}

	lifecycleManager := lifecycle.NewManager(storeInstance, kubernetesProvisioner)
	ctx, cancel := context.WithCancel(context.Background())
	go lifecycleManager.Start(ctx)

	kafkaConsumer := consumer.NewKafkaConsumer(cfg.KafkaBrokerURL, cfg.KafkaTopicRequests, storeInstance, kubernetesProvisioner)

	go func() {
		if err := kafkaConsumer.Start(ctx); err != nil {
			log.Printf("Kafka consumer error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	cancel()
}
