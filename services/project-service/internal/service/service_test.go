package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Aadithya-J/code_nest/proto"
	"github.com/Aadithya-J/code_nest/services/project-service/internal/db"
)

func TestService_CreateProject(t *testing.T) {
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(gormDB)
	require.NoError(t, err)

	// Create a mock Redis client
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	service := New(gormDB, redisClient, &mockAuthClient{}, "http://localhost:8080", "http://localhost:3000")

	req := &proto.CreateProjectRequest{
		UserId:  uuid.New().String(),
		Name:    "Test Project",
		RepoUrl: "https://github.com/test/repo.git",
	}

	resp, err := service.CreateProject(context.Background(), req)
	require.NoError(t, err)
	require.NotEmpty(t, resp.ProjectId)

	var project db.Project
	err = gormDB.First(&project, "id = ?", resp.ProjectId).Error
	require.NoError(t, err)
	require.Equal(t, req.Name, project.Name)
	require.Equal(t, req.RepoUrl, project.RepoURL)
	require.Equal(t, "STOPPED", project.Status)
}

func TestService_IsOwner(t *testing.T) {
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(gormDB)
	require.NoError(t, err)

	// Create a mock Redis client
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	service := New(gormDB, redisClient, &mockAuthClient{}, "http://localhost:8080", "http://localhost:3000")

	userID := uuid.New().String()
	project := db.Project{
		ID:      uuid.New().String(),
		Name:    "Test Project",
		UserID:  userID,
		RepoURL: "https://github.com/test/repo.git",
		Status:  "STOPPED",
		AtlasID: "ws-" + uuid.New().String(),
	}
	err = gormDB.Create(&project).Error
	require.NoError(t, err)

	req := &proto.IsOwnerRequest{
		UserId:    userID,
		ProjectId: project.AtlasID,
	}

	resp, err := service.IsOwner(context.Background(), req)
	require.NoError(t, err)
	require.True(t, resp.IsOwner)

	req = &proto.IsOwnerRequest{
		UserId:    uuid.New().String(),
		ProjectId: project.AtlasID,
	}

	resp, err = service.IsOwner(context.Background(), req)
	require.NoError(t, err)
	require.False(t, resp.IsOwner)
}

// mockAuthClient implements AuthClient for testing
type mockAuthClient struct{}

func (m *mockAuthClient) GenerateRepoToken(ctx context.Context, in *proto.GenerateRepoTokenRequest, opts ...grpc.CallOption) (*proto.GenerateRepoTokenResponse, error) {
	return &proto.GenerateRepoTokenResponse{Token: "mock-token"}, nil
}
