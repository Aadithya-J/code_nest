package rpc

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Aadithya-J/code_nest/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

type AuthClient struct {
	conn   *grpc.ClientConn
	Client proto.AuthServiceClient
}

func NewAuthClient(target string) (*AuthClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Check if TLS is enabled
	tlsEnabled := os.Getenv("GRPC_TLS_ENABLED") == "true"

	var dialOpts []grpc.DialOption
	dialOpts = append(dialOpts, grpc.WithBlock())

	if tlsEnabled {
		creds, err := credentials.NewClientTLSFromFile(os.Getenv("GRPC_TLS_CERT_FILE"), "")
		if err != nil {
			return nil, fmt.Errorf("failed to load TLS credentials: %w", err)
		}
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(creds))
	} else {
		// For local development, use plaintext (no TLS)
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.DialContext(ctx, target, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("connect auth-service: %w", err)
	}

	return &AuthClient{
		conn:   conn,
		Client: proto.NewAuthServiceClient(conn),
	}, nil
}

func (c *AuthClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *AuthClient) Signup(ctx context.Context, req *proto.SignupRequest) (*proto.AuthResponse, error) {
	return c.Client.Signup(ctx, req)
}

func (c *AuthClient) Login(ctx context.Context, req *proto.LoginRequest) (*proto.AuthResponse, error) {
	return c.Client.Login(ctx, req)
}

func (c *AuthClient) GetGoogleAuthURL(ctx context.Context, req *proto.GetGoogleAuthURLRequest) (*proto.GetGoogleAuthURLResponse, error) {
	return c.Client.GetGoogleAuthURL(ctx, req)
}

func (c *AuthClient) HandleGoogleCallback(ctx context.Context, req *proto.HandleGoogleCallbackRequest) (*proto.AuthResponse, error) {
	return c.Client.HandleGoogleCallback(ctx, req)
}

func (c *AuthClient) GetGitHubAuthURL(ctx context.Context, req *proto.GetGitHubAuthURLRequest) (*proto.GetGitHubAuthURLResponse, error) {
	return c.Client.GetGitHubAuthURL(ctx, req)
}

func (c *AuthClient) HandleGitHubCallback(ctx context.Context, req *proto.HandleGitHubCallbackRequest) (*proto.AuthResponse, error) {
	return c.Client.HandleGitHubCallback(ctx, req)
}

func (c *AuthClient) ValidateToken(ctx context.Context, token string) (*proto.ValidateTokenResponse, error) {
	return c.Client.ValidateToken(ctx, &proto.ValidateTokenRequest{Token: token})
}

func (c *AuthClient) GetGitHubAccessToken(ctx context.Context, req *proto.GetGitHubAccessTokenRequest) (*proto.GetGitHubAccessTokenResponse, error) {
	return c.Client.GetGitHubAccessToken(ctx, req)
}

func (c *AuthClient) GenerateRepoToken(ctx context.Context, req *proto.GenerateRepoTokenRequest) (*proto.GenerateRepoTokenResponse, error) {
	return c.Client.GenerateRepoToken(ctx, req)
}
