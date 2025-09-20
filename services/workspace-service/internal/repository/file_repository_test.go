package repository

import (
	"testing"

	"github.com/Aadithya-J/code_nest/services/workspace-service/internal/models"
	"github.com/stretchr/testify/require"
)

func TestFileRepository_CRUD(t *testing.T) {
	db := setupTestDB(t)
	projectRepo := NewProjectRepository(db)
	fileRepo := NewFileRepository(db)

	proj := &models.Project{UserID: "user1", Name: "file-test-proj"}
	require.NoError(t, projectRepo.CreateProject(proj))

	file := &models.File{
		ProjectID: proj.ID,
		Path:      "main.go",
		Content:   "package main\n\nfunc main() {}\n",
	}
	require.NoError(t, fileRepo.SaveFile(file))
	require.NotEmpty(t, file.ID)

	retrieved, err := fileRepo.GetFile(proj.ID, "main.go")
	require.NoError(t, err)
	require.Equal(t, file.Content, retrieved.Content)

	files, err := fileRepo.ListFiles(proj.ID)
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.Equal(t, "main.go", files[0].Path)
}
