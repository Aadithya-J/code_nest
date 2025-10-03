package config

import (
	"os"
)

type Config struct {
	KafkaBrokerURL     string
	KafkaTopicRequests string
	KafkaTopicStatus   string
	WorkspaceImage     string
	MaxSlots           int
	QueueSize          int
}

func LoadConfig() *Config {
	return &Config{
		KafkaBrokerURL:     getEnv("KAFKA_BROKER_URL", "redpanda:29092"),
		KafkaTopicRequests: getEnv("KAFKA_TOPIC_REQUESTS", "workspace.requests"),
		KafkaTopicStatus:   getEnv("KAFKA_TOPIC_STATUS", "workspace.status"),
		WorkspaceImage:     getEnv("WORKSPACE_IMAGE", "ghcr.io/aadithya-j/code_nest/workspace:latest"),
		MaxSlots:           3,
		QueueSize:          10,
	}
}

// Helper function to get env var with a default value
func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
