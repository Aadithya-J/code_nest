package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"sort"

	"github.com/Aadithya-J/code_nest/proto"
	"github.com/Aadithya-J/code_nest/services/workspace-service/internal/k8s"
	"github.com/Aadithya-J/code_nest/services/workspace-service/internal/models"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

type ProjectRepository interface {
	CreateProject(project *models.Project) error
	GetProjectByID(id string) (*models.Project, error)
	GetProjectsByUserID(userID string) ([]models.Project, error)
	UpdateProject(id string, updates map[string]interface{}) error
	DeleteProject(id string, userID string) error
}

type FileRepository interface {
	ListFiles(projectID string) ([]models.File, error)
}

type SessionRepository interface {
	GetActiveSessionByProjectID(ctx context.Context, projectID string) (*models.WorkspaceSession, error)
	GetAllActiveSessions(ctx context.Context) ([]models.WorkspaceSession, error)
	Create(ctx context.Context, session *models.WorkspaceSession) error
	UpdateStatus(ctx context.Context, sessionID, status, statusMessage string) error
}

type KubernetesClient interface {
	WriteFile(ctx context.Context, podName, filePath, content string) error
	ReadFile(ctx context.Context, podName, filePath string) (string, error)
	ExecCommand(ctx context.Context, podName string, command []string) (string, error)
	GetFileTree(ctx context.Context, podName, dirPath string) ([]k8s.FileTreeEntry, error)
}

type EventProducer interface {
	PublishWorkspaceRequest(ctx context.Context, routingKey string, message []byte) error
}

type WorkspaceService struct {
	proto.UnimplementedWorkspaceServiceServer
	ProjectRepo ProjectRepository
	FileRepo    FileRepository
	SessionRepo SessionRepository
	Producer    EventProducer
	K8sClient   KubernetesClient
}

func NewWorkspaceService(
	projectRepo ProjectRepository,
	fileRepo FileRepository,
	sessionRepo SessionRepository,
	producer EventProducer,
	k8sClient KubernetesClient,
) *WorkspaceService {
	return &WorkspaceService{
		ProjectRepo: projectRepo,
		FileRepo:    fileRepo,
		SessionRepo: sessionRepo,
		Producer:    producer,
		K8sClient:   k8sClient,
	}
}

// Helper to publish events (optional - for audit/logging)
func (s *WorkspaceService) publishEvent(eventType string, payload interface{}) {
	jsonData, err := json.Marshal(map[string]interface{}{
		"type":    eventType,
		"payload": payload,
	})
	if err != nil {
		return
	}
	// TODO: Publish to a separate audit queue if needed
	_ = jsonData // Placeholder
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
	// Validate project exists and user has access
	project, err := s.ProjectRepo.GetProjectByID(req.ProjectId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "project not found")
	}
	if project.UserID != req.UserId {
		return nil, status.Errorf(codes.PermissionDenied, "user not authorized to save to this project")
	}

	// Get active session for this project to find the slot
	session, err := s.SessionRepo.GetActiveSessionByProjectID(ctx, req.ProjectId)
	if err != nil {
		log.Printf("SaveFile: no active session for project %s: %v", req.ProjectId, err)
		return nil, status.Errorf(codes.NotFound, "no active workspace session for this project")
	}
	if session.SlotID == nil {
		log.Printf("SaveFile: session %s has nil SlotID (status=%s)", session.SessionID, session.Status)
		return nil, status.Errorf(codes.FailedPrecondition, "session does not have a slot assigned")
	}

	// Write file to the pod using kubectl exec
	podName := *session.SlotID
	filePath := filepath.Join("/workspace/project", req.Path)

	if err := s.K8sClient.WriteFile(ctx, podName, filePath, req.Content); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to write file to pod: %v", err)
	}

	s.publishEvent("file.saved", map[string]interface{}{
		"project_id": req.ProjectId,
		"path":       req.Path,
		"session_id": session.SessionID,
		"slot_id":    *session.SlotID,
	})

	return &proto.FileResponse{
		File: &proto.File{
			Id:        "", // No DB ID since we're not storing in DB
			ProjectId: req.ProjectId,
			Path:      req.Path,
			Content:   req.Content,
			UpdatedAt: "", // Could use current time if needed
		},
	}, nil
}

func (s *WorkspaceService) GetFile(ctx context.Context, req *proto.GetFileRequest) (*proto.FileResponse, error) {
	// Validate project exists and user has access
	project, err := s.ProjectRepo.GetProjectByID(req.ProjectId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "project not found")
	}
	if project.UserID != req.UserId {
		return nil, status.Errorf(codes.PermissionDenied, "user not authorized to read from this project")
	}

	// Get active session for this project to find the slot
	session, err := s.SessionRepo.GetActiveSessionByProjectID(ctx, req.ProjectId)
	if err != nil {
		log.Printf("GetFile: no active session for project %s: %v", req.ProjectId, err)
		return nil, status.Errorf(codes.NotFound, "no active workspace session for this project")
	}
	if session.SlotID == nil {
		log.Printf("GetFile: session %s has nil SlotID (status=%s)", session.SessionID, session.Status)
		return nil, status.Errorf(codes.FailedPrecondition, "session does not have a slot assigned")
	}

	// Read file from the pod using kubectl exec
	podName := *session.SlotID
	filePath := filepath.Join("/workspace/project", req.Path)

	content, err := s.K8sClient.ReadFile(ctx, podName, filePath)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "failed to read file from pod: %v", err)
	}

	return &proto.FileResponse{
		File: &proto.File{
			Id:        "", // No DB ID
			ProjectId: req.ProjectId,
			Path:      req.Path,
			Content:   content,
			UpdatedAt: "", // Could use current time if needed
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

func (s *WorkspaceService) DeleteFile(ctx context.Context, req *proto.DeleteFileRequest) (*proto.DeleteFileResponse, error) {
	// Validate project exists and user has access
	project, err := s.ProjectRepo.GetProjectByID(req.ProjectId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "project not found")
	}
	if project.UserID != req.UserId {
		return nil, status.Errorf(codes.PermissionDenied, "user not authorized to delete from this project")
	}

	// Get active session for this project to find the slot
	session, err := s.SessionRepo.GetActiveSessionByProjectID(ctx, req.ProjectId)
	if err != nil {
		log.Printf("DeleteFile: no active session for project %s: %v", req.ProjectId, err)
		return nil, status.Errorf(codes.NotFound, "no active workspace session for this project")
	}
	if session.SlotID == nil {
		log.Printf("DeleteFile: session %s has nil SlotID (status=%s)", session.SessionID, session.Status)
		return nil, status.Errorf(codes.FailedPrecondition, "session does not have a slot assigned")
	}

	// Delete file from the pod using kubectl exec
	podName := *session.SlotID
	filePath := filepath.Join("/workspace/project", req.Path)

	command := []string{"rm", "-f", filePath}
	_, err = s.K8sClient.ExecCommand(ctx, podName, command)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete file from pod: %v", err)
	}

	s.publishEvent("file.deleted", map[string]interface{}{
		"project_id": req.ProjectId,
		"path":       req.Path,
		"session_id": session.SessionID,
		"slot_id":    *session.SlotID,
	})

	return &proto.DeleteFileResponse{
		Success: true,
		Message: "File deleted successfully",
	}, nil
}

func (s *WorkspaceService) RenameFile(ctx context.Context, req *proto.RenameFileRequest) (*proto.FileResponse, error) {
	// Validate project exists and user has access
	project, err := s.ProjectRepo.GetProjectByID(req.ProjectId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "project not found")
	}
	if project.UserID != req.UserId {
		return nil, status.Errorf(codes.PermissionDenied, "user not authorized to rename files in this project")
	}

	// Get active session for this project to find the slot
	session, err := s.SessionRepo.GetActiveSessionByProjectID(ctx, req.ProjectId)
	if err != nil {
		log.Printf("RenameFile: no active session for project %s: %v", req.ProjectId, err)
		return nil, status.Errorf(codes.NotFound, "no active workspace session for this project")
	}
	if session.SlotID == nil {
		log.Printf("RenameFile: session %s has nil SlotID (status=%s)", session.SessionID, session.Status)
		return nil, status.Errorf(codes.FailedPrecondition, "session does not have a slot assigned")
	}

	// Rename file in the pod using kubectl exec
	podName := *session.SlotID
	oldPath := filepath.Join("/workspace/project", req.OldPath)
	newPath := filepath.Join("/workspace/project", req.NewPath)

	// Create directory for new path if needed
	command := []string{"sh", "-c", fmt.Sprintf("mkdir -p \"$(dirname '%s')\" && mv '%s' '%s'", newPath, oldPath, newPath)}
	_, err = s.K8sClient.ExecCommand(ctx, podName, command)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to rename file in pod: %v", err)
	}

	s.publishEvent("file.renamed", map[string]interface{}{
		"project_id": req.ProjectId,
		"old_path":   req.OldPath,
		"new_path":   req.NewPath,
		"session_id": session.SessionID,
		"slot_id":    *session.SlotID,
	})

	return &proto.FileResponse{
		File: &proto.File{
			Id:        "",
			ProjectId: req.ProjectId,
			Path:      req.NewPath,
			Content:   "",
			UpdatedAt: "",
		},
	}, nil
}

func (s *WorkspaceService) GetFileTree(ctx context.Context, req *proto.GetFileTreeRequest) (*proto.GetFileTreeResponse, error) {
	// Validate project exists and user has access
	project, err := s.ProjectRepo.GetProjectByID(req.ProjectId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "project not found")
	}
	if project.UserID != req.UserId {
		return nil, status.Errorf(codes.PermissionDenied, "user not authorized to access this project")
	}

	// Get active session for this project to find the slot
	session, err := s.SessionRepo.GetActiveSessionByProjectID(ctx, req.ProjectId)
	if err != nil {
		log.Printf("GetFileTree: no active session for project %s: %v", req.ProjectId, err)
		return nil, status.Errorf(codes.NotFound, "no active workspace session for this project")
	}
	if session.SlotID == nil {
		log.Printf("GetFileTree: session %s has nil SlotID (status=%s)", session.SessionID, session.Status)
		return nil, status.Errorf(codes.FailedPrecondition, "session does not have a slot assigned")
	}

	// Get file tree from the pod
	podName := *session.SlotID
	projectPath := "/workspace/project"

	entries, err := s.K8sClient.GetFileTree(ctx, podName, projectPath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get file tree from pod: %v", err)
	}

	// Build tree structure
	tree := buildFileTree(entries, projectPath)

	return &proto.GetFileTreeResponse{
		Nodes: tree,
	}, nil
}

// buildFileTree converts flat file list to hierarchical tree structure
func buildFileTree(entries []k8s.FileTreeEntry, rootPath string) []*proto.FileTreeNode {
	nodeMap := make(map[string]*proto.FileTreeNode)
	var rootNodes []*proto.FileTreeNode

	for _, entry := range entries {
		if entry.Path == rootPath {
			continue
		}

		relPath, err := filepath.Rel(rootPath, entry.Path)
		if err != nil {
			relPath = entry.Path
		}

		node := &proto.FileTreeNode{
			Path:        relPath,
			IsDirectory: entry.IsDirectory,
			Children:    []*proto.FileTreeNode{},
		}
		node.Name = filepath.Base(entry.Path)

		nodeMap[entry.Path] = node
	}

	for path, node := range nodeMap {
		parentPath := filepath.Dir(path)

		if parentPath == rootPath || parentPath == "." {
			rootNodes = append(rootNodes, node)
		} else if parentNode, exists := nodeMap[parentPath]; exists {
			parentNode.Children = append(parentNode.Children, node)
		}
	}

	sortTreeNodes(rootNodes)

	return rootNodes
}

func sortTreeNodes(nodes []*proto.FileTreeNode) {
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].IsDirectory == nodes[j].IsDirectory {
			return nodes[i].Name < nodes[j].Name
		}
		return nodes[i].IsDirectory && !nodes[j].IsDirectory
	})

	for _, node := range nodes {
		if len(node.Children) > 0 {
			sortTreeNodes(node.Children)
		}
	}
}
