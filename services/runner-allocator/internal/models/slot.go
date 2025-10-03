package models

import (
	"time"
)

type RunnerSlot struct {
	SlotID     string    `json:"slot_id"`
	UserID     string    `json:"user_id"`
	ProjectID  string    `json:"project_id"`
	SessionID  string    `json:"session_id"`
	PodIP      string    `json:"pod_ip"`
	Status     string    `json:"status"` // "idle", "active", "building"
	CreatedAt  time.Time `json:"created_at"`
	Template   string    `json:"template"` // "react-vite", "nodejs-express", "blank"
	GitHubRepo string    `json:"github_repo,omitempty"`
}

type QueuedUser struct {
	UserID     string    `json:"user_id"`
	ProjectID  string    `json:"project_id"`
	Template   string    `json:"template"`
	GitHubRepo string    `json:"github_repo,omitempty"`
	QueuedAt   time.Time `json:"queued_at"`
}
