package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/aadithya/code_nest/services/auth-service/internal/repository"
	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
)

type AuthService struct {
	repo      *repository.UserRepo
	jwtSecret string
	oauthConf *oauth2.Config
}

// VerifyToken parses and validates a JWT, returning the subject (email) if valid
func (s *AuthService) VerifyToken(tokenStr string) (string, error) {
	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.jwtSecret), nil
	})
	if err != nil {
		return "", err
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return "", errors.New("invalid token")
	}
	sub, ok := claims["sub"].(string)
	if !ok {
		return "", errors.New("invalid sub claim")
	}
	return sub, nil
}

func NewAuthService(repo *repository.UserRepo, jwtSecret string, oauthConf *oauth2.Config) *AuthService {
	return &AuthService{repo: repo, jwtSecret: jwtSecret, oauthConf: oauthConf}
}

func (s *AuthService) Signup(email, password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	user := &repository.User{Email: email, PasswordHash: string(hash)}
	if err := s.repo.Create(user); err != nil {
		return "", err
	}
	return s.GenerateToken(email)
}

func (s *AuthService) Login(email, password string) (string, error) {
	user, err := s.repo.FindByEmail(email)
	if err != nil {
		return "", err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", errors.New("invalid credentials")
	}
	return s.GenerateToken(email)
}

func (s *AuthService) GenerateToken(email string) (string, error) {
	claims := jwt.MapClaims{
		"sub": email,
		"exp": time.Now().Add(time.Hour * 72).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.jwtSecret))
}

func (s *AuthService) HandleGoogleCallback(ctx context.Context, code string) (string, error) {
	token, err := s.oauthConf.Exchange(ctx, code)
	if err != nil {
		return "", err
	}
	client := s.oauthConf.Client(ctx, token)

	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var info struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", err
	}
	email := info.Email
	user, err := s.repo.FindByEmail(email)
	if err != nil {
		user = &repository.User{Email: email}
		if err := s.repo.Create(user); err != nil {
			return "", err
		}
	}
	return s.GenerateToken(email)
}

func (s *AuthService) GetGoogleAuthURL(state string) string {
	return s.oauthConf.AuthCodeURL(state)
}
