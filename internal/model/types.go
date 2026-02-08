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
	RunStatusCreated          RunStatus = "created"
	RunStatusPlanning         RunStatus = "planning"
	RunStatusRunning          RunStatus = "running"
	RunStatusAwaitingGuidance RunStatus = "awaiting_guidance"
	RunStatusPaused           RunStatus = "paused"
	RunStatusFailed           RunStatus = "failed"
	RunStatusStopping         RunStatus = "stopping"
	RunStatusStopped          RunStatus = "stopped"
	RunStatusClosing          RunStatus = "closing"
	RunStatusClosed           RunStatus = "closed"
	RunStatusComplete         RunStatus = "completed"
)

type RunMode string

const (
	RunModeStandard  RunMode = "run"
	RunModeBootstrap RunMode = "bootstrap"
)

type DocAuthorityMode string

const (
	DocAuthorityModeWorkspaceActive DocAuthorityMode = "workspace_active"
)

type DocSeedMode string

const (
	DocSeedModeNone                DocSeedMode = "none"
	DocSeedModeCopyFromRepoOnStart DocSeedMode = "copy_from_repo_on_start"
)

type DocSyncStatus string

const (
	DocSyncStatusPending DocSyncStatus = "pending"
	DocSyncStatusSynced  DocSyncStatus = "synced"
	DocSyncStatusFailed  DocSyncStatus = "failed"
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
	Name    string   `json:"name"`
	Profile string   `json:"profile,omitempty"`
	Runner  string   `json:"runner,omitempty"`
	Skills  []string `json:"skills,omitempty"`
	Command string   `json:"command"`
}

type RunSpec struct {
	RunID                string            `json:"run_id"`
	Mode                 RunMode           `json:"mode"`
	Tickets              []string          `json:"tickets"`
	Repos                []string          `json:"repos"`
	DocRepo              string            `json:"doc_repo,omitempty"`
	DocHomeRepo          string            `json:"doc_home_repo,omitempty"`
	DocAuthorityMode     DocAuthorityMode  `json:"doc_authority_mode,omitempty"`
	DocSeedMode          DocSeedMode       `json:"doc_seed_mode,omitempty"`
	DocFreshnessRevision string            `json:"doc_freshness_revision,omitempty"`
	BaseBranch           string            `json:"base_branch"`
	WorkspaceStrategy    WorkspaceStrategy `json:"workspace_strategy"`
	Agents               []AgentSpec       `json:"agents"`
	PolicyPath           string            `json:"policy_path"`
	DryRun               bool              `json:"dry_run"`
	CreatedAt            time.Time         `json:"created_at"`
}

type DocSyncState struct {
	RunID            string        `json:"run_id"`
	Ticket           string        `json:"ticket"`
	WorkspaceName    string        `json:"workspace_name"`
	DocHomeRepo      string        `json:"doc_home_repo"`
	DocAuthorityMode string        `json:"doc_authority_mode"`
	DocSeedMode      string        `json:"doc_seed_mode"`
	Status           DocSyncStatus `json:"status"`
	Revision         string        `json:"revision,omitempty"`
	ErrorText        string        `json:"error_text,omitempty"`
	UpdatedAt        time.Time     `json:"updated_at"`
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

type IntakeQA struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

type RunBrief struct {
	RunID        string     `json:"run_id"`
	Ticket       string     `json:"ticket"`
	Goal         string     `json:"goal"`
	Scope        string     `json:"scope"`
	DoneCriteria string     `json:"done_criteria"`
	Constraints  string     `json:"constraints"`
	MergeIntent  string     `json:"merge_intent"`
	QA           []IntakeQA `json:"qa"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type GuidanceStatus string

const (
	GuidanceStatusPending  GuidanceStatus = "pending"
	GuidanceStatusAnswered GuidanceStatus = "answered"
)

type GuidanceRequest struct {
	ID            int64          `json:"id"`
	RunID         string         `json:"run_id"`
	WorkspaceName string         `json:"workspace_name"`
	AgentName     string         `json:"agent_name"`
	Question      string         `json:"question"`
	Context       string         `json:"context,omitempty"`
	Answer        string         `json:"answer,omitempty"`
	Status        GuidanceStatus `json:"status"`
	CreatedAt     time.Time      `json:"created_at"`
	AnsweredAt    *time.Time     `json:"answered_at,omitempty"`
}

type GuidanceRequestPayload struct {
	RunID    string `json:"run_id,omitempty"`
	Agent    string `json:"agent,omitempty"`
	Question string `json:"question"`
	Context  string `json:"context,omitempty"`
}

type GuidanceResponsePayload struct {
	GuidanceID int64  `json:"guidance_id"`
	RunID      string `json:"run_id"`
	Agent      string `json:"agent,omitempty"`
	Question   string `json:"question"`
	Answer     string `json:"answer"`
	AnsweredAt string `json:"answered_at"`
}

type CompletionSignalPayload struct {
	RunID   string `json:"run_id,omitempty"`
	Agent   string `json:"agent,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type OperatorRunState struct {
	RunID           string     `json:"run_id"`
	RestartAttempts int        `json:"restart_attempts"`
	LastRestartAt   *time.Time `json:"last_restart_at,omitempty"`
	CooldownUntil   *time.Time `json:"cooldown_until,omitempty"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type PullRequestState string

const (
	PullRequestStateOpen   PullRequestState = "open"
	PullRequestStateClosed PullRequestState = "closed"
	PullRequestStateMerged PullRequestState = "merged"
	PullRequestStateDraft  PullRequestState = "draft"
)

type RunPullRequest struct {
	RunID          string           `json:"run_id"`
	Ticket         string           `json:"ticket"`
	Repo           string           `json:"repo"`
	WorkspaceName  string           `json:"workspace_name,omitempty"`
	HeadBranch     string           `json:"head_branch,omitempty"`
	BaseBranch     string           `json:"base_branch,omitempty"`
	RemoteName     string           `json:"remote_name,omitempty"`
	CommitSHA      string           `json:"commit_sha,omitempty"`
	PRNumber       int              `json:"pr_number,omitempty"`
	PRURL          string           `json:"pr_url,omitempty"`
	PRState        PullRequestState `json:"pr_state,omitempty"`
	CredentialMode string           `json:"credential_mode,omitempty"`
	Actor          string           `json:"actor,omitempty"`
	ValidationJSON string           `json:"validation_json,omitempty"`
	ErrorText      string           `json:"error_text,omitempty"`
	CreatedAt      time.Time        `json:"created_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
}
