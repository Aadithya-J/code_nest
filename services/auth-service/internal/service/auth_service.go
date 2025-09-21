package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/Aadithya-J/code_nest/proto"
	"github.com/Aadithya-J/code_nest/services/auth-service/config"
	"github.com/Aadithya-J/code_nest/services/auth-service/internal/repository"
	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
	"gopkg.in/square/go-jose.v2"
)

type AuthService struct {
	proto.UnimplementedAuthServiceServer
	repo             *repository.UserRepo
	privateKey       *rsa.PrivateKey
	jwks             *jose.JSONWebKeySet
	oauthConf        *oauth2.Config
	githubAppID      int64
	githubAppSlug    string
	githubPrivateKey []byte
}

func NewAuthService(repo *repository.UserRepo, oauthConf *oauth2.Config, cfg config.Config) (*AuthService, error) {
	privateKey, err := loadOrGenerateRSAKey()
	if err != nil {
		return nil, fmt.Errorf("failed to load or generate rsa key: %w", err)
	}

	jwk := jose.JSONWebKey{Key: &privateKey.PublicKey, KeyID: "1", Algorithm: "RS256", Use: "sig"}
	jwks := &jose.JSONWebKeySet{Keys: []jose.JSONWebKey{jwk}}

	pkBytes, _ := os.ReadFile(cfg.GitHub.PrivateKeyPath)
	return &AuthService{
		repo:             repo,
		privateKey:       privateKey,
		jwks:             jwks,
		oauthConf:        oauthConf,
		githubAppID:      cfg.GitHub.AppID,
		githubAppSlug:    cfg.GitHub.AppSlug,
		githubPrivateKey: pkBytes,
	}, nil
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

// GitHub OAuth methods
func (s *AuthService) GetGitHubAuthURL(ctx context.Context, req *proto.GetGitHubAuthURLRequest) (*proto.GetGitHubAuthURLResponse, error) {
	baseURL := os.Getenv("APP_BASE_URL")
	if baseURL == "" {
		return nil, fmt.Errorf("APP_BASE_URL environment variable is required")
	}
	
	callbackURL := fmt.Sprintf("%s/auth/github/callback", baseURL)
	installURL := fmt.Sprintf("https://github.com/apps/%s/installations/new?redirect_url=%s",
		s.githubAppSlug, callbackURL)

	return &proto.GetGitHubAuthURLResponse{Url: installURL}, nil
}

func (s *AuthService) HandleGitHubCallback(ctx context.Context, req *proto.HandleGitHubCallbackRequest) (*proto.AuthResponse, error) {
	// Get user from the authenticated request
	if req.UserId == "" {
		return &proto.AuthResponse{Error: "User ID required"}, nil
	}
	
	user, err := s.repo.FindByEmail(req.UserId)
	if err != nil {
		return &proto.AuthResponse{Error: "User not found"}, nil
	}

	// Get GitHub username from installation
	username, err := s.getGitHubUsername(req.InstallationId)
	if err != nil {
		return &proto.AuthResponse{Error: "Failed to get GitHub username"}, nil
	}

	// Update user with GitHub info
	err = s.repo.UpdateGitHubInfo(user.ID, req.InstallationId, username)
	if err != nil {
		return &proto.AuthResponse{Error: "Failed to update GitHub info"}, nil
	}

	// Generate new JWT with GitHub linked
	token, err := s.GenerateToken(user.Email)
	if err != nil {
		return &proto.AuthResponse{Error: "Failed to generate token"}, nil
	}

	return &proto.AuthResponse{Token: token}, nil
}

func (s *AuthService) GetGitHubAccessToken(ctx context.Context, req *proto.GetGitHubAccessTokenRequest) (*proto.GetGitHubAccessTokenResponse, error) {
	user, err := s.repo.FindByEmail(req.UserId) // Assuming UserId is email for now
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	if user.GitHubInstallationID == 0 {
		return nil, fmt.Errorf("user has not linked GitHub")
	}

	// Generate GitHub App JWT
	appJWT, err := s.generateGitHubAppJWT()
	if err != nil {
		return nil, fmt.Errorf("failed to generate app JWT: %w", err)
	}

	// Get installation access token
	token, expiresAt, err := s.getInstallationAccessToken(user.GitHubInstallationID, appJWT)
	if err != nil {
		return nil, fmt.Errorf("failed to get installation token: %w", err)
	}

	return &proto.GetGitHubAccessTokenResponse{
		Token:     token,
		ExpiresAt: expiresAt,
	}, nil
}

// Helper methods for GitHub integration
func (s *AuthService) generateGitHubAppJWT() (string, error) {
	block, _ := pem.Decode(s.githubPrivateKey)
	if block == nil {
		return "", fmt.Errorf("failed to parse PEM block")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("failed to parse private key: %w", err)
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"iat": now.Unix(),
		"exp": now.Add(10 * time.Minute).Unix(), // GitHub requires max 10 minutes
		"iss": strconv.FormatInt(s.githubAppID, 10),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(privateKey)
}

func (s *AuthService) getInstallationAccessToken(installationID int64, appJWT string) (string, int64, error) {
	url := fmt.Sprintf("https://api.github.com/app/installations/%d/access_tokens", installationID)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer([]byte("{}")))
	if err != nil {
		return "", 0, err
	}

	req.Header.Set("Authorization", "Bearer "+appJWT)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", 0, fmt.Errorf("GitHub API error: %d %s", resp.StatusCode, string(body))
	}

	var result struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", 0, err
	}

	return result.Token, result.ExpiresAt.Unix(), nil
}

func (s *AuthService) getGitHubUsername(installationID int64) (string, error) {
	// Generate app JWT
	appJWT, err := s.generateGitHubAppJWT()
	if err != nil {
		return "", err
	}

	// Get installation info
	url := fmt.Sprintf("https://api.github.com/app/installations/%d", installationID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+appJWT)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitHub API error: %d %s", resp.StatusCode, string(body))
	}

	var installation struct {
		Account struct {
			Login string `json:"login"`
		} `json:"account"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&installation); err != nil {
		return "", err
	}

	return installation.Account.Login, nil
}
