package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Aadithya-J/code_nest/proto"
	"github.com/Aadithya-J/code_nest/services/auth-service/internal/repository"
	"github.com/golang-jwt/jwt/v4"
	"gopkg.in/square/go-jose.v2"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
)

type AuthService struct {
	proto.UnimplementedAuthServiceServer
	repo         *repository.UserRepo
	privateKey   *rsa.PrivateKey
	jwks         *jose.JSONWebKeySet
	oauthConf    *oauth2.Config
}


func NewAuthService(repo *repository.UserRepo, oauthConf *oauth2.Config) (*AuthService, error) {
	privateKey, err := loadOrGenerateRSAKey()
	if err != nil {
		return nil, fmt.Errorf("failed to load or generate rsa key: %w", err)
	}

	jwk := jose.JSONWebKey{Key: &privateKey.PublicKey, KeyID: "1", Algorithm: "RS256", Use: "sig"}
	jwks := &jose.JSONWebKeySet{Keys: []jose.JSONWebKey{jwk}}

	return &AuthService{repo: repo, privateKey: privateKey, jwks: jwks, oauthConf: oauthConf}, nil
}

func loadOrGenerateRSAKey() (*rsa.PrivateKey, error) {
	keyPath := "/app/keys/rsa_private.pem"
	
	// Try to load existing key
	if keyData, err := os.ReadFile(keyPath); err == nil {
		block, _ := pem.Decode(keyData)
		if block != nil && block.Type == "RSA PRIVATE KEY" {
			privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
			if err == nil {
				log.Println("Loaded existing RSA key from", keyPath)
				return privateKey, nil
			}
		}
	}
	
	// Generate new key if loading failed
	log.Println("Generating new RSA key and saving to", keyPath)
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	
	// Save the key for future use
	if err := saveRSAKey(privateKey, keyPath); err != nil {
		log.Printf("Warning: failed to save RSA key: %v", err)
		// Continue anyway - key will work for this session
	}
	
	return privateKey, nil
}

func saveRSAKey(privateKey *rsa.PrivateKey, keyPath string) error {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(keyPath), 0755); err != nil {
		return err
	}
	
	// Encode private key to PEM format
	keyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: keyBytes,
	})
	
	// Write to file
	return os.WriteFile(keyPath, keyPEM, 0600)
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
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	
	// Set the Key ID in the header to match our JWKS
	token.Header["kid"] = "1"
	
	return token.SignedString(s.privateKey)
}

func (s *AuthService) JWKSHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.jwks)
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
