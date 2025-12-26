package db

import (
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Project struct {
	ID            string `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Name          string `gorm:"not null;size:100"`
	UserID        string `gorm:"type:uuid;not null;index"`
	RepoURL       string `gorm:"not null"`
	AtlasID       string `gorm:"uniqueIndex;not null"`
	Status        string `gorm:"not null;default:'STOPPED'"`
	WebhookSecret string `gorm:"type:text"`
	UpdatedAt     time.Time
	CreatedAt     time.Time
}

func Connect(dsn string) (*gorm.DB, error) {
	return gorm.Open(postgres.Open(dsn), &gorm.Config{})
}

func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&Project{})
}

func DSN(host string, port int, user, pass, dbname string) string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", host, port, user, pass, dbname)
}
