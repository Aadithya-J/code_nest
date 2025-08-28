package repository

import (
	"github.com/Aadithya-J/code_nest/services/workspace-service/internal/models"
	"gorm.io/gorm"
)

type ProjectRepository struct {
	DB *gorm.DB
}

func NewProjectRepository(db *gorm.DB) *ProjectRepository {
	return &ProjectRepository{DB: db}
}

func (r *ProjectRepository) CreateProject(project *models.Project) error {
	return r.DB.Create(project).Error
}

func (r *ProjectRepository) GetProjectsByUserID(userID string) ([]models.Project, error) {
	var projects []models.Project
	err := r.DB.Where("user_id = ?", userID).Find(&projects).Error
	return projects, err
}

func (r *ProjectRepository) UpdateProject(projectID string, updates map[string]interface{}) error {
	return r.DB.Model(&models.Project{}).Where("id = ?", projectID).Updates(updates).Error
}

func (r *ProjectRepository) DeleteProject(projectID string, userID string) error {
	return r.DB.Where("id = ? AND user_id = ?", projectID, userID).Delete(&models.Project{}).Error
}

func (r *ProjectRepository) GetProjectByID(projectID string) (*models.Project, error) {
	var project models.Project
	err := r.DB.Where("id = ?", projectID).First(&project).Error
	if err != nil {
		return nil, err
	}
	return &project, nil
}
