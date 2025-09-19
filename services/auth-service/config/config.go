package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Server struct {
		Port string
	}
	DB struct {
		ConnString string
		Schema     string
	}
	JWT struct {
		Secret string
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
		redirect = "http://localhost:8080/auth/google/callback"
		log.Printf("GOOGLE_REDIRECT_URL not set, using default %s", redirect)
	}
	cfg.Google.RedirectURL = redirect

	return cfg
}
