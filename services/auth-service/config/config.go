package config

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Server struct {
		Port string
	}
	JWKS struct {
		Port string
	}
	DB struct {
		ConnString string
		Schema     string
	}
	JWT struct {
		Secret string
	}
	GitHub struct {
		AppID          int64
		AppSlug        string
		PrivateKeyPath string
		PrivateKeyPEM  string
	}
	Google struct {
		ClientID     string
		ClientSecret string
		RedirectURL  string
	}
}

func LoadConfig() Config {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	cfg := Config{}

	port := os.Getenv("AUTH_SERVICE_PORT")
	if port == "" {
		port = "50051" // Default gRPC port
		log.Printf("AUTH_SERVICE_PORT not set, using default %s", port)
	}
	cfg.Server.Port = port

	jwksPort := os.Getenv("JWKS_PORT")
	if jwksPort == "" {
		jwksPort = "8081" // Default JWKS port
		log.Printf("JWKS_PORT not set, using default %s", jwksPort)
	}
	cfg.JWKS.Port = jwksPort

	conn := os.Getenv("DATABASE_URL")
	if conn == "" {
		log.Fatalf("DATABASE_URL env var required")
	}
	cfg.DB.ConnString = conn

	schema := os.Getenv("DB_SCHEMA")
	if schema == "" {
		schema = "auth"
	}
	cfg.DB.Schema = schema

	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		log.Fatalf("JWT_SECRET env var required")
	}
	cfg.JWT.Secret = secret

	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	if clientID == "" {
		log.Fatalf("GOOGLE_CLIENT_ID env var required")
	}
	cfg.Google.ClientID = clientID

	clientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	if clientSecret == "" {
		log.Fatalf("GOOGLE_CLIENT_SECRET env var required")
	}
	cfg.Google.ClientSecret = clientSecret

	redirect := os.Getenv("GOOGLE_REDIRECT_URL")
	if redirect == "" {
		log.Fatalf("GOOGLE_REDIRECT_URL env var required")
	}
	cfg.Google.RedirectURL = redirect

	// GitHub App
	appIDStr := os.Getenv("GITHUB_APP_ID")
	if appIDStr == "" {
		log.Fatalf("GITHUB_APP_ID env var required")
	}
	var appID int64
	fmt.Sscan(appIDStr, &appID)
	cfg.GitHub.AppID = appID
	cfg.GitHub.AppSlug = os.Getenv("GITHUB_APP_SLUG")
	if cfg.GitHub.AppSlug == "" {
		log.Fatalf("GITHUB_APP_SLUG env var required")
	}
	cfg.GitHub.PrivateKeyPEM = os.Getenv("GITHUB_APP_PRIVATE_KEY")
	pkPath := os.Getenv("GITHUB_PRIVATE_KEY_PATH")
	if pkPath == "" {
		pkPath = "/app/github_app.pem"
	}
	cfg.GitHub.PrivateKeyPath = pkPath

	return cfg
}
