package config

import (
	"os"
)

type Config struct {
	RabbitMQURL    string
	WorkspaceImage string
	KubeconfigPath string
	MaxSlots       int
	QueueSize      int
	AuthServiceURL string
}

func LoadConfig() *Config {
	return &Config{
		RabbitMQURL:    getEnv("RABBITMQ_URL", "amqp://user:password@rabbitmq:5672/"),
		WorkspaceImage: getEnv("WORKSPACE_IMAGE", "ghcr.io/aadithya-j/code_nest/workspace:latest"),
		KubeconfigPath: getEnv("KUBECONFIG_PATH", ""),
		MaxSlots:       3,
		QueueSize:      10,
		AuthServiceURL: getEnv("AUTH_SERVICE_URL", "auth-service:50051"),
	}
}

// Helper function to get env var with a default value
func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
