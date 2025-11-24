package k8s

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
)

// Client wraps Kubernetes client for executing commands in workspace pods
type Client struct {
	clientset *kubernetes.Clientset
	config    *rest.Config
	namespace string
}

// NewClient creates a new Kubernetes client
func NewClient(kubeconfigPath, namespace string) (*Client, error) {
	var config *rest.Config
	var err error

	// Try in-cluster config first
	config, err = rest.InClusterConfig()
	if err != nil {
		// Fall back to kubeconfig file
		if kubeconfigPath == "" {
			kubeconfigPath = os.Getenv("KUBECONFIG")
		}

		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return &Client{
		clientset: clientset,
		config:    config,
		namespace: namespace,
	}, nil
}

// ExecCommand executes a command in a pod and returns stdout
func (c *Client) ExecCommand(ctx context.Context, podName string, command []string) (string, error) {
	req := c.clientset.CoreV1().RESTClient().
		Post().
		Resource("pods").
		Name(podName).
		Namespace(c.namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Command: command,
			Stdin:   false,
			Stdout:  true,
			Stderr:  true,
			TTY:     false,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(c.config, "POST", req.URL())
	if err != nil {
		return "", fmt.Errorf("failed to create executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})

	if err != nil {
		return "", fmt.Errorf("command failed: %w, stderr: %s", err, stderr.String())
	}

	return stdout.String(), nil
}

// WriteFile writes content to a file in the pod
func (c *Client) WriteFile(ctx context.Context, podName, filePath, content string) error {
	// Ensure parent directory exists before writing
	dirPath := filepath.Dir(filePath)
	if dirPath != "." && dirPath != "" {
		_, err := c.ExecCommand(ctx, podName, []string{"sh", "-c", fmt.Sprintf("mkdir -p '%s'", dirPath)})
		if err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// Write file using cat with heredoc to handle special characters
	command := []string{"sh", "-c", fmt.Sprintf("cat > '%s' << 'EOF'\n%s\nEOF", filePath, content)}
	_, err := c.ExecCommand(ctx, podName, command)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// ReadFile reads content from a file in the pod
func (c *Client) ReadFile(ctx context.Context, podName, filePath string) (string, error) {
	command := []string{"cat", filePath}
	content, err := c.ExecCommand(ctx, podName, command)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}
	return content, nil
}

// ListFiles recursively lists all files in a directory
func (c *Client) ListFiles(ctx context.Context, podName, dirPath string) ([]string, error) {
	command := []string{"find", dirPath, "-type", "f"}
	output, err := c.ExecCommand(ctx, podName, command)
	if err != nil {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}

	var files []string
	for _, line := range bytes.Split([]byte(output), []byte("\n")) {
		if len(line) > 0 {
			files = append(files, string(line))
		}
	}

	return files, nil
}

// GetPodByLabel finds a pod by label selector
func (c *Client) GetPodByLabel(ctx context.Context, labelSelector string) (*corev1.Pod, error) {
	pods, err := c.clientset.CoreV1().Pods(c.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	if len(pods.Items) == 0 {
		return nil, fmt.Errorf("no pod found with label %s", labelSelector)
	}

	return &pods.Items[0], nil
}

// FileTreeEntry represents a file or directory in the tree
type FileTreeEntry struct {
	Path        string
	IsDirectory bool
}

// GetFileTree recursively lists all files and directories
func (c *Client) GetFileTree(ctx context.Context, podName, dirPath string) ([]FileTreeEntry, error) {
	// Use find to list all files and directories with type indicator
	command := []string{"sh", "-c", fmt.Sprintf("find '%s' -printf '%%p|%%y\\n' 2>/dev/null || find '%s' -exec stat -c '%%n|%%F' {} \\;", dirPath, dirPath)}
	output, err := c.ExecCommand(ctx, podName, command)
	if err != nil {
		return nil, fmt.Errorf("failed to get file tree: %w", err)
	}

	var entries []FileTreeEntry
	for _, line := range bytes.Split([]byte(output), []byte("\n")) {
		if len(line) == 0 {
			continue
		}

		parts := bytes.SplitN(line, []byte("|"), 2)
		if len(parts) != 2 {
			continue
		}

		path := string(parts[0])
		fileType := string(parts[1])

		isDir := fileType == "d" || fileType == "directory"

		entries = append(entries, FileTreeEntry{
			Path:        path,
			IsDirectory: isDir,
		})
	}

	return entries, nil
}
