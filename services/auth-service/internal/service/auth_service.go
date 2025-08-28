package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Aadithya-J/code_nest/proto"
	"github.com/Aadithya-J/code_nest/services/auth-service/internal/repository"
	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
)

type AuthService struct {
	proto.UnimplementedAuthServiceServer
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

func (s *AuthService) ValidateToken(ctx context.Context, req *proto.ValidateTokenRequest) (*proto.ValidateTokenResponse, error) {
	email, err := s.VerifyToken(req.Token)
	if err != nil {
		return &proto.ValidateTokenResponse{Valid: false, Error: err.Error()}, nil
	}
	return &proto.ValidateTokenResponse{Valid: true, UserId: email}, nil
}

func NewAuthService(repo *repository.UserRepo, jwtSecret string, oauthConf *oauth2.Config) *AuthService {
	return &AuthService{repo: repo, jwtSecret: jwtSecret, oauthConf: oauthConf}
}

func (s *AuthService) Signup(ctx context.Context, req *proto.SignupRequest) (*proto.AuthResponse, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	user := &repository.User{Email: req.Email, PasswordHash: string(hash)}
	if err := s.repo.Create(user); err != nil {
		return nil, err
	}
	token, err := s.GenerateToken(req.Email)
	if err != nil {
		return nil, err
	}
	return &proto.AuthResponse{Token: token}, nil
}

func (s *AuthService) Login(ctx context.Context, req *proto.LoginRequest) (*proto.AuthResponse, error) {
	user, err := s.repo.FindByEmail(req.Email)
	if err != nil {
		return nil, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, errors.New("invalid credentials")
	}
	token, err := s.GenerateToken(req.Email)
	if err != nil {
		return nil, err
	}
	return &proto.AuthResponse{Token: token}, nil
}

func (s *AuthService) GenerateToken(email string) (string, error) {
	claims := jwt.MapClaims{
		"sub": email,
		"exp": time.Now().Add(time.Hour * 72).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.jwtSecret))
}

func (s *AuthService) HandleGoogleCallback(ctx context.Context, req *proto.HandleGoogleCallbackRequest) (*proto.AuthResponse, error) {
	token, err := s.oauthConf.Exchange(ctx, req.Code)
	if err != nil {
		return nil, err
	}
	client := s.oauthConf.Client(ctx, token)

	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var info struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}
	email := info.Email
	user, err := s.repo.FindByEmail(email)
	if err != nil {
		user = &repository.User{Email: email}
		if err := s.repo.Create(user); err != nil {
			return nil, err
		}
	}
	jwt, err := s.GenerateToken(email)
	if err != nil {
		return nil, err
	}
	return &proto.AuthResponse{Token: jwt}, nil
}

func (s *AuthService) GetGoogleAuthURL(ctx context.Context, req *proto.GetGoogleAuthURLRequest) (*proto.GetGoogleAuthURLResponse, error) {
	url := s.oauthConf.AuthCodeURL(req.State)
	return &proto.GetGoogleAuthURLResponse{Url: url}, nil
}
