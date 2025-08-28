package main

import (
	"log"
	"net"

	"github.com/Aadithya-J/code_nest/proto"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/grpc"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/Aadithya-J/code_nest/services/auth-service/config"
	"github.com/Aadithya-J/code_nest/services/auth-service/internal/repository"
	"github.com/Aadithya-J/code_nest/services/auth-service/internal/service"
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

	lis, err := net.Listen("tcp", ":"+cfg.Server.Port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	proto.RegisterAuthServiceServer(s, authSvc)
	log.Printf("gRPC server listening on :%s", cfg.Server.Port)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve gRPC: %v", err)
	}
}
