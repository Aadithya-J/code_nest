package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/Aadithya-J/code_nest/proto"
	"github.com/Aadithya-J/code_nest/services/workspace-service/internal/models"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type SessionService struct {
	proto.UnimplementedSessionServiceServer
	Producer    EventProducer
	ProjectRepo ProjectRepository
	SessionRepo SessionRepository
}

func NewSessionService(producer EventProducer, projectRepo ProjectRepository, sessionRepo SessionRepository) *SessionService {
	return &SessionService{
		Producer:    producer,
		ProjectRepo: projectRepo,
		SessionRepo: sessionRepo,
	}
}

type WorkspaceSession struct {
	SessionID  string    `json:"session_id"`
	ProjectID  string    `json:"project_id"`
	UserID     string    `json:"user_id"`
	GitRepoURL string    `json:"git_repo_url"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

func (s *SessionService) CreateWorkspaceSession(ctx context.Context, req *proto.CreateWorkspaceSessionRequest) (*proto.WorkspaceSessionResponse, error) {
	// Validate project exists
	project, err := s.ProjectRepo.GetProjectByID(req.ProjectId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "project not found: %v", err)
	}

	// Verify user owns the project
	if project.UserID != req.UserId {
		return nil, status.Errorf(codes.PermissionDenied, "user does not own this project")
	}

	// Generate unique session ID using random bytes
	randomBytes := make([]byte, 8)
	if _, err := rand.Read(randomBytes); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate session ID: %v", err)
	}
	sessionID := fmt.Sprintf("session-%s-%s", req.ProjectId, hex.EncodeToString(randomBytes))

	session := &WorkspaceSession{
		SessionID:  sessionID,
		ProjectID:  req.ProjectId,
		UserID:     req.UserId,
		GitRepoURL: req.GitRepoUrl,
		Status:     "CREATING",
		CreatedAt:  time.Now(),
	}

	// Persist session record so workspace-service can look it up while provisioning
	initialStatusMessage := "Workspace session creation requested"
	modelSession := &models.WorkspaceSession{
		SessionID:     session.SessionID,
		ProjectID:     session.ProjectID,
		UserID:        session.UserID,
		SlotID:        nil,
		GitRepoURL:    session.GitRepoURL,
		Status:        session.Status,
		StatusMessage: initialStatusMessage,
		CreatedAt:     session.CreatedAt,
		UpdatedAt:     session.CreatedAt,
	}

	if err := s.SessionRepo.Create(ctx, modelSession); err != nil {
		log.Printf("CreateWorkspaceSession: failed to persist session %s for project %s: %v", session.SessionID, session.ProjectID, err)
		return nil, status.Errorf(codes.Internal, "failed to persist session: %v", err)
	}

	log.Printf("CreateWorkspaceSession: created session %s for project %s (user=%s, status=%s)", session.SessionID, session.ProjectID, session.UserID, session.Status)

	event := map[string]interface{}{
		"event_type": "WORKSPACE_CREATE_REQUESTED",
		"timestamp":  time.Now().UTC(),
		"payload": map[string]interface{}{
			"project_id":   session.ProjectID,
			"user_id":      session.UserID,
			"git_repo_url": session.GitRepoURL,
			"session_id":   session.SessionID,
		},
	}

	eventData, err := json.Marshal(event)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to marshal event: %v", err)
	}

	if err := s.Producer.PublishWorkspaceRequest(ctx, "create.requested", eventData); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to publish event: %v", err)
	}

	return &proto.WorkspaceSessionResponse{
		SessionId: session.SessionID,
		Status:    session.Status,
		Message:   "Workspace session creation requested",
	}, nil
}

func (s *SessionService) ReleaseWorkspaceSession(ctx context.Context, req *proto.ReleaseWorkspaceSessionRequest) (*proto.WorkspaceSessionResponse, error) {
	// Verify project exists and belongs to the user
	project, err := s.ProjectRepo.GetProjectByID(req.ProjectId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "project not found: %v", err)
	}

	if project.UserID != req.UserId {
		return nil, status.Errorf(codes.PermissionDenied, "unauthorized: project does not belong to user")
	}

	if err := s.SessionRepo.UpdateStatus(ctx, req.SessionId, models.SessionStatusReleasing, "Release requested"); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update session status: %v", err)
	}

	event := map[string]interface{}{
		"event_type": "WORKSPACE_RELEASE_REQUESTED",
		"timestamp":  time.Now().UTC(),
		"payload": map[string]interface{}{
			"project_id": req.ProjectId,
			"user_id":    req.UserId,
			"session_id": req.SessionId,
		},
	}

	eventData, err := json.Marshal(event)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to marshal event: %v", err)
	}

	if err := s.Producer.PublishWorkspaceRequest(ctx, "release.requested", eventData); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to publish event: %v", err)
	}

	return &proto.WorkspaceSessionResponse{
		SessionId: req.SessionId,
		Status:    "RELEASING",
		Message:   "Workspace session release requested",
	}, nil
}

func (s *SessionService) GetAllActiveSessions(ctx context.Context, req *proto.GetAllActiveSessionsRequest) (*proto.GetAllActiveSessionsResponse, error) {
	// This is a development-only endpoint
	sessions, err := s.SessionRepo.GetAllActiveSessions(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get active sessions: %v", err)
	}

	var activeSessions []*proto.ActiveSession
	for _, session := range sessions {
		activeSessions = append(activeSessions, &proto.ActiveSession{
			SessionId: session.SessionID,
			ProjectId: session.ProjectID,
			UserId:    session.UserID,
			Status:    session.Status,
		})
	}

	return &proto.GetAllActiveSessionsResponse{
		Sessions: activeSessions,
	}, nil
}
