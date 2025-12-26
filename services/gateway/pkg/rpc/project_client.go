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

type ProjectClient struct {
	conn   *grpc.ClientConn
	Client proto.ProjectServiceClient
}

func NewProjectClient(target string) (*ProjectClient, error) {
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
		return nil, fmt.Errorf("connect project-service: %w", err)
	}

	return &ProjectClient{
		conn:   conn,
		Client: proto.NewProjectServiceClient(conn),
	}, nil
}

func (c *ProjectClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *ProjectClient) CreateProject(ctx context.Context, req *proto.CreateProjectRequest) (*proto.CreateProjectResponse, error) {
	return c.Client.CreateProject(ctx, req)
}

func (c *ProjectClient) StartWorkspace(ctx context.Context, req *proto.StartWorkspaceRequest) (*proto.StartWorkspaceResponse, error) {
	return c.Client.StartWorkspace(ctx, req)
}

func (c *ProjectClient) StopWorkspace(ctx context.Context, req *proto.StopWorkspaceRequest) (*proto.StopWorkspaceResponse, error) {
	return c.Client.StopWorkspace(ctx, req)
}

func (c *ProjectClient) IsOwner(ctx context.Context, req *proto.IsOwnerRequest) (*proto.IsOwnerResponse, error) {
	return c.Client.IsOwner(ctx, req)
}

func (c *ProjectClient) VerifyAndComplete(ctx context.Context, req *proto.VerifyAndCompleteRequest) (*proto.VerifyAndCompleteResponse, error) {
	return c.Client.VerifyAndComplete(ctx, req)
}

func (c *ProjectClient) WebhookUpdate(ctx context.Context, req *proto.WebhookUpdateRequest) (*proto.WebhookUpdateResponse, error) {
	return c.Client.WebhookUpdate(ctx, req)
}
