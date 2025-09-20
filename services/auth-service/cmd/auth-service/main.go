package main

import (
	"fmt"
	"log"
	"net"
	"strings"

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
	cfg := config.LoadConfig()

	dsn := cfg.DB.ConnString
	if !strings.Contains(dsn, "search_path") {
		sep := "?"
		if strings.Contains(dsn, "?") {
			sep = "&"
		}
		dsn = dsn + sep + "search_path=" + cfg.DB.Schema
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	createSchema := fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", cfg.DB.Schema)
	if err := db.Exec(createSchema).Error; err != nil {
		log.Fatalf("failed to create schema: %v", err)
	}
	if err := db.Exec(fmt.Sprintf("SET search_path TO %s", cfg.DB.Schema)).Error; err != nil {
		log.Fatalf("failed to set search_path: %v", err)
	}

	if err := db.AutoMigrate(&repository.User{}); err != nil {
		log.Fatalf("migration error: %v", err)
	}
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
