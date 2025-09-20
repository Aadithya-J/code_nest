package repository

import (
	"github.com/Aadithya-J/code_nest/services/workspace-service/internal/models"
	"gorm.io/gorm"
)

type FileRepository struct {
	DB *gorm.DB
}

func NewFileRepository(db *gorm.DB) *FileRepository {
	return &FileRepository{DB: db}
}

func (r *FileRepository) SaveFile(file *models.File) error {
	return r.DB.Save(file).Error
}

func (r *FileRepository) GetFile(projectID, path string) (*models.File, error) {
	var file models.File
	err := r.DB.Where("project_id = ? AND path = ?", projectID, path).First(&file).Error
	return &file, err
}

func (r *FileRepository) ListFiles(projectID string) ([]models.File, error) {
	var files []models.File
	err := r.DB.Where("project_id = ?", projectID).Find(&files).Error
	return files, err
}
