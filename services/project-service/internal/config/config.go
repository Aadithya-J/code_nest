package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	GRPCPort   string
	DBURL      string
	RedisAddr  string
	AuthRPCURL string
	GatewayURL string
	AtlasBase  string
}

func Load() Config {
	_ = godotenv.Load()
	return Config{
		GRPCPort:   getEnv("PROJECT_GRPC_PORT", "50052"),
		DBURL:      getEnv("PROJECT_DB_URL", "postgres://auth_user:auth_pass@postgres:5432/auth_db?sslmode=disable"),
		RedisAddr:  getEnv("REDIS_ADDR", "redis:6379"),
		AuthRPCURL: getEnv("AUTH_RPC_URL", "auth-service:50051"),
		GatewayURL: getEnv("GATEWAY_URL", "http://localhost:3000"),
		AtlasBase:  getEnv("ATLAS_BASE_URL", "http://localhost:8080"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	if fallback == "" {
		log.Fatalf("%s environment variable is required", key)
	}
	return fallback
}
