package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL    string
	ServicePort    string
	KafkaBrokerURL string
	KafkaTopicCmd  string
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

	kafkaBroker := os.Getenv("KAFKA_BROKER_URL")
	if kafkaBroker == "" {
		log.Fatalf("KAFKA_BROKER_URL env var required")
	}

	kafkaTopic := os.Getenv("KAFKA_TOPIC_CMD")
	if kafkaTopic == "" {
		kafkaTopic = "workspace.cmd"
		log.Printf("KAFKA_TOPIC_CMD not set, using default %s", kafkaTopic)
	}

	return &Config{
		DatabaseURL:    dbURL,
		ServicePort:    port,
		KafkaBrokerURL: kafkaBroker,
		KafkaTopicCmd:  kafkaTopic,
	}
}
