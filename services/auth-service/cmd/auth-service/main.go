package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/go-gormigrate/gormigrate/v2"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/grpc"

	"github.com/Aadithya-J/code_nest/proto"
	"github.com/Aadithya-J/code_nest/services/auth-service/config"
	"github.com/Aadithya-J/code_nest/services/auth-service/internal/db"
	"github.com/Aadithya-J/code_nest/services/auth-service/internal/repository"
	"github.com/Aadithya-J/code_nest/services/auth-service/internal/service"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
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

	gormDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	createSchema := fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", cfg.DB.Schema)
	if err := gormDB.Exec(createSchema).Error; err != nil {
		log.Fatalf("failed to create schema: %v", err)
	}
	if err := gormDB.Exec(fmt.Sprintf("SET search_path TO %s", cfg.DB.Schema)).Error; err != nil {
		log.Fatalf("failed to set search_path: %v", err)
	}

	m := gormigrate.New(gormDB, gormigrate.DefaultOptions, db.Migrations())
	if err := m.Migrate(); err != nil {
		log.Fatalf("migration error: %v", err)
	}
	repo := repository.NewUserRepo(gormDB)
	githubRepo := repository.NewGitHubInstallationRepo(gormDB)

	oauthConf := &oauth2.Config{
		ClientID:     cfg.Google.ClientID,
		ClientSecret: cfg.Google.ClientSecret,
		RedirectURL:  cfg.Google.RedirectURL,
		Scopes:       []string{"openid", "email", "profile"},
		Endpoint:     google.Endpoint,
	}
	authSvc, err := service.NewAuthService(repo, githubRepo, oauthConf, cfg, nil)
	if err != nil {
		log.Fatalf("failed to create auth service: %v", err)
	}

	lis, err := net.Listen("tcp", ":"+cfg.Server.Port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	proto.RegisterAuthServiceServer(s, authSvc)
	log.Printf("gRPC server listening on :%s", cfg.Server.Port)
	go func() {
		log.Printf("Starting JWKS server on :%s", cfg.JWKS.Port)
		http.HandleFunc("/.well-known/jwks.json", authSvc.JWKSHandler)
		if err := http.ListenAndServe(":"+cfg.JWKS.Port, nil); err != nil {
			log.Fatalf("failed to serve JWKS: %v", err)
		}
	}()

	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve gRPC: %v", err)
	}
}
