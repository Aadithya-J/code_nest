package models

import "time"

type File struct {
	ID        string    `gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	ProjectID string    `gorm:"type:uuid;not null;index"`
	Path      string    `gorm:"type:text;not null"`
	Content   string    `gorm:"type:text"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}
