package db

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"

	"github.com/Aadithya-J/code_nest/services/auth-service/internal/repository"
)

func Migrations() []*gormigrate.Migration {
	return []*gormigrate.Migration{
		{
			ID: "20250827_create_users_table",
			Migrate: func(tx *gorm.DB) error {
				return tx.AutoMigrate(&repository.User{})
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.Migrator().DropTable("users")
			},
		},
	}
}
