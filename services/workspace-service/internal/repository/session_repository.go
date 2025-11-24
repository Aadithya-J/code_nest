package repository

import (
	"context"
	"time"

	"github.com/Aadithya-J/code_nest/services/workspace-service/internal/models"
	"gorm.io/gorm"
)

type SessionRepository struct {
	db *gorm.DB
}

func NewSessionRepository(db *gorm.DB) *SessionRepository {
	return &SessionRepository{db: db}
}

func (r *SessionRepository) Create(ctx context.Context, session *models.WorkspaceSession) error {
	return r.db.WithContext(ctx).Create(session).Error
}

func (r *SessionRepository) UpdateStatus(ctx context.Context, sessionID, status, statusMessage string) error {
	updates := map[string]interface{}{
		"status":         status,
		"status_message": statusMessage,
		"updated_at":     time.Now(),
	}
	return r.db.WithContext(ctx).
		Model(&models.WorkspaceSession{}).
		Where("session_id = ?", sessionID).
		Updates(updates).Error
}

func (r *SessionRepository) UpdateSlot(ctx context.Context, sessionID string, slotID *string) error {
	return r.db.WithContext(ctx).
		Model(&models.WorkspaceSession{}).
		Where("session_id = ?", sessionID).
		Updates(map[string]interface{}{
			"slot_id":    slotID,
			"updated_at": time.Now(),
		}).Error
}

func (r *SessionRepository) MarkReleased(ctx context.Context, sessionID string) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&models.WorkspaceSession{}).
		Where("session_id = ?", sessionID).
		Updates(map[string]interface{}{
			"status":      models.SessionStatusReleased,
			"released_at": &now,
			"updated_at":  now,
		}).Error
}

func (r *SessionRepository) GetByID(ctx context.Context, sessionID string) (*models.WorkspaceSession, error) {
	var session models.WorkspaceSession
	if err := r.db.WithContext(ctx).First(&session, "session_id = ?", sessionID).Error; err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *SessionRepository) GetActiveSessionByProjectID(ctx context.Context, projectID string) (*models.WorkspaceSession, error) {
	var session models.WorkspaceSession
	if err := r.db.WithContext(ctx).
		Where("project_id = ?", projectID).
		Where("status IN ?", []string{models.SessionStatusRunning, models.SessionStatusCreating}).
		Order("created_at DESC").
		First(&session).Error; err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *SessionRepository) GetAllActiveSessions(ctx context.Context) ([]models.WorkspaceSession, error) {
	var sessions []models.WorkspaceSession
	if err := r.db.WithContext(ctx).
		Where("status IN ?", []string{models.SessionStatusRunning, models.SessionStatusCreating, models.SessionStatusQueued}).
		Find(&sessions).Error; err != nil {
		return nil, err
	}
	return sessions, nil
}
