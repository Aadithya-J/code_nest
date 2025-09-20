package repository

import (
	"context"
	"fmt"
	"testing"

	"github.com/Aadithya-J/code_nest/services/workspace-service/internal/models"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	ctx := context.Background()
	req := testcontainers.ContainerRequest{
		Image:        "postgres:14-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "test",
			"POSTGRES_PASSWORD": "test",
			"POSTGRES_DB":       "test",
		},
		WaitingFor: wait.ForListeningPort("5432/tcp"),
	}
	pgContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)

	host, err := pgContainer.Host(ctx)
	require.NoError(t, err)
	port, err := pgContainer.MappedPort(ctx, "5432")
	require.NoError(t, err)

	dsn := fmt.Sprintf("host=%s port=%s user=test password=test dbname=test sslmode=disable search_path=workspace", host, port.Port())
	db, err := gorm.Open(gormpostgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec("CREATE SCHEMA IF NOT EXISTS workspace").Error)
	require.NoError(t, db.AutoMigrate(&models.Project{}, &models.File{}))

	return db
}

func TestProjectRepository_CRUD(t *testing.T) {
	db := setupTestDB(t)
	repo := NewProjectRepository(db)

	proj := &models.Project{
		Name:        "demo",
		Description: "demo project",
		UserID:      "user1",
	}
	require.NoError(t, repo.CreateProject(proj))
	require.NotEmpty(t, proj.ID)

	list, err := repo.GetProjectsByUserID("user1")
	require.NoError(t, err)
	require.Len(t, list, 1)

	updates := map[string]interface{}{"description": "updated"}
	require.NoError(t, repo.UpdateProject(proj.ID, updates))

	p2, err := repo.GetProjectByID(proj.ID)
	require.NoError(t, err)
	require.Equal(t, "updated", p2.Description)

	require.NoError(t, repo.DeleteProject(proj.ID, "user1"))

	_, err = repo.GetProjectByID(proj.ID)
	require.Error(t, err)
}
