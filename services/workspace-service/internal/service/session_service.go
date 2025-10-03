package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Aadithya-J/code_nest/proto"
	"github.com/Aadithya-J/code_nest/services/workspace-service/internal/kafka"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type SessionService struct {
	proto.UnimplementedSessionServiceServer
	Producer *kafka.Producer
}

func NewSessionService(producer *kafka.Producer) *SessionService {
	return &SessionService{Producer: producer}
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
	sessionID := fmt.Sprintf("session-%s-%d", req.ProjectId, time.Now().Unix())
	
	session := &WorkspaceSession{
		SessionID:  sessionID,
		ProjectID:  req.ProjectId,
		UserID:     req.UserId,
		GitRepoURL: req.GitRepoUrl,
		Status:     "CREATING",
		CreatedAt:  time.Now(),
	}

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

	if err := s.Producer.Publish(ctx, []byte(session.SessionID), eventData); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to publish event: %v", err)
	}

	return &proto.WorkspaceSessionResponse{
		SessionId: session.SessionID,
		Status:    session.Status,
		Message:   "Workspace session creation requested",
	}, nil
}

func (s *SessionService) ReleaseWorkspaceSession(ctx context.Context, req *proto.ReleaseWorkspaceSessionRequest) (*proto.WorkspaceSessionResponse, error) {
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

	if err := s.Producer.Publish(ctx, []byte(req.SessionId), eventData); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to publish event: %v", err)
	}

	return &proto.WorkspaceSessionResponse{
		SessionId: req.SessionId,
		Status:    "RELEASING",
		Message:   "Workspace session release requested",
	}, nil
}
