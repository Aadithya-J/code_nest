package main

import (
	"log"
	"net"
	"os"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/Aadithya-J/code_nest/proto"
	"github.com/Aadithya-J/code_nest/services/project-service/internal/config"
	"github.com/Aadithya-J/code_nest/services/project-service/internal/db"
	"github.com/Aadithya-J/code_nest/services/project-service/internal/service"
)

func main() {
	cfg := config.Load()

	gdb, err := db.Connect(cfg.DBURL)
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}

	m := gormigrate.New(gdb, gormigrate.DefaultOptions, db.Migrations())
	if err := m.Migrate(); err != nil {
		log.Fatalf("migration error: %v", err)
	}

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})

	// Check if TLS is enabled
	tlsEnabled := os.Getenv("GRPC_TLS_ENABLED") == "true"

	var dialOpts []grpc.DialOption
	if tlsEnabled {
		creds, err := credentials.NewClientTLSFromFile(os.Getenv("GRPC_TLS_CERT_FILE"), "")
		if err != nil {
			log.Fatalf("failed to load TLS credentials: %v", err)
		}
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(creds))
	} else {
		// For local development, use plaintext gRPC
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	authConn, err := grpc.Dial(cfg.AuthRPCURL, dialOpts...)
	if err != nil {
		log.Fatalf("dial auth: %v", err)
	}
	authClient := proto.NewAuthServiceClient(authConn)

	svc := service.New(gdb, rdb, authClient, cfg.AtlasBase, cfg.GatewayURL)

	lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	proto.RegisterProjectServiceServer(grpcServer, svc)
	log.Printf("project-service listening on :%s", cfg.GRPCPort)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
