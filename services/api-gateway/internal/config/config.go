package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	AuthSvcUrl      string
	WorkspaceSvcUrl string
	Port            string
	AuthSvcJWKSUrl  string
}

func LoadConfig() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	authSvcUrl := os.Getenv("AUTH_SERVICE_URL")
	if authSvcUrl == "" {
		log.Fatalf("AUTH_SERVICE_URL env var required")
	}

	workspaceSvcUrl := os.Getenv("WORKSPACE_SERVICE_URL")
	if workspaceSvcUrl == "" {
		log.Fatalf("WORKSPACE_SERVICE_URL env var required")
	}

	authSvcJWKSUrl := os.Getenv("AUTH_SERVICE_JWKS_URL")
	if authSvcJWKSUrl == "" {
		log.Fatalf("AUTH_SERVICE_JWKS_URL env var required")
	}

	port := os.Getenv("API_GATEWAY_PORT")
	if port == "" {
		port = "8080" // Default port
		log.Printf("API_GATEWAY_PORT not set, using default %s", port)
	}

	return &Config{
		AuthSvcUrl:      authSvcUrl,
		WorkspaceSvcUrl: workspaceSvcUrl,
		Port:            port,
		AuthSvcJWKSUrl:  authSvcJWKSUrl,
	}
}
