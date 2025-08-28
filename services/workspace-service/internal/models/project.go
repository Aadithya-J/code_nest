package models

import "time"

type Project struct {
	ID          string    `gorm:"type:uuid;primary_key;default:gen_random_uuid()"`
	Name        string    `gorm:"type:varchar(255);not null"`
	Description string    `gorm:"type:text"`
	UserID      string    `gorm:"type:varchar(255);not null"`
	CreatedAt   time.Time `gorm:"autoCreateTime"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime"`
}
