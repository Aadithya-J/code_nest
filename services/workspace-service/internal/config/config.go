package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL        string
	DBSchema           string
	ServicePort        string
	RabbitMQURL        string
	KubeconfigPath     string
	WorkspaceNamespace string
}

func LoadConfig() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatalf("DATABASE_URL env var required")
	}

	port := os.Getenv("WORKSPACE_SERVICE_PORT")
	if port == "" {
		port = "50052"
		log.Printf("WORKSPACE_SERVICE_PORT not set, using default %s", port)
	}

	rabbitMQURL := os.Getenv("RABBITMQ_URL")
	if rabbitMQURL == "" {
		log.Fatalf("RABBITMQ_URL env var required")
	}

	schema := os.Getenv("DB_SCHEMA")
	if schema == "" {
		schema = "workspace"
	}

	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		kubeconfigPath = os.Getenv("HOME") + "/.kube/config"
	}

	workspaceNamespace := os.Getenv("WORKSPACE_NAMESPACE")
	if workspaceNamespace == "" {
		workspaceNamespace = "workspaces"
	}

	return &Config{
		DatabaseURL:        dbURL,
		DBSchema:           schema,
		ServicePort:        port,
		RabbitMQURL:        rabbitMQURL,
		KubeconfigPath:     kubeconfigPath,
		WorkspaceNamespace: workspaceNamespace,
	}
}
