package service

import (
	"context"
	"encoding/json"

	"github.com/Aadithya-J/code_nest/proto"
	"github.com/Aadithya-J/code_nest/services/workspace-service/internal/kafka"
	"github.com/Aadithya-J/code_nest/services/workspace-service/internal/models"
	"github.com/Aadithya-J/code_nest/services/workspace-service/internal/repository"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

type WorkspaceService struct {
	proto.UnimplementedWorkspaceServiceServer
	Repo     *repository.ProjectRepository
	Producer *kafka.Producer
}

func NewWorkspaceService(repo *repository.ProjectRepository, producer *kafka.Producer) *WorkspaceService {
	return &WorkspaceService{Repo: repo, Producer: producer}
}

// Helper to publish events
func (s *WorkspaceService) publishEvent(eventType string, project *models.Project) {
	event := map[string]interface{}{
		"type":    eventType,
		"project": project,
	}
	payload, err := json.Marshal(event)
	if err != nil {
		// Log the error but don't block the main operation
		return
	}
	go s.Producer.Publish(context.Background(), []byte(project.ID), payload)
}

func (s *WorkspaceService) CreateProject(ctx context.Context, req *proto.CreateProjectRequest) (*proto.ProjectResponse, error) {
	project := &models.Project{
		Name:        req.Name,
		Description: req.Description,
		UserID:      req.UserId,
	}

	if err := s.Repo.CreateProject(project); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create project: %v", err)
	}

	s.publishEvent("project.created", project)

	return &proto.ProjectResponse{
		Project: &proto.Project{
			Id:          project.ID,
			Name:        project.Name,
			Description: project.Description,
			UserId:      project.UserID,
			CreatedAt:   project.CreatedAt.String(),
			UpdatedAt:   project.UpdatedAt.String(),
		},
	}, nil
}

func (s *WorkspaceService) GetProjects(ctx context.Context, req *proto.GetProjectsRequest) (*proto.GetProjectsResponse, error) {
	projects, err := s.Repo.GetProjectsByUserID(req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get projects: %v", err)
	}

	var protoProjects []*proto.Project
	for _, p := range projects {
		protoProjects = append(protoProjects, &proto.Project{
			Id:          p.ID,
			Name:        p.Name,
			Description: p.Description,
			UserId:      p.UserID,
			CreatedAt:   p.CreatedAt.String(),
			UpdatedAt:   p.UpdatedAt.String(),
		})
	}

	return &proto.GetProjectsResponse{Projects: protoProjects}, nil
}

func (s *WorkspaceService) UpdateProject(ctx context.Context, req *proto.UpdateProjectRequest) (*proto.ProjectResponse, error) {
	// First, verify the user is authorized to update this project.
	project, err := s.Repo.GetProjectByID(req.Id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, status.Errorf(codes.NotFound, "project not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to retrieve project: %v", err)
	}

	if project.UserID != req.UserId {
		return nil, status.Errorf(codes.PermissionDenied, "user not authorized to update this project")
	}

	// Create a map of the fields to update.
	updates := make(map[string]interface{})
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}

	// If there are updates, perform the update.
	if len(updates) > 0 {
		if err := s.Repo.UpdateProject(req.Id, updates); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to update project: %v", err)
		}
	}

	// Retrieve the updated project to return the latest state.
	updatedProject, err := s.Repo.GetProjectByID(req.Id)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to retrieve updated project: %v", err)
	}

	s.publishEvent("project.updated", updatedProject)

	return &proto.ProjectResponse{
		Project: &proto.Project{
			Id:          updatedProject.ID,
			Name:        updatedProject.Name,
			Description: updatedProject.Description,
			UserId:      updatedProject.UserID,
			CreatedAt:   updatedProject.CreatedAt.String(),
			UpdatedAt:   updatedProject.UpdatedAt.String(),
		},
	}, nil
}

func (s *WorkspaceService) DeleteProject(ctx context.Context, req *proto.DeleteProjectRequest) (*proto.DeleteProjectResponse, error) {
	project, err := s.Repo.GetProjectByID(req.Id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, status.Errorf(codes.NotFound, "project not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to retrieve project: %v", err)
	}

	if project.UserID != req.UserId {
		return nil, status.Errorf(codes.PermissionDenied, "user not authorized to delete this project")
	}

	if err := s.Repo.DeleteProject(req.Id, req.UserId); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete project: %v", err)
	}

	s.publishEvent("project.deleted", project)

	return &proto.DeleteProjectResponse{
		Success: true,
		Message: "Project deleted successfully",
	}, nil
}
