package provisioner

import (
	"context"
	"fmt"
	"io"
	"log"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

// DockerProvisioner implements the consumer.Provisioner interface using the Docker engine.
type DockerProvisioner struct {
	cli            *client.Client
	workspaceImage string
}

// NewDockerProvisioner creates a new DockerProvisioner.
func NewDockerProvisioner(workspaceImage string) (*DockerProvisioner, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	_, err = cli.Ping(context.Background())
	if err != nil {
		return nil, err
	}

	log.Println("Successfully connected to Docker daemon")
	return &DockerProvisioner{
		cli:            cli,
		workspaceImage: workspaceImage,
	}, nil
}

// ProvisionWorkspace creates and starts a new workspace container.
func (p *DockerProvisioner) ProvisionWorkspace(ctx context.Context, projectID, gitRepoURL string) error {
	imageName := p.workspaceImage
	containerName := fmt.Sprintf("workspace-%s", projectID)

	reader, err := p.cli.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}
	defer reader.Close()
	io.Copy(io.Discard, reader)

	exposedPorts := nat.PortSet{
		"3000/tcp": struct{}{},
		"7681/tcp": struct{}{},
	}

	config := &container.Config{
		Image:        imageName,
		ExposedPorts: exposedPorts,
		Labels: map[string]string{
			"project_id": projectID,
			"service":    "code_nest_workspace",
		},
	}

	hostConfig := &container.HostConfig{
		PublishAllPorts: true,
		RestartPolicy: container.RestartPolicy{
			Name: "unless-stopped",
		},
	}

	resp, err := p.cli.ContainerCreate(ctx, config, hostConfig, nil, nil, containerName)
	if err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}

	if err := p.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	if gitRepoURL != "" {
		if err := p.setupGitRepo(ctx, resp.ID, gitRepoURL); err != nil {
			log.Printf("Failed to setup git repo: %v", err)
		}
	}

	log.Printf("Workspace provisioned: container %s for project %s", resp.ID[:12], projectID)
	return nil
}

func (p *DockerProvisioner) setupGitRepo(ctx context.Context, containerID, repoURL string) error {
	cloneCmd := []string{"git", "clone", repoURL, "/workspace/code"}
	execConfig := container.ExecOptions{
		Cmd:          cloneCmd,
		AttachStdout: true,
		AttachStderr: true,
	}

	execResp, err := p.cli.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return err
	}

	return p.cli.ContainerExecStart(ctx, execResp.ID, container.ExecStartOptions{})
}

// DeprovisionWorkspace stops and removes a workspace container.
func (p *DockerProvisioner) DeprovisionWorkspace(ctx context.Context, projectID string) error {
	containerName := fmt.Sprintf("workspace-%s", projectID)
	
	timeout := 10
	if err := p.cli.ContainerStop(ctx, containerName, container.StopOptions{Timeout: &timeout}); err != nil {
		log.Printf("Failed to stop container %s: %v", containerName, err)
	}
	
	if err := p.cli.ContainerRemove(ctx, containerName, container.RemoveOptions{Force: true}); err != nil {
		return fmt.Errorf("failed to remove container: %w", err)
	}
	
	log.Printf("Workspace deprovisioned: %s", projectID)
	return nil
}
