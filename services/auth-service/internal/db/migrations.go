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
		{
			ID: "20251225_make_google_id_nullable",
			Migrate: func(tx *gorm.DB) error {
				// Allow multiple users without Google linkage by making google_id NULLable and removing default "".
				if err := tx.Exec(`UPDATE users SET google_id = NULL WHERE google_id = ''`).Error; err != nil {
					return err
				}
				if err := tx.Exec(`ALTER TABLE users ALTER COLUMN google_id DROP NOT NULL`).Error; err != nil {
					return err
				}
				if err := tx.Exec(`ALTER TABLE users ALTER COLUMN google_id DROP DEFAULT`).Error; err != nil {
					return err
				}
				return nil
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.Exec(`ALTER TABLE users ALTER COLUMN google_id SET DEFAULT ''; ALTER TABLE users ALTER COLUMN google_id SET NOT NULL`).Error
			},
		},
		{
			ID: "20251226_create_github_installations",
			Migrate: func(tx *gorm.DB) error {
				return tx.AutoMigrate(&repository.GitHubInstallation{})
			},
			Rollback: func(tx *gorm.DB) error {
				return tx.Migrator().DropTable("git_hub_installations")
			},
		},
	}
}
