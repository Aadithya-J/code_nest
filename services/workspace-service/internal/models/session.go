package models

import "time"

const (
	SessionStatusCreating  = "CREATING"
	SessionStatusRunning   = "RUNNING"
	SessionStatusQueued    = "QUEUED"
	SessionStatusReleasing = "RELEASING"
	SessionStatusReleased  = "RELEASED"
	SessionStatusFailed    = "FAILED"
)

// WorkspaceSession captures lifecycle information for an allocated workspace request.
type WorkspaceSession struct {
	SessionID      string     `gorm:"primaryKey;column:session_id"`
	ProjectID      string     `gorm:"index;column:project_id"`
	UserID         string     `gorm:"index;column:user_id"`
	SlotID         *string    `gorm:"column:slot_id"`
	GitRepoURL     string     `gorm:"column:git_repo_url"`
	Status         string     `gorm:"column:status"`
	StatusMessage  string     `gorm:"column:status_message"`
	LastActivityAt *time.Time `gorm:"column:last_activity_at"`
	ReleasedAt     *time.Time `gorm:"column:released_at"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
	UpdatedAt      time.Time  `gorm:"column:updated_at"`
}

func (WorkspaceSession) TableName() string {
	return "workspace_sessions"
}
