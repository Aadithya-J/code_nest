package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Server struct {
		Port string
	}
	DB struct {
		ConnString string
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

func LoadConfig() (Config, error) {
	_ = godotenv.Load()
	cfg := Config{}

	port := os.Getenv("AUTH_SERVICE_PORT")
	if port == "" {
		port = "8081"
	}
	cfg.Server.Port = port

	conn := os.Getenv("AUTH_DB_CONN")
	if conn == "" {
		return cfg, fmt.Errorf("AUTH_DB_CONN env required")
	}
	cfg.DB.ConnString = conn

	secret := os.Getenv("AUTH_JWT_SECRET")
	if secret == "" {
		return cfg, fmt.Errorf("AUTH_JWT_SECRET env required")
	}
	cfg.JWT.Secret = secret

	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	if clientID == "" {
		return cfg, fmt.Errorf("GOOGLE_CLIENT_ID env required")
	}
	cfg.Google.ClientID = clientID

	clientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	if clientSecret == "" {
		return cfg, fmt.Errorf("GOOGLE_CLIENT_SECRET env required")
	}
	cfg.Google.ClientSecret = clientSecret

	redirect := os.Getenv("AUTH_GOOGLE_REDIRECT_URL")
	if redirect == "" {
		redirect = fmt.Sprintf("http://localhost:%s/auth/google/callback", cfg.Server.Port)
	}
	cfg.Google.RedirectURL = redirect

	return cfg, nil
}
