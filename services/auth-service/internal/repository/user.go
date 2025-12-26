package repository

import (
	"gorm.io/gorm"
)

type User struct {
	ID           string  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Email        string  `gorm:"uniqueIndex;not null"`
	PasswordHash string  `gorm:"not null"`
	GoogleID     *string `gorm:"column:google_id;uniqueIndex"`
	AvatarURL    string  `gorm:"column:avatar_url"`
}

type UserRepo struct {
	db *gorm.DB
}

func NewUserRepo(db *gorm.DB) *UserRepo {
	return &UserRepo{db: db}
}

func (r *UserRepo) Create(user *User) error {
	return r.db.Create(user).Error
}

func (r *UserRepo) FindByEmail(email string) (*User, error) {
	var user User
	if err := r.db.Where("email = ?", email).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepo) FindByID(id string) (*User, error) {
	var user User
	if err := r.db.Where("id = ?", id).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepo) UpdateGoogleInfo(userID string, googleID, avatarURL string) error {
	return r.db.Model(&User{}).Where("id = ?", userID).Updates(map[string]interface{}{
		"google_id":  googleID,
		"avatar_url": avatarURL,
	}).Error
}

func (r *UserRepo) FindByGoogleID(googleID string) (*User, error) {
	var user User
	if err := r.db.Where("google_id = ?", googleID).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}
