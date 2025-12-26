package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port         string
	AuthRPCURL   string
	AllowOrigins string
	RedisAddr    string
	ProjectRPC   string
}

func Load() Config {
	_ = godotenv.Load()

	cfg := Config{
		Port:         getEnv("PORT", "3000"),
		AuthRPCURL:   getEnv("AUTH_RPC_URL", "auth-service:50051"),
		AllowOrigins: getEnv("ALLOWED_ORIGINS", "http://localhost:5173"),
		RedisAddr:    getEnv("REDIS_ADDR", "redis:6379"),
		ProjectRPC:   getEnv("PROJECT_RPC_URL", "project-service:50052"),
	}
	return cfg
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	if fallback == "" {
		log.Fatalf("%s environment variable is required", key)
	}
	return fallback
}
