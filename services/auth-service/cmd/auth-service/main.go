package main

import (
	"log"

	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/aadithya/code_nest/services/auth-service/config"
	"github.com/aadithya/code_nest/services/auth-service/internal/api"
	"github.com/aadithya/code_nest/services/auth-service/internal/repository"
	"github.com/aadithya/code_nest/services/auth-service/internal/service"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	db, err := gorm.Open(postgres.Open(cfg.DB.ConnString), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	_ = db.AutoMigrate(&repository.User{})
	repo := repository.NewUserRepo(db)

	oauthConf := &oauth2.Config{
		ClientID:     cfg.Google.ClientID,
		ClientSecret: cfg.Google.ClientSecret,
		RedirectURL:  cfg.Google.RedirectURL,
		Scopes:       []string{"openid", "email", "profile"},
		Endpoint:     google.Endpoint,
	}
	authSvc := service.NewAuthService(repo, cfg.JWT.Secret, oauthConf)
	handler := api.NewHandler(authSvc)

	r := gin.Default()

	auth := r.Group("/auth")
	{
		auth.POST("/signup", handler.Signup)
		auth.POST("/login", handler.Login)
		auth.GET("/health", handler.Health)
		auth.GET("/google/login", handler.GoogleLogin)
		auth.GET("/google/callback", handler.GoogleCallback)
	}

	addr := ":" + cfg.Server.Port
	log.Printf("Starting auth-service on %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatal(err)
	}
}
