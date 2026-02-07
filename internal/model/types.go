package model

import "time"

type WorkspaceStrategy string

const (
	WorkspaceStrategyCreate WorkspaceStrategy = "create"
	WorkspaceStrategyFork   WorkspaceStrategy = "fork"
	WorkspaceStrategyReuse  WorkspaceStrategy = "reuse"
)

type RunStatus string

const (
	RunStatusCreated  RunStatus = "created"
	RunStatusPlanning RunStatus = "planning"
	RunStatusRunning  RunStatus = "running"
	RunStatusPaused   RunStatus = "paused"
	RunStatusFailed   RunStatus = "failed"
	RunStatusStopping RunStatus = "stopping"
	RunStatusStopped  RunStatus = "stopped"
	RunStatusClosing  RunStatus = "closing"
	RunStatusClosed   RunStatus = "closed"
	RunStatusComplete RunStatus = "completed"
)

type StepStatus string

const (
	StepStatusPending StepStatus = "pending"
	StepStatusRunning StepStatus = "running"
	StepStatusDone    StepStatus = "done"
	StepStatusFailed  StepStatus = "failed"
	StepStatusSkipped StepStatus = "skipped"
)

type AgentStatus string

const (
	AgentStatusPending  AgentStatus = "pending"
	AgentStatusRunning  AgentStatus = "running"
	AgentStatusIdle     AgentStatus = "idle"
	AgentStatusStalled  AgentStatus = "stalled"
	AgentStatusDead     AgentStatus = "dead"
	AgentStatusStopping AgentStatus = "stopping"
	AgentStatusStopped  AgentStatus = "stopped"
	AgentStatusFailed   AgentStatus = "failed"
)

type HealthState string

const (
	HealthStateHealthy HealthState = "healthy"
	HealthStateIdle    HealthState = "idle"
	HealthStateStalled HealthState = "stalled"
	HealthStateDead    HealthState = "dead"
)

type AgentSpec struct {
	Name    string `json:"name"`
	Command string `json:"command"`
}

type RunSpec struct {
	RunID             string            `json:"run_id"`
	Tickets           []string          `json:"tickets"`
	Repos             []string          `json:"repos"`
	WorkspaceStrategy WorkspaceStrategy `json:"workspace_strategy"`
	Agents            []AgentSpec       `json:"agents"`
	PolicyPath        string            `json:"policy_path"`
	DryRun            bool              `json:"dry_run"`
	CreatedAt         time.Time         `json:"created_at"`
}

type PlanStep struct {
	Index         int        `json:"index"`
	Name          string     `json:"name"`
	Kind          string     `json:"kind"`
	Command       string     `json:"command"`
	Blocking      bool       `json:"blocking"`
	Ticket        string     `json:"ticket,omitempty"`
	WorkspaceName string     `json:"workspace_name,omitempty"`
	Agent         string     `json:"agent,omitempty"`
	Status        StepStatus `json:"status"`
}

type RunRecord struct {
	RunID     string    `json:"run_id"`
	Status    RunStatus `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	ErrorText string    `json:"error_text,omitempty"`
}

type StepRecord struct {
	RunID         string     `json:"run_id"`
	Index         int        `json:"index"`
	Name          string     `json:"name"`
	Kind          string     `json:"kind"`
	Command       string     `json:"command"`
	Blocking      bool       `json:"blocking"`
	Ticket        string     `json:"ticket,omitempty"`
	WorkspaceName string     `json:"workspace_name,omitempty"`
	Agent         string     `json:"agent,omitempty"`
	Status        StepStatus `json:"status"`
	ErrorText     string     `json:"error_text,omitempty"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
}

type AgentRecord struct {
	RunID          string      `json:"run_id"`
	Name           string      `json:"name"`
	WorkspaceName  string      `json:"workspace_name"`
	SessionName    string      `json:"session_name"`
	Status         AgentStatus `json:"status"`
	HealthState    HealthState `json:"health_state"`
	LastActivityAt *time.Time  `json:"last_activity_at,omitempty"`
	LastProgressAt *time.Time  `json:"last_progress_at,omitempty"`
}
