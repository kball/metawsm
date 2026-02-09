package orchestrator

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"metawsm/internal/model"
	"metawsm/internal/policy"
)

type RunGuidanceSnapshot struct {
	ThreadID      string
	AgentName     string
	WorkspaceName string
	Question      string
}

type RunUnhealthyAgentSnapshot struct {
	AgentName     string
	WorkspaceName string
	SessionName   string
	Status        model.AgentStatus
	Health        model.HealthState
	LastActivity  string
	LastProgress  string
	ActivityAge   string
	ProgressAge   string
}

type RunSnapshot struct {
	RunID                string
	Status               model.RunStatus
	Tickets              []string
	PendingGuidance      []RunGuidanceSnapshot
	UnhealthyAgents      []RunUnhealthyAgentSnapshot
	HasDirtyDiffs        bool
	DraftPullRequests    int
	OpenPullRequests     int
	QueuedReviewFeedback int
	NewReviewFeedback    int
}

func (s *Service) RunSnapshot(ctx context.Context, runID string) (RunSnapshot, error) {
	record, specJSON, _, err := s.store.GetRun(runID)
	if err != nil {
		return RunSnapshot{}, err
	}
	var spec model.RunSpec
	if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
		spec = model.RunSpec{}
	}
	agents, err := s.store.GetAgents(runID)
	if err != nil {
		return RunSnapshot{}, err
	}
	tickets, err := s.store.GetTickets(runID)
	if err != nil {
		return RunSnapshot{}, err
	}

	cfg, _, err := policy.Load("")
	if err != nil {
		cfg = policy.Default()
	}
	now := time.Now()
	for _, agent := range agents {
		health, status, lastActivity, lastProgress := evaluateHealth(ctx, cfg, agent, now)
		_ = s.store.UpdateAgentStatus(runID, agent.Name, agent.WorkspaceName, status, health, lastActivity, lastProgress)
	}
	agents, _ = s.store.GetAgents(runID)
	if spec.Mode == model.RunModeBootstrap {
		if err := s.syncBootstrapSignals(ctx, runID, record.Status, spec, agents); err != nil {
			return RunSnapshot{}, err
		}
		record, _, _, _ = s.store.GetRun(runID)
	}

	workspaceDiffs := collectWorkspaceDiffs(ctx, workspaceNamesFromAgents(agents), spec.Repos)
	progressByWorkspace := latestProgressFromWorkspaceDiffs(workspaceDiffs)
	for i := range agents {
		progressAt, ok := progressByWorkspace[agents[i].WorkspaceName]
		if !ok {
			continue
		}
		if agents[i].LastProgressAt != nil && !progressAt.After(*agents[i].LastProgressAt) {
			continue
		}
		progressCopy := progressAt
		agents[i].LastProgressAt = &progressCopy
		_ = s.store.UpdateAgentStatus(
			runID,
			agents[i].Name,
			agents[i].WorkspaceName,
			agents[i].Status,
			agents[i].HealthState,
			agents[i].LastActivityAt,
			agents[i].LastProgressAt,
		)
	}

	controlStates, err := s.forumControlStatesForRun(runID, agents)
	if err != nil {
		return RunSnapshot{}, err
	}
	pendingGuidance := make([]RunGuidanceSnapshot, 0, len(agents))
	for _, agent := range agents {
		state, ok := controlStates[agent.Name]
		if !ok || !state.PendingGuidance {
			continue
		}
		pendingGuidance = append(pendingGuidance, RunGuidanceSnapshot{
			ThreadID:      strings.TrimSpace(state.ThreadID),
			AgentName:     strings.TrimSpace(agent.Name),
			WorkspaceName: strings.TrimSpace(agent.WorkspaceName),
			Question:      strings.TrimSpace(state.PendingGuidanceQuestion),
		})
	}

	hasDirtyDiffs := false
	for _, diff := range workspaceDiffs {
		if diff.Error != nil {
			continue
		}
		for _, repo := range diff.Repos {
			if repo.Error != nil {
				continue
			}
			if len(repo.StatusLines) > 0 {
				hasDirtyDiffs = true
				break
			}
		}
		if hasDirtyDiffs {
			break
		}
	}

	runPullRequests, err := s.store.ListRunPullRequests(runID)
	if err != nil {
		return RunSnapshot{}, err
	}
	draftPullRequests := 0
	openPullRequests := 0
	for _, item := range runPullRequests {
		switch strings.TrimSpace(strings.ToLower(string(item.PRState))) {
		case "draft":
			draftPullRequests++
		case "open":
			openPullRequests++
		}
	}

	runReviewFeedback, err := s.store.ListRunReviewFeedback(runID)
	if err != nil {
		return RunSnapshot{}, err
	}
	queuedReviewFeedback := 0
	newReviewFeedback := 0
	for _, item := range runReviewFeedback {
		switch item.Status {
		case model.ReviewFeedbackStatusQueued:
			queuedReviewFeedback++
		case model.ReviewFeedbackStatusNew:
			newReviewFeedback++
		}
	}

	unhealthyAgents := make([]RunUnhealthyAgentSnapshot, 0, len(agents))
	for _, agent := range agents {
		if !isUnhealthySnapshotAgent(agent.Status, agent.HealthState) {
			continue
		}
		unhealthyAgents = append(unhealthyAgents, RunUnhealthyAgentSnapshot{
			AgentName:     strings.TrimSpace(agent.Name),
			WorkspaceName: strings.TrimSpace(agent.WorkspaceName),
			SessionName:   strings.TrimSpace(agent.SessionName),
			Status:        agent.Status,
			Health:        agent.HealthState,
			LastActivity:  formatTimeOrDash(agent.LastActivityAt),
			LastProgress:  formatTimeOrDash(agent.LastProgressAt),
			ActivityAge:   formatAgeOrDash(now, agent.LastActivityAt),
			ProgressAge:   formatAgeOrDash(now, agent.LastProgressAt),
		})
	}

	return RunSnapshot{
		RunID:                record.RunID,
		Status:               record.Status,
		Tickets:              tickets,
		PendingGuidance:      pendingGuidance,
		UnhealthyAgents:      unhealthyAgents,
		HasDirtyDiffs:        hasDirtyDiffs,
		DraftPullRequests:    draftPullRequests,
		OpenPullRequests:     openPullRequests,
		QueuedReviewFeedback: queuedReviewFeedback,
		NewReviewFeedback:    newReviewFeedback,
	}, nil
}

func isUnhealthySnapshotAgent(status model.AgentStatus, health model.HealthState) bool {
	return health == model.HealthStateDead ||
		health == model.HealthStateStalled ||
		status == model.AgentStatusFailed ||
		status == model.AgentStatusDead
}
