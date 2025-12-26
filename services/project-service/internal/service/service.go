package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Aadithya-J/code_nest/proto"
	"github.com/Aadithya-J/code_nest/services/project-service/internal/db"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// AuthClient exposes only the call we need from auth-service.
type AuthClient interface {
	GenerateRepoToken(ctx context.Context, in *proto.GenerateRepoTokenRequest, opts ...grpc.CallOption) (*proto.GenerateRepoTokenResponse, error)
}

// CircuitBreaker implements a simple circuit breaker pattern
type CircuitBreaker struct {
	maxFailures  int32
	resetTimeout time.Duration
	failures     int32
	lastFailTime int64
	state        int32 // 0=closed, 1=open, 2=half-open
	mu           sync.RWMutex
}

func NewCircuitBreaker(maxFailures int32, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		maxFailures:  maxFailures,
		resetTimeout: resetTimeout,
		state:        0, // closed
	}
}

func (cb *CircuitBreaker) Call(fn func() error) error {
	state := atomic.LoadInt32(&cb.state)

	if state == 1 { // open
		if time.Since(time.Unix(atomic.LoadInt64(&cb.lastFailTime), 0)) > cb.resetTimeout {
			if atomic.CompareAndSwapInt32(&cb.state, 1, 2) { // half-open
				state = 2
			}
		} else {
			return errors.New("circuit breaker is open")
		}
	}

	err := fn()

	if err != nil {
		cb.mu.Lock()
		cb.failures++
		atomic.StoreInt64(&cb.lastFailTime, time.Now().Unix())

		if cb.failures >= cb.maxFailures {
			atomic.StoreInt32(&cb.state, 1) // open
		}
		cb.mu.Unlock()
		return err
	}

	// Success - reset failures
	cb.mu.Lock()
	cb.failures = 0
	if state == 2 { // half-open
		atomic.StoreInt32(&cb.state, 0) // closed
	}
	cb.mu.Unlock()

	return nil
}

type Service struct {
	proto.UnimplementedProjectServiceServer
	db        *gorm.DB
	rdb       *redis.Client
	auth      AuthClient
	atlasBase string
	gateway   string
	httpCli   *http.Client
	cb        *CircuitBreaker
}

// generateAtlasID creates a consistent Atlas ID for a project
func (s *Service) generateAtlasID(projectID string) string {
	return fmt.Sprintf("ws-%s", projectID)
}

func New(db *gorm.DB, rdb *redis.Client, auth AuthClient, atlasBase, gateway string) *Service {
	if db == nil {
		panic("database connection is required")
	}
	if rdb == nil {
		panic("redis connection is required")
	}
	if auth == nil {
		panic("auth client is required")
	}
	if atlasBase == "" {
		panic("atlas base URL is required")
	}
	if gateway == "" {
		panic("gateway URL is required")
	}

	return &Service{
		db:        db,
		rdb:       rdb,
		auth:      auth,
		atlasBase: atlasBase,
		gateway:   gateway,
		httpCli: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
				DisableCompression:  false,
			},
		},
		cb: NewCircuitBreaker(5, 30*time.Second),
	}
}

// CreateProject inserts a STOPPED project row.
func (s *Service) CreateProject(ctx context.Context, req *proto.CreateProjectRequest) (*proto.CreateProjectResponse, error) {
	if req.GetUserId() == "" || req.GetRepoUrl() == "" || req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id, name, repo_url required")
	}
	// Additional validation
	if len(req.GetName()) < 2 || len(req.GetName()) > 100 {
		return nil, status.Error(codes.InvalidArgument, "project name must be between 2 and 100 characters")
	}
	if !strings.HasPrefix(req.GetRepoUrl(), "https://github.com/") && !strings.HasPrefix(req.GetRepoUrl(), "git@github.com:") {
		return nil, status.Error(codes.InvalidArgument, "repo_url must be a GitHub repository")
	}
	id := uuid.New().String()
	project := db.Project{
		ID:      id,
		Name:    req.GetName(),
		UserID:  req.GetUserId(),
		RepoURL: req.GetRepoUrl(),
		Status:  "STOPPED",
	}
	// Precompute atlas id for consistency.
	project.AtlasID = s.generateAtlasID(id)
	if err := s.db.WithContext(ctx).Create(&project).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "create project: %v", err)
	}
	return &proto.CreateProjectResponse{ProjectId: id}, nil
}

// StartWorkspace orchestrates start without creating projects.
func (s *Service) StartWorkspace(ctx context.Context, req *proto.StartWorkspaceRequest) (*proto.StartWorkspaceResponse, error) {
	if req.GetProjectId() == "" || req.GetUserId() == "" {
		return nil, status.Error(codes.InvalidArgument, "project_id and user_id required")
	}

	lockKey := fmt.Sprintf("lock:project:%s", req.GetProjectId())
	ok, err := s.rdb.SetNX(ctx, lockKey, "1", 30*time.Second).Result()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "lock error: %v", err)
	}
	if !ok {
		return nil, status.Error(codes.ResourceExhausted, "project busy")
	}
	defer s.rdb.Del(ctx, lockKey)

	var project db.Project
	if err := s.db.WithContext(ctx).First(&project, "id = ?", req.GetProjectId()).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "project not found")
		}
		return nil, status.Errorf(codes.Internal, "fetch project: %v", err)
	}
	if project.UserID != req.GetUserId() {
		return nil, status.Error(codes.PermissionDenied, "not owner")
	}

	if project.Status == "RUNNING" || project.Status == "STARTING" {
		return &proto.StartWorkspaceResponse{
			Ok:      true,
			Status:  toStatusEnum(project.Status),
			AtlasId: project.AtlasID,
		}, nil
	}

	callbackToken := uuid.New().String()
	project.WebhookSecret = callbackToken
	project.Status = "STARTING"
	if project.AtlasID == "" {
		project.AtlasID = s.generateAtlasID(project.ID)
	}
	if err := s.db.WithContext(ctx).Save(&project).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "save project: %v", err)
	}

	git, err := s.auth.GenerateRepoToken(ctx, &proto.GenerateRepoTokenRequest{UserId: req.GetUserId()})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get repo token: %v", err)
	}

	payload := map[string]interface{}{
		"id":    project.AtlasID,
		"image": "aadithya1/ide-agent:latest",
		"auth_config": map[string]interface{}{
			"enabled":    true,
			"verify_url": s.gateway + "/auth/verify",
		},
		"env": map[string]string{
			"GIT_REPO":             project.RepoURL,
			"GIT_TOKEN":            git.GetToken(),
			"GIT_USER_NAME":        git.GetUsername(),
			"AGENT_CALLBACK_URL":   s.gateway + "/api/internal/webhook",
			"AGENT_CALLBACK_TOKEN": callbackToken,
			"ATLAS_ID":             project.AtlasID,
			"ATLAS_BASE_URL":       s.atlasBase,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to marshal atlas request: %v", err)
	}

	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, s.atlasBase+"/sandboxes", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	var resp *http.Response
	err = s.cb.Call(func() error {
		var err error
		resp, err = s.httpCli.Do(request)
		return err
	})

	if err != nil {
		return nil, status.Errorf(codes.Internal, "atlas request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, status.Errorf(codes.Internal, "atlas error status: %d, response: %s", resp.StatusCode, string(body))
	}

	return &proto.StartWorkspaceResponse{
		Ok:      true,
		Status:  proto.ProjectStatus_STARTING,
		AtlasId: project.AtlasID,
	}, nil
}

func (s *Service) StopWorkspace(ctx context.Context, req *proto.StopWorkspaceRequest) (*proto.StopWorkspaceResponse, error) {
	if req.GetProjectId() == "" || req.GetAtlasId() == "" {
		return nil, status.Error(codes.InvalidArgument, "project_id and atlas_id required")
	}

	url := fmt.Sprintf("%s/sandboxes/%s", s.atlasBase, req.GetAtlasId())
	request, _ := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)

	var resp *http.Response
	var err error
	err = s.cb.Call(func() error {
		resp, err = s.httpCli.Do(request)
		return err
	})

	if err != nil {
		return nil, status.Errorf(codes.Internal, "atlas delete failed: %v", err)
	}
	defer resp.Body.Close()

	// update status best-effort
	s.db.WithContext(ctx).Model(&db.Project{}).
		Where("id = ?", req.GetProjectId()).
		Update("status", "STOPPED")

	return &proto.StopWorkspaceResponse{Ok: true}, nil
}

// WebhookUpdate validates the callback_token and updates status.
func (s *Service) WebhookUpdate(ctx context.Context, req *proto.WebhookUpdateRequest) (*proto.WebhookUpdateResponse, error) {
	if req.GetAtlasId() == "" || req.GetCallbackToken() == "" {
		return nil, status.Error(codes.InvalidArgument, "atlas_id and callback_token required")
	}

	var project db.Project
	// Use pessimistic locking to prevent race conditions
	if err := s.db.WithContext(ctx).Set("gorm:query_option", "FOR UPDATE").First(&project, "atlas_id = ?", req.GetAtlasId()).Error; err != nil {
		return nil, status.Errorf(codes.NotFound, "project not found: %v", err)
	}
	if project.WebhookSecret == "" || project.WebhookSecret != req.GetCallbackToken() {
		return nil, status.Error(codes.PermissionDenied, "invalid callback token")
	}

	statusVal := req.GetStatus()
	switch statusVal {
	case "READY":
		project.Status = "RUNNING"
	case "ERROR":
		project.Status = "ERROR"
	default:
		return nil, status.Error(codes.InvalidArgument, "invalid status")
	}
	if err := s.db.WithContext(ctx).Save(&project).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "update project: %v", err)
	}
	return &proto.WebhookUpdateResponse{Ok: true}, nil
}

// VerifyAndComplete validates callback token and updates status.
func (s *Service) VerifyAndComplete(ctx context.Context, req *proto.VerifyAndCompleteRequest) (*proto.VerifyAndCompleteResponse, error) {
	if req.GetAtlasId() == "" || req.GetCallbackToken() == "" {
		return nil, status.Error(codes.InvalidArgument, "atlas_id and callback_token required")
	}
	var project db.Project
	// Use pessimistic locking to prevent race conditions
	if err := s.db.WithContext(ctx).Set("gorm:query_option", "FOR UPDATE").First(&project, "atlas_id = ?", req.GetAtlasId()).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "project not found")
		}
		return nil, status.Errorf(codes.Internal, "query project: %v", err)
	}
	if project.WebhookSecret == "" || project.WebhookSecret != req.GetCallbackToken() {
		return nil, status.Error(codes.PermissionDenied, "invalid callback token")
	}
	switch req.GetStatus() {
	case "READY":
		project.Status = "RUNNING"
	case "ERROR":
		project.Status = "ERROR"
	default:
		return nil, status.Error(codes.InvalidArgument, "invalid status")
	}
	if err := s.db.WithContext(ctx).Save(&project).Error; err != nil {
		return nil, status.Errorf(codes.Internal, "update project: %v", err)
	}
	return &proto.VerifyAndCompleteResponse{Ok: true, Status: toStatusEnum(project.Status)}, nil
}

// IsOwner checks if user owns project (project_id which contains atlas_id).
func (s *Service) IsOwner(ctx context.Context, req *proto.IsOwnerRequest) (*proto.IsOwnerResponse, error) {
	if req.GetUserId() == "" || req.GetProjectId() == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and project_id required")
	}
	var project db.Project
	if err := s.db.WithContext(ctx).First(&project, "atlas_id = ?", req.GetProjectId()).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &proto.IsOwnerResponse{IsOwner: false}, nil
		}
		return nil, status.Errorf(codes.Internal, "query project: %v", err)
	}
	return &proto.IsOwnerResponse{IsOwner: project.UserID == req.GetUserId()}, nil
}

func randomSecret(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	res := make([]byte, n)
	for i := range res {
		res[i] = letters[rand.Intn(len(letters))]
	}
	return string(res)
}

func toStatusEnum(status string) proto.ProjectStatus {
	switch status {
	case "STOPPED":
		return proto.ProjectStatus_STOPPED
	case "STARTING":
		return proto.ProjectStatus_STARTING
	case "RUNNING":
		return proto.ProjectStatus_RUNNING
	case "ERROR":
		return proto.ProjectStatus_ERROR
	default:
		return proto.ProjectStatus_PROJECT_STATUS_UNSPECIFIED
	}
}
