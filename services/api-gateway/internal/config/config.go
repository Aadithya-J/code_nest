package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	AuthSvcUrl         string
	WorkspaceSvcUrl    string
	Port               string
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string
	JwtSecret          string
}

func LoadConfig() *Config {
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found, using environment variables")
	}

	return &Config{
		AuthSvcUrl:         os.Getenv("AUTH_SERVICE_URL"),
		WorkspaceSvcUrl:    os.Getenv("WORKSPACE_SERVICE_URL"),
		Port:               os.Getenv("PORT"),
		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		GoogleRedirectURL:  os.Getenv("AUTH_GOOGLE_REDIRECT_URL"),
		JwtSecret:          os.Getenv("JWT_SECRET"),
	}
}
