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
	ProjectRepo *repository.ProjectRepository
	FileRepo    *repository.FileRepository
	Producer    *kafka.Producer
}

func NewWorkspaceService(projectRepo *repository.ProjectRepository, fileRepo *repository.FileRepository, producer *kafka.Producer) *WorkspaceService {
	return &WorkspaceService{ProjectRepo: projectRepo, FileRepo: fileRepo, Producer: producer}
}

// Helper to publish events
func (s *WorkspaceService) publishEvent(eventType string, payload interface{}) {
	jsonData, err := json.Marshal(map[string]interface{}{
		"type":    eventType,
		"payload": payload,
	})
	if err != nil {
		return
	}
	go s.Producer.Publish(context.Background(), nil, jsonData)
}

func (s *WorkspaceService) CreateProject(ctx context.Context, req *proto.CreateProjectRequest) (*proto.ProjectResponse, error) {
	project := &models.Project{
		Name:        req.Name,
		Description: req.Description,
		UserID:      req.UserId,
	}

	if err := s.ProjectRepo.CreateProject(project); err != nil {
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
	projects, err := s.ProjectRepo.GetProjectsByUserID(req.UserId)
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
	project, err := s.ProjectRepo.GetProjectByID(req.Id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, status.Errorf(codes.NotFound, "project not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to retrieve project: %v", err)
	}

	if project.UserID != req.UserId {
		return nil, status.Errorf(codes.PermissionDenied, "user not authorized to update this project")
	}

	updates := make(map[string]interface{})
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}

	if len(updates) > 0 {
		if err := s.ProjectRepo.UpdateProject(req.Id, updates); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to update project: %v", err)
		}
	}

	updatedProject, err := s.ProjectRepo.GetProjectByID(req.Id)
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
	project, err := s.ProjectRepo.GetProjectByID(req.Id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, status.Errorf(codes.NotFound, "project not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to retrieve project: %v", err)
	}

	if project.UserID != req.UserId {
		return nil, status.Errorf(codes.PermissionDenied, "user not authorized to delete this project")
	}

	if err := s.ProjectRepo.DeleteProject(req.Id, req.UserId); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete project: %v", err)
	}

	s.publishEvent("project.deleted", project)

	return &proto.DeleteProjectResponse{
		Success: true,
		Message: "Project deleted successfully",
	}, nil
}

func (s *WorkspaceService) SaveFile(ctx context.Context, req *proto.SaveFileRequest) (*proto.FileResponse, error) {
	project, err := s.ProjectRepo.GetProjectByID(req.ProjectId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "project not found")
	}
	if project.UserID != req.UserId {
		return nil, status.Errorf(codes.PermissionDenied, "user not authorized to save to this project")
	}

	file := &models.File{
		ProjectID: req.ProjectId,
		Path:      req.Path,
		Content:   req.Content,
	}

	if err := s.FileRepo.SaveFile(file); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to save file: %v", err)
	}

	s.publishEvent("file.saved", file)

	return &proto.FileResponse{
		File: &proto.File{
			Id:        file.ID,
			ProjectId: file.ProjectID,
			Path:      file.Path,
			Content:   file.Content,
			UpdatedAt: file.UpdatedAt.String(),
		},
	}, nil
}

func (s *WorkspaceService) GetFile(ctx context.Context, req *proto.GetFileRequest) (*proto.FileResponse, error) {
	project, err := s.ProjectRepo.GetProjectByID(req.ProjectId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "project not found")
	}
	if project.UserID != req.UserId {
		return nil, status.Errorf(codes.PermissionDenied, "user not authorized to read from this project")
	}

	file, err := s.FileRepo.GetFile(req.ProjectId, req.Path)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "file not found")
	}

	return &proto.FileResponse{
		File: &proto.File{
			Id:        file.ID,
			ProjectId: file.ProjectID,
			Path:      file.Path,
			Content:   file.Content,
			UpdatedAt: file.UpdatedAt.String(),
		},
	}, nil
}

func (s *WorkspaceService) ListFiles(ctx context.Context, req *proto.ListFilesRequest) (*proto.ListFilesResponse, error) {
	project, err := s.ProjectRepo.GetProjectByID(req.ProjectId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "project not found")
	}
	if project.UserID != req.UserId {
		return nil, status.Errorf(codes.PermissionDenied, "user not authorized to list files in this project")
	}

	files, err := s.FileRepo.ListFiles(req.ProjectId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list files: %v", err)
	}

	var protoFiles []*proto.File
	for _, f := range files {
		protoFiles = append(protoFiles, &proto.File{
			Id:        f.ID,
			ProjectId: f.ProjectID,
			Path:      f.Path,
			Content:   f.Content,
			UpdatedAt: f.UpdatedAt.String(),
		})
	}

	return &proto.ListFilesResponse{Files: protoFiles}, nil
}
