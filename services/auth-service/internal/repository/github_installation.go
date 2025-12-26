package repository

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GitHubInstallation struct {
	ID                  int64  `gorm:"primaryKey;autoIncrement"`
	InstallationID      int64  `gorm:"column:installation_id;uniqueIndex;not null"`
	UserID              string `gorm:"column:user_id;type:uuid;not null"`
	AccountName         string `gorm:"column:account_name"`
	AccountType         string `gorm:"column:account_type"`         // User or Organization
	RepositorySelection string `gorm:"column:repository_selection"` // all or selected
	AccessToken         string `gorm:"column:access_token"`
	TokenExpiry         int64  `gorm:"column:token_expiry"` // Unix timestamp
	CreatedAt           int64  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt           int64  `gorm:"column:updated_at;autoUpdateTime"`
}

type GitHubInstallationRepo struct {
	db *gorm.DB
}

func NewGitHubInstallationRepo(db *gorm.DB) *GitHubInstallationRepo {
	return &GitHubInstallationRepo{db: db}
}

func (r *GitHubInstallationRepo) Create(installation *GitHubInstallation) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "installation_id"}},
		DoNothing: true,
	}).Create(installation).Error
}

func (r *GitHubInstallationRepo) FindByUserID(userID string) ([]*GitHubInstallation, error) {
	var installations []*GitHubInstallation
	if err := r.db.Where("user_id = ?", userID).Find(&installations).Error; err != nil {
		return nil, err
	}
	return installations, nil
}

func (r *GitHubInstallationRepo) FindByInstallationID(installationID int64) (*GitHubInstallation, error) {
	var installation GitHubInstallation
	if err := r.db.Where("installation_id = ?", installationID).First(&installation).Error; err != nil {
		return nil, err
	}
	return &installation, nil
}

func (r *GitHubInstallationRepo) UpdateAccessToken(installationID int64, token string, expiry int64) error {
	return r.db.Model(&GitHubInstallation{}).Where("installation_id = ?", installationID).Updates(map[string]interface{}{
		"access_token": token,
		"token_expiry": expiry,
	}).Error
}

func (r *GitHubInstallationRepo) Delete(installationID int64) error {
	return r.db.Where("installation_id = ?", installationID).Delete(&GitHubInstallation{}).Error
}
