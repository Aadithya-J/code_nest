package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
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
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gopkg.in/square/go-jose.v2"
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
	Get(url string) (*http.Response, error)
}

type UserRepository interface {
	Create(user *repository.User) error
	FindByEmail(email string) (*repository.User, error)
	FindByID(id string) (*repository.User, error)
}

type GitHubInstallationRepository interface {
	Create(installation *repository.GitHubInstallation) error
	FindByUserID(userID string) ([]*repository.GitHubInstallation, error)
	FindByInstallationID(installationID int64) (*repository.GitHubInstallation, error)
	UpdateAccessToken(installationID int64, token string, expiry int64) error
}

type AuthService struct {
	proto.UnimplementedAuthServiceServer
	repo             UserRepository
	githubRepo       GitHubInstallationRepository
	privateKey       *rsa.PrivateKey
	jwks             *jose.JSONWebKeySet
	oauthConf        *oauth2.Config
	githubAppID      int64
	githubAppSlug    string
	githubPrivateKey []byte
	httpClient       HTTPClient
}

func NewAuthService(repo UserRepository, githubRepo GitHubInstallationRepository, oauthConf *oauth2.Config, cfg config.Config, httpClient HTTPClient) (*AuthService, error) {
	privateKey, err := loadOrGenerateRSAKey()
	if err != nil {
		return nil, fmt.Errorf("failed to load or generate rsa key: %w", err)
	}

	jwk := jose.JSONWebKey{Key: &privateKey.PublicKey, KeyID: "1", Algorithm: "RS256", Use: "sig"}
	jwks := &jose.JSONWebKeySet{Keys: []jose.JSONWebKey{jwk}}

	var pkBytes []byte
	if cfg.GitHub.PrivateKeyPEM != "" {
		pkBytes = []byte(cfg.GitHub.PrivateKeyPEM)
	} else {
		var err error
		pkBytes, err = os.ReadFile(cfg.GitHub.PrivateKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read GitHub private key from %s: %w", cfg.GitHub.PrivateKeyPath, err)
		}
	}

	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
				DisableCompression:  false,
			},
		}
	}

	return &AuthService{
		repo:             repo,
		githubRepo:       githubRepo,
		privateKey:       privateKey,
		jwks:             jwks,
		oauthConf:        oauthConf,
		githubAppID:      cfg.GitHub.AppID,
		githubAppSlug:    cfg.GitHub.AppSlug,
		githubPrivateKey: pkBytes,
		httpClient:       httpClient,
	}, nil
}

func loadOrGenerateRSAKey() (*rsa.PrivateKey, error) {
	keyPath := os.Getenv("RSA_KEY_PATH")
	if keyPath == "" {
		keyPath = "/app/keys/rsa_private.pem"
	}

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

	log.Println("Generating new RSA key and saving to", keyPath)
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	if err := saveRSAKey(privateKey, keyPath); err != nil {
		log.Printf("Warning: failed to save RSA key: %v", err)
		// Continue anyway - key will work for this session
	}

	return privateKey, nil
}

func saveRSAKey(privateKey *rsa.PrivateKey, keyPath string) error {
	if err := os.MkdirAll(filepath.Dir(keyPath), 0755); err != nil {
		return err
	}

	keyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: keyBytes,
	})

	return os.WriteFile(keyPath, keyPEM, 0600)
}

func (s *AuthService) Signup(ctx context.Context, req *proto.SignupRequest) (*proto.AuthResponse, error) {
	// Input validation
	if req.Email == "" {
		return nil, status.Error(codes.InvalidArgument, "email is required")
	}
	if req.Password == "" {
		return nil, status.Error(codes.InvalidArgument, "password is required")
	}
	if len(req.Password) < 8 {
		return nil, status.Error(codes.InvalidArgument, "password must be at least 8 characters")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	user := &repository.User{Email: req.Email, PasswordHash: string(hash)}
	if err := s.repo.Create(user); err != nil {
		return nil, err
	}
	token, err := s.GenerateToken(user.ID, req.Email)
	if err != nil {
		return nil, err
	}
	return &proto.AuthResponse{Token: token}, nil
}

func (s *AuthService) Login(ctx context.Context, req *proto.LoginRequest) (*proto.AuthResponse, error) {
	// Input validation
	if req.Email == "" {
		return nil, status.Error(codes.InvalidArgument, "email is required")
	}
	if req.Password == "" {
		return nil, status.Error(codes.InvalidArgument, "password is required")
	}

	user, err := s.repo.FindByEmail(req.Email)
	if err != nil {
		return nil, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid credentials")
	}
	token, err := s.GenerateToken(user.ID, req.Email)
	if err != nil {
		return nil, err
	}
	return &proto.AuthResponse{Token: token}, nil
}

func (s *AuthService) GenerateToken(userID, email string) (string, error) {
	claims := jwt.MapClaims{
		"sub":   userID,
		"email": email,
		"exp":   time.Now().Add(time.Hour * 72).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)

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
	jwt, err := s.GenerateToken(user.ID, email)
	if err != nil {
		return nil, err
	}
	return &proto.AuthResponse{Token: jwt}, nil
}

func (s *AuthService) GetGoogleAuthURL(ctx context.Context, req *proto.GetGoogleAuthURLRequest) (*proto.GetGoogleAuthURLResponse, error) {
	url := s.oauthConf.AuthCodeURL(req.State)
	return &proto.GetGoogleAuthURLResponse{Url: url}, nil
}

func (s *AuthService) GetGitHubAuthURL(ctx context.Context, req *proto.GetGitHubAuthURLRequest) (*proto.GetGitHubAuthURLResponse, error) {
	installURL := fmt.Sprintf("https://github.com/apps/%s/installations/new", s.githubAppSlug)
	return &proto.GetGitHubAuthURLResponse{Url: installURL}, nil
}

func (s *AuthService) HandleGitHubCallback(ctx context.Context, req *proto.HandleGitHubCallbackRequest) (*proto.AuthResponse, error) {
	if req.UserId == "" {
		return &proto.AuthResponse{Error: "User ID required"}, nil
	}

	user, err := s.repo.FindByID(req.UserId)
	if err != nil {
		return &proto.AuthResponse{Error: "User not found"}, nil
	}

	username, err := s.getGitHubUsername(ctx, req.InstallationId)
	if err != nil {
		return &proto.AuthResponse{Error: "Failed to get GitHub username"}, nil
	}

	// Create GitHub installation record
	installation := &repository.GitHubInstallation{
		InstallationID:      req.InstallationId,
		UserID:              user.ID,
		AccountName:         username,
		AccountType:         "User", // Default to User, can be updated from GitHub API
		RepositorySelection: "all",  // Default to all, can be updated from GitHub API
	}

	err = s.githubRepo.Create(installation)
	if err != nil {
		return &proto.AuthResponse{Error: "Failed to save GitHub installation"}, nil
	}

	token, err := s.GenerateToken(user.ID, user.Email)
	if err != nil {
		return &proto.AuthResponse{Error: "Failed to generate token"}, nil
	}
	return &proto.AuthResponse{Token: token}, nil
}

func (s *AuthService) GetGitHubAccessToken(ctx context.Context, req *proto.GetGitHubAccessTokenRequest) (*proto.GetGitHubAccessTokenResponse, error) {
	// Get GitHub installation for this user
	installations, err := s.githubRepo.FindByUserID(req.UserId)
	if err != nil || len(installations) == 0 {
		return nil, fmt.Errorf("user has not linked GitHub")
	}

	// Use the first installation (could be enhanced to support multiple)
	installation := installations[0]

	appJWT, err := s.generateGitHubAppJWT()
	if err != nil {
		return nil, fmt.Errorf("failed to generate app JWT: %w", err)
	}

	token, _, err := s.getInstallationAccessToken(ctx, installation.InstallationID, appJWT)
	if err != nil {
		return nil, fmt.Errorf("failed to get installation token: %w", err)
	}

	username := installation.AccountName
	if username == "" {
		if u, err := s.getGitHubUsername(ctx, installation.InstallationID); err == nil {
			username = u
		}
	}

	return &proto.GetGitHubAccessTokenResponse{
		Token:    token,
		Username: username,
	}, nil
}

func (s *AuthService) ValidateToken(ctx context.Context, req *proto.ValidateTokenRequest) (*proto.ValidateTokenResponse, error) {
	if req.GetToken() == "" {
		return &proto.ValidateTokenResponse{
			Valid: false,
			Error: "token required",
		}, nil
	}

	parsed, err := jwt.Parse(req.Token, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Method)
		}
		return &s.privateKey.PublicKey, nil
	})
	if err != nil || !parsed.Valid {
		return &proto.ValidateTokenResponse{
			Valid: false,
			Error: "invalid token",
		}, nil
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return &proto.ValidateTokenResponse{
			Valid: false,
			Error: "invalid claims",
		}, nil
	}

	sub, _ := claims["sub"].(string)
	return &proto.ValidateTokenResponse{
		Valid:  true,
		UserId: sub,
	}, nil
}

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

func (s *AuthService) getInstallationAccessToken(ctx context.Context, installationID int64, appJWT string) (string, int64, error) {
	url := fmt.Sprintf("https://api.github.com/app/installations/%d/access_tokens", installationID)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer([]byte("{}")))
	if err != nil {
		return "", 0, err
	}

	req.Header.Set("Authorization", "Bearer "+appJWT)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
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

func (s *AuthService) getGitHubUsername(ctx context.Context, installationID int64) (string, error) {
	appJWT, err := s.generateGitHubAppJWT()
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("https://api.github.com/app/installations/%d", installationID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+appJWT)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := s.httpClient.Do(req)
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

func (s *AuthService) GenerateRepoToken(ctx context.Context, req *proto.GenerateRepoTokenRequest) (*proto.GenerateRepoTokenResponse, error) {
	// Find GitHub installation for the user
	installations, err := s.githubRepo.FindByUserID(req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to find installations: %v", err)
	}
	if len(installations) == 0 {
		return nil, status.Errorf(codes.NotFound, "no GitHub installation found for user")
	}
	inst := installations[0]

	// If we have a cached token and it's not expired, return it
	if inst.AccessToken != "" && inst.TokenExpiry > time.Now().Unix() {
		return &proto.GenerateRepoTokenResponse{
			Token:    inst.AccessToken,
			Username: inst.AccountName,
		}, nil
	}

	// Mint a new installation access token
	appJWT, err := s.generateGitHubAppJWT()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate app JWT: %v", err)
	}
	token, _, err := s.getInstallationAccessToken(ctx, inst.InstallationID, appJWT)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get installation access token: %v", err)
	}
	username := inst.AccountName

	// Cache the token
	if err := s.githubRepo.UpdateAccessToken(inst.InstallationID, token, time.Now().Add(50*time.Minute).Unix()); err != nil {
		log.Printf("failed to cache installation token: %v", err)
	}

	return &proto.GenerateRepoTokenResponse{
		Token:    token,
		Username: username,
	}, nil
}
