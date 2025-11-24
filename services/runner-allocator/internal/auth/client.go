package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/Aadithya-J/code_nest/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Client struct {
	conn   *grpc.ClientConn
	client proto.AuthServiceClient
}

func NewClient(authServiceURL string) (*Client, error) {
	conn, err := grpc.Dial(authServiceURL, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to auth service: %w", err)
	}

	return &Client{
		conn:   conn,
		client: proto.NewAuthServiceClient(conn),
	}, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) GetGitHubToken(ctx context.Context, userID string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := c.client.GetGitHubAccessToken(ctx, &proto.GetGitHubAccessTokenRequest{
		UserId: userID,
	})
	if err != nil {
		return "", fmt.Errorf("failed to get github token: %w", err)
	}

	return resp.Token, nil
}
