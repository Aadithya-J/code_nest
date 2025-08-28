package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL string
	ServicePort string
}

func LoadConfig() *Config {
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found, using environment variables")
	}

	return &Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		ServicePort: os.Getenv("SERVICE_PORT"),
	}
}
