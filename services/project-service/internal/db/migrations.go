package db

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func Migrations() []*gormigrate.Migration {
	return []*gormigrate.Migration{
		{
			ID: "20250827_create_projects_table",
			Migrate: func(tx *gorm.DB) error {
				return tx.AutoMigrate(&Project{})
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.Migrator().DropTable("projects")
			},
		},
	}
}
