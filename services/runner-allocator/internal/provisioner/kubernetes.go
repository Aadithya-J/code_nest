package provisioner

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/Aadithya-J/code_nest/services/runner-allocator/internal/models"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// KubernetesProvisioner manages workspace pods using the Kubernetes API
type KubernetesProvisioner struct {
	clientset      *kubernetes.Clientset
	namespace      string
	workspaceImage string
}

// NewKubernetesProvisioner creates a new Kubernetes provisioner
func NewKubernetesProvisioner(workspaceImage, kubeconfigPath string) (*KubernetesProvisioner, error) {
	// Use provided kubeconfig path or default locations
	if kubeconfigPath == "" {
		if home := homedir.HomeDir(); home != "" {
			kubeconfigPath = filepath.Join(home, ".kube", "config")
		}
		// Fallback to k3s default location
		if kubeconfigPath == "" {
			kubeconfigPath = "/tmp/k3s-config.yaml"
		}
	}

	// Build config from kubeconfig file
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubeconfig: %w", err)
	}

	// Create clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Test connection
	_, err = clientset.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to kubernetes cluster: %w", err)
	}

	log.Println("Successfully connected to Kubernetes cluster")
	return &KubernetesProvisioner{
		clientset:      clientset,
		namespace:      "workspaces", // Use dedicated namespace for workspaces
		workspaceImage: workspaceImage,
	}, nil
}

// ProvisionWorkspace assigns a project to an existing slot pod
func (k *KubernetesProvisioner) ProvisionWorkspace(ctx context.Context, projectID, gitRepoURL string) error {
	// In the slot model, we don't create new pods - we update existing slot pods
	// The slot assignment is handled by the store, we just need to update the pod
	// For now, this is a placeholder - actual implementation will restart pod with new env
	log.Printf("Provisioning workspace for project %s in assigned slot", projectID)
	return nil
}

// CreateSlotPod creates a fixed slot pod (called once at startup)
func (k *KubernetesProvisioner) CreateSlotPod(ctx context.Context, slotID string) error {
	podName := fmt.Sprintf("workspace-slot-%s", slotID)
	serviceName := fmt.Sprintf("workspace-slot-%s-svc", slotID)

	// Create Pod with PersistentVolume
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: k.namespace,
			Labels: map[string]string{
				"app":       "workspace",
				"slot-id":   slotID,
				"component": "workspace-pod",
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:            "workspace",
					Image:           k.workspaceImage,
					ImagePullPolicy: corev1.PullNever, // Use local image, don't pull from registry
					Ports: []corev1.ContainerPort{
						{
							Name:          "ttyd",
							ContainerPort: 7681,
							Protocol:      corev1.ProtocolTCP,
						},
						{
							Name:          "preview",
							ContainerPort: 3000,
							Protocol:      corev1.ProtocolTCP,
						},
					},
					Env: []corev1.EnvVar{
						{
							Name:  "SLOT_ID",
							Value: slotID,
						},
						{
							Name:  "PROJECT_ID",
							Value: "", // Will be updated when slot is assigned
						},
						{
							Name:  "SESSION_ID",
							Value: "", // Will be updated when slot is assigned
						},
						{
							Name:  "GIT_REPO_URL",
							Value: "", // Will be updated when slot is assigned
						},
						{
							Name:  "GITHUB_TOKEN",
							Value: "", // Will be updated when slot is assigned
						},
						{
							Name:  "RABBITMQ_URL",
							Value: "", // Will be updated when slot is assigned
						},
						{
							Name:  "TARGET_BRANCH",
							Value: "main", // Default branch to merge into
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "workspace-data",
							MountPath: "/workspace",
						},
					},
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),  // 0.5 CPU cores
							corev1.ResourceMemory: resource.MustParse("256Mi"), // 256 MB RAM
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),  // 0.1 CPU cores
							corev1.ResourceMemory: resource.MustParse("128Mi"), // 128 MB RAM
						},
					},
					LivenessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path: "/",
								Port: intstr.FromInt(7681),
							},
						},
						InitialDelaySeconds: 30,
						PeriodSeconds:       10,
					},
					ReadinessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							HTTPGet: &corev1.HTTPGetAction{
								Path: "/",
								Port: intstr.FromInt(7681),
							},
						},
						InitialDelaySeconds: 5,
						PeriodSeconds:       5,
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "workspace-data",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{}, // For now, use emptyDir. Later: PVC
					},
				},
			},
		},
	}

	// Create the pod
	_, err := k.clientset.CoreV1().Pods(k.namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create workspace pod: %w", err)
	}

	// Create Service for the pod
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: k.namespace,
			Labels: map[string]string{
				"app":       "workspace",
				"slot-id":   slotID,
				"component": "workspace-service",
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"slot-id": slotID,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "ttyd",
					Port:       7681,
					TargetPort: intstr.FromInt(7681),
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "preview",
					Port:       3000,
					TargetPort: intstr.FromInt(3000),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	_, err = k.clientset.CoreV1().Services(k.namespace).Create(ctx, service, metav1.CreateOptions{})
	if err != nil {
		// Clean up pod if service creation fails
		k.clientset.CoreV1().Pods(k.namespace).Delete(ctx, podName, metav1.DeleteOptions{})
		return fmt.Errorf("failed to create workspace service: %w", err)
	}

	// Wait for pod to be ready (with timeout)
	return k.waitForPodReady(ctx, podName, 2*time.Minute)
}

// DeprovisionWorkspace removes the workspace pod and service
func (k *KubernetesProvisioner) DeprovisionWorkspace(ctx context.Context, projectID string) error {
	podName := fmt.Sprintf("workspace-%s", projectID)
	serviceName := fmt.Sprintf("workspace-%s-svc", projectID)

	// Delete service first
	err := k.clientset.CoreV1().Services(k.namespace).Delete(ctx, serviceName, metav1.DeleteOptions{})
	if err != nil {
		log.Printf("Warning: failed to delete service %s: %v", serviceName, err)
	}

	// Delete pod
	err = k.clientset.CoreV1().Pods(k.namespace).Delete(ctx, podName, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete workspace pod: %w", err)
	}

	log.Printf("Successfully deprovisioned workspace for project %s", projectID)
	return nil
}

// waitForPodReady waits for a pod to be in Ready state
func (k *KubernetesProvisioner) waitForPodReady(ctx context.Context, podName string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for pod %s to be ready", podName)
		default:
		}

		pod, err := k.clientset.CoreV1().Pods(k.namespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get pod status: %w", err)
		}

		// Check if pod is ready
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
				log.Printf("Pod %s is ready", podName)
				return nil
			}
		}

		// Check if pod failed
		if pod.Status.Phase == corev1.PodFailed {
			return fmt.Errorf("pod %s failed to start", podName)
		}

		time.Sleep(2 * time.Second)
	}
}

// InitializeSlots creates the 3 fixed workspace slots at startup
func (k *KubernetesProvisioner) InitializeSlots(ctx context.Context) error {
	log.Println("Initializing 3 fixed workspace slots...")

	for i := 1; i <= 3; i++ {
		slotID := fmt.Sprintf("%d", i)
		podName := fmt.Sprintf("workspace-slot-%s", slotID)

		// Check if pod already exists
		_, err := k.clientset.CoreV1().Pods(k.namespace).Get(ctx, podName, metav1.GetOptions{})
		if err == nil {
			log.Printf("Slot %s already exists, skipping creation", slotID)
			continue
		}

		// Create the slot pod
		if err := k.CreateSlotPod(ctx, slotID); err != nil {
			return fmt.Errorf("failed to create slot %s: %w", slotID, err)
		}

		log.Printf("✅ Created slot %s", slotID)
	}

	log.Println("All workspace slots initialized successfully")
	return nil
}

// AssignSlotToProject updates a slot pod with new project details
func (k *KubernetesProvisioner) AssignSlotToProject(ctx context.Context, slotID string, assignment *models.SlotAssignment) error {
	podName := fmt.Sprintf("workspace-slot-%s", slotID)

	// Get the pod
	pod, err := k.clientset.CoreV1().Pods(k.namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get slot pod: %w", err)
	}

	// Update environment variables
	for i := range pod.Spec.Containers[0].Env {
		switch pod.Spec.Containers[0].Env[i].Name {
		case "PROJECT_ID":
			pod.Spec.Containers[0].Env[i].Value = assignment.ProjectID
		case "SESSION_ID":
			pod.Spec.Containers[0].Env[i].Value = assignment.SessionID
		case "GIT_REPO_URL":
			pod.Spec.Containers[0].Env[i].Value = assignment.GitRepoURL
		case "GITHUB_TOKEN":
			pod.Spec.Containers[0].Env[i].Value = assignment.GitHubToken
		case "RABBITMQ_URL":
			pod.Spec.Containers[0].Env[i].Value = assignment.RabbitMQURL
		case "TARGET_BRANCH":
			pod.Spec.Containers[0].Env[i].Value = assignment.TargetBranch
		}
	} // Delete the old pod to restart with new env vars
	err = k.clientset.CoreV1().Pods(k.namespace).Delete(ctx, podName, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete pod for restart: %w", err)
	}

	// Wait for pod to be fully deleted
	for {
		_, err := k.clientset.CoreV1().Pods(k.namespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			// Pod is deleted, we can proceed
			break
		}
		time.Sleep(1 * time.Second)
	}

	// Recreate the pod with updated env vars
	pod.ResourceVersion = ""
	pod.UID = ""
	_, err = k.clientset.CoreV1().Pods(k.namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to recreate pod: %w", err)
	}

	log.Printf("Assigned slot %s to project %s, waiting for pod to be ready...", slotID, assignment.ProjectID)
	
	// Wait for the pod to be ready before returning
	if err := k.waitForPodReady(ctx, podName, 2*time.Minute); err != nil {
		return fmt.Errorf("pod failed to become ready: %w", err)
	}
	
	log.Printf("✅ Slot %s is ready for project %s", slotID, assignment.ProjectID)
	return nil
}

// ReleaseSlot cleans a slot and makes it available for reuse
func (k *KubernetesProvisioner) ReleaseSlot(ctx context.Context, slotID string) error {
	// Reset the slot by restarting with empty env vars
	emptyAssignment := &models.SlotAssignment{
		ProjectID:    "",
		SessionID:    "",
		GitRepoURL:   "",
		GitHubToken:  "",
		RabbitMQURL:  "",
		TargetBranch: "main",
	}
	return k.AssignSlotToProject(ctx, slotID, emptyAssignment)
}

// GetWorkspaceEndpoint returns the service endpoint for a workspace slot
func (k *KubernetesProvisioner) GetWorkspaceEndpoint(ctx context.Context, slotID string) (string, error) {
	serviceName := fmt.Sprintf("workspace-slot-%s-svc", slotID)

	service, err := k.clientset.CoreV1().Services(k.namespace).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get service: %w", err)
	}

	// Return the cluster IP and port
	return fmt.Sprintf("%s.%s.svc.cluster.local:7681", service.Name, service.Namespace), nil
}
