package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"metawsm/internal/hsm"
	"metawsm/internal/model"
	"metawsm/internal/policy"
	"metawsm/internal/store"
)

type Service struct {
	store *store.SQLiteStore
}

func NewService(dbPath string) (*Service, error) {
	sqliteStore := store.NewSQLiteStore(dbPath)
	if err := sqliteStore.Init(); err != nil {
		return nil, err
	}
	return &Service{store: sqliteStore}, nil
}

type RunOptions struct {
	RunID             string
	Tickets           []string
	Repos             []string
	BaseBranch        string
	AgentNames        []string
	WorkspaceStrategy model.WorkspaceStrategy
	PolicyPath        string
	DryRun            bool
	Mode              model.RunMode
	RunBrief          *model.RunBrief
}

type CloseOptions struct {
	RunID          string
	DryRun         bool
	ChangelogEntry string
}

type RestartOptions struct {
	Ticket string
	RunID  string
	DryRun bool
}

type CleanupOptions struct {
	Ticket           string
	RunID            string
	DryRun           bool
	DeleteWorkspaces bool
}

type RunResult struct {
	RunID string
	Steps []model.PlanStep
}

type GuideResult struct {
	RunID         string
	GuidanceID    int64
	WorkspaceName string
	AgentName     string
	Question      string
}

type RestartResult struct {
	RunID   string
	Actions []string
}

type CleanupResult struct {
	RunID   string
	Actions []string
}

func (s *Service) Run(ctx context.Context, options RunOptions) (RunResult, error) {
	cfg, policyPath, err := policy.Load(options.PolicyPath)
	if err != nil {
		return RunResult{}, err
	}

	tickets := normalizeTokens(options.Tickets)
	if len(tickets) == 0 {
		return RunResult{}, fmt.Errorf("at least one --ticket is required")
	}
	repos := normalizeTokens(options.Repos)
	if len(repos) == 0 && options.WorkspaceStrategy != model.WorkspaceStrategyReuse {
		return RunResult{}, fmt.Errorf("at least one --repos entry is required for create/fork")
	}

	strategy := options.WorkspaceStrategy
	if strategy == "" {
		strategy = model.WorkspaceStrategy(cfg.Workspace.DefaultStrategy)
	}
	baseBranch := normalizeBaseBranch(options.BaseBranch)
	if baseBranch == "" {
		baseBranch = normalizeBaseBranch(cfg.Workspace.BaseBranch)
	}
	if baseBranch == "" {
		baseBranch = "main"
	}

	agents, err := policy.ResolveAgents(cfg, normalizeTokens(options.AgentNames))
	if err != nil {
		return RunResult{}, err
	}

	runID := strings.TrimSpace(options.RunID)
	if runID == "" {
		runID = generateRunID()
	}
	mode := options.Mode
	if mode == "" {
		mode = model.RunModeStandard
	}

	spec := model.RunSpec{
		RunID:             runID,
		Mode:              mode,
		Tickets:           tickets,
		Repos:             repos,
		BaseBranch:        baseBranch,
		WorkspaceStrategy: strategy,
		Agents:            agents,
		PolicyPath:        policyPath,
		DryRun:            options.DryRun,
		CreatedAt:         time.Now(),
	}

	policyJSON, err := json.Marshal(cfg)
	if err != nil {
		return RunResult{}, fmt.Errorf("marshal policy: %w", err)
	}
	if err := s.store.CreateRun(spec, string(policyJSON)); err != nil {
		return RunResult{}, err
	}
	if options.RunBrief != nil {
		brief := *options.RunBrief
		brief.RunID = spec.RunID
		if strings.TrimSpace(brief.Ticket) == "" && len(tickets) > 0 {
			brief.Ticket = tickets[0]
		}
		now := time.Now()
		if brief.CreatedAt.IsZero() {
			brief.CreatedAt = now
		}
		brief.UpdatedAt = now
		if err := s.store.UpsertRunBrief(brief); err != nil {
			return RunResult{}, err
		}
	}

	if err := s.transitionRun(spec.RunID, model.RunStatusCreated, model.RunStatusPlanning, "planning run"); err != nil {
		return RunResult{}, err
	}

	steps := buildPlan(spec, cfg)
	if err := s.store.SaveSteps(spec.RunID, steps); err != nil {
		return RunResult{}, err
	}
	if err := s.seedAgents(spec.RunID, steps); err != nil {
		return RunResult{}, err
	}

	if spec.DryRun {
		if err := s.transitionRun(spec.RunID, model.RunStatusPlanning, model.RunStatusPaused, "dry-run complete"); err != nil {
			return RunResult{}, err
		}
		return RunResult{RunID: spec.RunID, Steps: steps}, nil
	}

	if err := s.transitionRun(spec.RunID, model.RunStatusPlanning, model.RunStatusRunning, "executing plan"); err != nil {
		return RunResult{}, err
	}
	if err := s.executeSteps(ctx, spec, cfg, steps); err != nil {
		_ = s.transitionRun(spec.RunID, model.RunStatusRunning, model.RunStatusFailed, err.Error())
		return RunResult{}, err
	}
	if spec.Mode == model.RunModeBootstrap {
		_ = s.store.AddEvent(spec.RunID, "run", spec.RunID, "bootstrap", string(model.RunStatusRunning), string(model.RunStatusRunning), "bootstrap setup complete; monitoring for guidance/completion signals")
		return RunResult{RunID: spec.RunID, Steps: steps}, nil
	}
	if err := s.transitionRun(spec.RunID, model.RunStatusRunning, model.RunStatusComplete, "run completed"); err != nil {
		return RunResult{}, err
	}

	return RunResult{RunID: spec.RunID, Steps: steps}, nil
}

func (s *Service) Resume(ctx context.Context, runID string) error {
	record, specJSON, policyJSON, err := s.store.GetRun(runID)
	if err != nil {
		return err
	}
	if !hsm.CanTransitionRun(record.Status, model.RunStatusRunning) {
		return fmt.Errorf("run %s cannot transition from %s to %s", runID, record.Status, model.RunStatusRunning)
	}

	var spec model.RunSpec
	if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
		return fmt.Errorf("unmarshal run spec: %w", err)
	}
	var cfg policy.Config
	if err := json.Unmarshal([]byte(policyJSON), &cfg); err != nil {
		return fmt.Errorf("unmarshal policy: %w", err)
	}

	if err := s.transitionRun(runID, record.Status, model.RunStatusRunning, "resume requested"); err != nil {
		return err
	}

	steps, err := s.store.GetSteps(runID)
	if err != nil {
		return err
	}
	planSteps := make([]model.PlanStep, 0, len(steps))
	for _, step := range steps {
		planSteps = append(planSteps, model.PlanStep{
			Index:         step.Index,
			Name:          step.Name,
			Kind:          step.Kind,
			Command:       step.Command,
			Blocking:      step.Blocking,
			Ticket:        step.Ticket,
			WorkspaceName: step.WorkspaceName,
			Agent:         step.Agent,
			Status:        step.Status,
		})
	}

	if err := s.executeSteps(ctx, spec, cfg, planSteps); err != nil {
		_ = s.transitionRun(runID, model.RunStatusRunning, model.RunStatusFailed, err.Error())
		return err
	}
	if spec.Mode == model.RunModeBootstrap {
		_ = s.store.AddEvent(runID, "run", runID, "bootstrap", string(model.RunStatusRunning), string(model.RunStatusRunning), "resume completed; monitoring for guidance/completion signals")
		return nil
	}
	return s.transitionRun(runID, model.RunStatusRunning, model.RunStatusComplete, "resume completed")
}

func (s *Service) Stop(ctx context.Context, runID string) error {
	record, _, _, err := s.store.GetRun(runID)
	if err != nil {
		return err
	}
	if !hsm.CanTransitionRun(record.Status, model.RunStatusStopping) {
		return fmt.Errorf("run %s cannot transition from %s to %s", runID, record.Status, model.RunStatusStopping)
	}

	if err := s.transitionRun(runID, record.Status, model.RunStatusStopping, "stop requested"); err != nil {
		return err
	}
	agents, err := s.store.GetAgents(runID)
	if err != nil {
		return err
	}
	now := time.Now()
	for _, agent := range agents {
		_ = tmuxKillSession(ctx, agent.SessionName)
		_ = s.store.UpdateAgentStatus(runID, agent.Name, agent.WorkspaceName, model.AgentStatusStopped, model.HealthStateDead, &now, agent.LastProgressAt)
	}
	return s.transitionRun(runID, model.RunStatusStopping, model.RunStatusStopped, "run stopped")
}

func (s *Service) Restart(ctx context.Context, options RestartOptions) (RestartResult, error) {
	runID, err := s.resolveRunID(options.RunID, options.Ticket)
	if err != nil {
		return RestartResult{}, err
	}
	record, specJSON, _, err := s.store.GetRun(runID)
	if err != nil {
		return RestartResult{}, err
	}
	if record.Status == model.RunStatusClosed {
		return RestartResult{}, fmt.Errorf("run %s is closed and cannot be restarted", runID)
	}

	var spec model.RunSpec
	if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
		return RestartResult{}, fmt.Errorf("unmarshal run spec: %w", err)
	}
	agentCommand := map[string]string{}
	for _, agent := range spec.Agents {
		agentCommand[agent.Name] = agent.Command
	}

	agents, err := s.store.GetAgents(runID)
	if err != nil {
		return RestartResult{}, err
	}
	if len(agents) == 0 {
		return RestartResult{}, fmt.Errorf("run %s has no agents to restart", runID)
	}

	if record.Status != model.RunStatusRunning && hsm.CanTransitionRun(record.Status, model.RunStatusRunning) {
		if !options.DryRun {
			if err := s.transitionRun(runID, record.Status, model.RunStatusRunning, "restart requested"); err != nil {
				return RestartResult{}, err
			}
		}
	}

	actions := make([]string, 0, len(agents)*2)
	now := time.Now()
	for _, agent := range agents {
		workspacePath, err := resolveWorkspacePath(agent.WorkspaceName)
		if err != nil {
			return RestartResult{}, err
		}
		command := strings.TrimSpace(agentCommand[agent.Name])
		if command == "" {
			command = "bash"
		}
		sessionName := strings.TrimSpace(agent.SessionName)
		if sessionName == "" {
			sessionName = policy.RenderSessionName("{agent}-{workspace}", agent.Name, agent.WorkspaceName)
		}

		killCmd := fmt.Sprintf("tmux kill-session -t %s", shellQuote(sessionName))
		startCmd := fmt.Sprintf("tmux new-session -d -s %s -c %s %s", shellQuote(sessionName), shellQuote(workspacePath), shellQuote(command))
		actions = append(actions, killCmd, startCmd)

		if options.DryRun {
			continue
		}
		_ = tmuxKillSession(ctx, sessionName)
		if err := runShell(ctx, startCmd); err != nil {
			return RestartResult{}, err
		}
		if err := s.store.UpdateAgentStatus(runID, agent.Name, agent.WorkspaceName, model.AgentStatusRunning, model.HealthStateHealthy, &now, &now); err != nil {
			return RestartResult{}, err
		}
	}
	if !options.DryRun {
		_ = s.store.AddEvent(runID, "run", runID, "restart", "", "", fmt.Sprintf("restarted %d agents", len(agents)))
	}
	return RestartResult{RunID: runID, Actions: actions}, nil
}

func (s *Service) Cleanup(ctx context.Context, options CleanupOptions) (CleanupResult, error) {
	runID, err := s.resolveRunID(options.RunID, options.Ticket)
	if err != nil {
		return CleanupResult{}, err
	}

	record, _, _, err := s.store.GetRun(runID)
	if err != nil {
		return CleanupResult{}, err
	}
	agents, err := s.store.GetAgents(runID)
	if err != nil {
		return CleanupResult{}, err
	}

	actions := make([]string, 0, len(agents)*2)
	workspaceSet := map[string]struct{}{}
	for _, agent := range agents {
		sessionName := strings.TrimSpace(agent.SessionName)
		if sessionName == "" {
			sessionName = policy.RenderSessionName("{agent}-{workspace}", agent.Name, agent.WorkspaceName)
		}
		actions = append(actions, fmt.Sprintf("tmux kill-session -t %s", shellQuote(sessionName)))
		workspaceSet[agent.WorkspaceName] = struct{}{}
	}
	workspaces := make([]string, 0, len(workspaceSet))
	for workspaceName := range workspaceSet {
		if strings.TrimSpace(workspaceName) != "" {
			workspaces = append(workspaces, workspaceName)
		}
	}
	sort.Strings(workspaces)

	if options.DeleteWorkspaces {
		for _, workspaceName := range workspaces {
			actions = append(actions, fmt.Sprintf("wsm delete %s", shellQuote(workspaceName)))
		}
	}

	if options.DryRun {
		return CleanupResult{RunID: runID, Actions: actions}, nil
	}

	for _, agent := range agents {
		sessionName := strings.TrimSpace(agent.SessionName)
		if sessionName == "" {
			sessionName = policy.RenderSessionName("{agent}-{workspace}", agent.Name, agent.WorkspaceName)
		}
		_ = tmuxKillSession(ctx, sessionName)
	}

	now := time.Now()
	for _, agent := range agents {
		_ = s.store.UpdateAgentStatus(runID, agent.Name, agent.WorkspaceName, model.AgentStatusStopped, model.HealthStateDead, &now, agent.LastProgressAt)
	}

	for _, workspaceName := range workspaces {
		if options.DeleteWorkspaces {
			if err := deleteWorkspaceIfPresent(ctx, workspaceName); err != nil {
				return CleanupResult{}, err
			}
		}
	}

	if record.Status != model.RunStatusStopped && hsm.CanTransitionRun(record.Status, model.RunStatusStopping) {
		if err := s.transitionRun(runID, record.Status, model.RunStatusStopping, "cleanup requested"); err != nil {
			return CleanupResult{}, err
		}
		if err := s.transitionRun(runID, model.RunStatusStopping, model.RunStatusStopped, "cleanup completed"); err != nil {
			return CleanupResult{}, err
		}
	}
	_ = s.store.AddEvent(runID, "run", runID, "cleanup", "", "", fmt.Sprintf("cleanup complete; workspaces=%d", len(workspaces)))
	return CleanupResult{RunID: runID, Actions: actions}, nil
}

func (s *Service) resolveRunID(explicitRunID string, ticket string) (string, error) {
	runID := strings.TrimSpace(explicitRunID)
	if runID != "" {
		return runID, nil
	}
	ticket = strings.TrimSpace(ticket)
	if ticket == "" {
		return "", fmt.Errorf("either run id or ticket is required")
	}
	runIDs, err := s.store.ListRunIDsByTicket(ticket)
	if err != nil {
		return "", err
	}
	if len(runIDs) == 0 {
		return "", fmt.Errorf("no run found for ticket %s", ticket)
	}
	for _, candidate := range runIDs {
		_, specJSON, _, err := s.store.GetRun(candidate)
		if err != nil {
			continue
		}
		var spec model.RunSpec
		if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
			continue
		}
		if !spec.DryRun {
			return candidate, nil
		}
	}
	return runIDs[0], nil
}

func (s *Service) Guide(ctx context.Context, runID string, answer string) (GuideResult, error) {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return GuideResult{}, fmt.Errorf("guidance answer cannot be empty")
	}

	record, _, _, err := s.store.GetRun(runID)
	if err != nil {
		return GuideResult{}, err
	}
	if record.Status != model.RunStatusAwaitingGuidance {
		return GuideResult{}, fmt.Errorf("run %s is not waiting for guidance (current: %s)", runID, record.Status)
	}

	pending, err := s.store.ListGuidanceRequests(runID, model.GuidanceStatusPending)
	if err != nil {
		return GuideResult{}, err
	}
	if len(pending) == 0 {
		return GuideResult{}, fmt.Errorf("run %s has no pending guidance requests", runID)
	}

	req := pending[0]
	workspacePath, err := resolveWorkspacePath(req.WorkspaceName)
	if err != nil {
		return GuideResult{}, err
	}
	responsePath := filepath.Join(workspacePath, ".metawsm", "guidance-response.json")
	if err := os.MkdirAll(filepath.Dir(responsePath), 0o755); err != nil {
		return GuideResult{}, err
	}
	answeredAt := time.Now().Format(time.RFC3339)
	payload := model.GuidanceResponsePayload{
		GuidanceID: req.ID,
		RunID:      runID,
		Agent:      req.AgentName,
		Question:   req.Question,
		Answer:     answer,
		AnsweredAt: answeredAt,
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return GuideResult{}, err
	}
	if err := os.WriteFile(responsePath, append(encoded, '\n'), 0o644); err != nil {
		return GuideResult{}, err
	}

	requestPath := filepath.Join(workspacePath, ".metawsm", "guidance-request.json")
	_ = os.Remove(requestPath)

	if err := s.store.MarkGuidanceAnswered(req.ID, answer); err != nil {
		return GuideResult{}, err
	}
	_ = s.store.AddEvent(runID, "guidance", fmt.Sprintf("%d", req.ID), "answered", string(model.GuidanceStatusPending), string(model.GuidanceStatusAnswered), fmt.Sprintf("%s@%s", req.AgentName, req.WorkspaceName))

	if err := s.transitionRun(runID, model.RunStatusAwaitingGuidance, model.RunStatusRunning, "guidance answered"); err != nil {
		return GuideResult{}, err
	}
	return GuideResult{
		RunID:         runID,
		GuidanceID:    req.ID,
		WorkspaceName: req.WorkspaceName,
		AgentName:     req.AgentName,
		Question:      req.Question,
	}, nil
}

func (s *Service) Close(ctx context.Context, options CloseOptions) error {
	record, specJSON, _, err := s.store.GetRun(options.RunID)
	if err != nil {
		return err
	}
	var spec model.RunSpec
	if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
		return fmt.Errorf("unmarshal run spec: %w", err)
	}

	if record.Status != model.RunStatusComplete && record.Status != model.RunStatusClosed {
		return fmt.Errorf("run %s must be in completed state before close (current: %s)", options.RunID, record.Status)
	}
	if record.Status == model.RunStatusClosed {
		return nil
	}

	agents, err := s.store.GetAgents(options.RunID)
	if err != nil {
		return err
	}
	tickets, err := s.store.GetTickets(options.RunID)
	if err != nil {
		return err
	}

	workspaceSet := map[string]struct{}{}
	for _, agent := range agents {
		workspaceSet[agent.WorkspaceName] = struct{}{}
	}
	workspaces := make([]string, 0, len(workspaceSet))
	for workspaceName := range workspaceSet {
		workspaces = append(workspaces, workspaceName)
	}
	sort.Strings(workspaces)

	for _, workspaceName := range workspaces {
		path, err := resolveWorkspacePath(workspaceName)
		if err != nil {
			return err
		}
		dirty, err := hasDirtyGitState(ctx, path)
		if err != nil {
			return err
		}
		if dirty {
			return fmt.Errorf("workspace %s (%s) has uncommitted changes; close blocked", workspaceName, path)
		}
	}
	if spec.Mode == model.RunModeBootstrap {
		if err := s.ensureBootstrapCloseChecks(options.RunID, workspaces); err != nil {
			return err
		}
	}

	if err := s.transitionRun(options.RunID, model.RunStatusComplete, model.RunStatusClosing, "close started"); err != nil {
		return err
	}

	for _, workspaceName := range workspaces {
		command := fmt.Sprintf("wsm merge %s", shellQuote(workspaceName))
		if options.DryRun {
			fmt.Println("[dry-run]", command)
			continue
		}
		if err := runShell(ctx, command); err != nil {
			_ = s.transitionRun(options.RunID, model.RunStatusClosing, model.RunStatusFailed, err.Error())
			return err
		}
	}

	entry := options.ChangelogEntry
	if strings.TrimSpace(entry) == "" {
		entry = fmt.Sprintf("Closed by metawsm run %s", options.RunID)
	}
	for _, ticket := range tickets {
		command := fmt.Sprintf("docmgr ticket close --ticket %s --changelog-entry %s", shellQuote(ticket), shellQuote(entry))
		if options.DryRun {
			fmt.Println("[dry-run]", command)
			continue
		}
		if err := runShell(ctx, command); err != nil {
			_ = s.transitionRun(options.RunID, model.RunStatusClosing, model.RunStatusFailed, err.Error())
			return err
		}
	}

	return s.transitionRun(options.RunID, model.RunStatusClosing, model.RunStatusClosed, "close completed")
}

func (s *Service) Status(ctx context.Context, runID string) (string, error) {
	record, specJSON, _, err := s.store.GetRun(runID)
	if err != nil {
		return "", err
	}
	var spec model.RunSpec
	if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
		spec = model.RunSpec{}
	}
	steps, err := s.store.GetSteps(runID)
	if err != nil {
		return "", err
	}
	agents, err := s.store.GetAgents(runID)
	if err != nil {
		return "", err
	}
	tickets, err := s.store.GetTickets(runID)
	if err != nil {
		return "", err
	}

	cfg, _, err := policy.Load("")
	if err != nil {
		cfg = policy.Default()
	}

	now := time.Now()
	for _, agent := range agents {
		health, status, lastActivity := evaluateHealth(ctx, cfg, agent, now)
		lastProgress := agent.LastProgressAt
		if status == model.AgentStatusRunning || status == model.AgentStatusIdle || status == model.AgentStatusStalled {
			lastProgress = &now
		}
		_ = s.store.UpdateAgentStatus(runID, agent.Name, agent.WorkspaceName, status, health, lastActivity, lastProgress)
	}
	agents, _ = s.store.GetAgents(runID)
	if spec.Mode == model.RunModeBootstrap {
		if err := s.syncBootstrapSignals(ctx, runID, record.Status, agents); err != nil {
			return "", err
		}
		record, _, _, _ = s.store.GetRun(runID)
	}
	pendingGuidance, _ := s.store.ListGuidanceRequests(runID, model.GuidanceStatusPending)
	brief, _ := s.store.GetRunBrief(runID)

	var doneCount, failedCount, runningCount, pendingCount int
	for _, step := range steps {
		switch step.Status {
		case model.StepStatusDone:
			doneCount++
		case model.StepStatusFailed:
			failedCount++
		case model.StepStatusRunning:
			runningCount++
		default:
			pendingCount++
		}
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Run: %s\n", runID))
	b.WriteString(fmt.Sprintf("Status: %s\n", record.Status))
	if spec.Mode != "" {
		b.WriteString(fmt.Sprintf("Mode: %s\n", spec.Mode))
	}
	b.WriteString(fmt.Sprintf("Tickets: %s\n", strings.Join(tickets, ", ")))
	if brief != nil {
		b.WriteString("Brief:\n")
		b.WriteString(fmt.Sprintf("  goal=%s\n", brief.Goal))
		b.WriteString(fmt.Sprintf("  scope=%s\n", brief.Scope))
		b.WriteString(fmt.Sprintf("  done=%s\n", brief.DoneCriteria))
		b.WriteString(fmt.Sprintf("  constraints=%s\n", brief.Constraints))
		b.WriteString(fmt.Sprintf("  merge_intent=%s\n", brief.MergeIntent))
	}
	if len(pendingGuidance) > 0 {
		b.WriteString("Guidance:\n")
		for _, item := range pendingGuidance {
			b.WriteString(fmt.Sprintf("  - id=%d %s@%s question=%s\n", item.ID, item.AgentName, item.WorkspaceName, item.Question))
		}
	}
	b.WriteString(fmt.Sprintf("Steps: total=%d done=%d running=%d pending=%d failed=%d\n", len(steps), doneCount, runningCount, pendingCount, failedCount))
	b.WriteString("Agents:\n")
	if len(agents) == 0 {
		b.WriteString("  - none\n")
	} else {
		for _, agent := range agents {
			b.WriteString(fmt.Sprintf("  - %s@%s session=%s status=%s health=%s\n",
				agent.Name, agent.WorkspaceName, agent.SessionName, agent.Status, agent.HealthState))
		}
	}
	return b.String(), nil
}

func (s *Service) ActiveRuns() ([]model.RunRecord, error) {
	runs, err := s.store.ListRuns()
	if err != nil {
		return nil, err
	}

	out := make([]model.RunRecord, 0, len(runs))
	for _, run := range runs {
		if isActiveRunStatus(run.Status) {
			out = append(out, run)
		}
	}
	return out, nil
}

func (s *Service) syncBootstrapSignals(ctx context.Context, runID string, currentStatus model.RunStatus, agents []model.AgentRecord) error {
	if currentStatus != model.RunStatusRunning && currentStatus != model.RunStatusAwaitingGuidance {
		return nil
	}

	existingPending, err := s.store.ListGuidanceRequests(runID, model.GuidanceStatusPending)
	if err != nil {
		return err
	}
	pendingByKey := make(map[string]struct{}, len(existingPending))
	for _, item := range existingPending {
		key := guidanceKey(item.WorkspaceName, item.AgentName, item.Question)
		pendingByKey[key] = struct{}{}
	}

	allComplete := len(agents) > 0
	for _, agent := range agents {
		workspacePath, err := resolveWorkspacePath(agent.WorkspaceName)
		if err != nil {
			allComplete = false
			continue
		}

		if req, ok := readGuidanceRequestFile(workspacePath); ok {
			if strings.TrimSpace(req.RunID) == "" || req.RunID == runID {
				if strings.TrimSpace(req.Agent) == "" {
					req.Agent = agent.Name
				}
				key := guidanceKey(agent.WorkspaceName, req.Agent, req.Question)
				if _, exists := pendingByKey[key]; !exists && strings.TrimSpace(req.Question) != "" {
					id, err := s.store.AddGuidanceRequest(model.GuidanceRequest{
						RunID:         runID,
						WorkspaceName: agent.WorkspaceName,
						AgentName:     req.Agent,
						Question:      strings.TrimSpace(req.Question),
						Context:       strings.TrimSpace(req.Context),
						Status:        model.GuidanceStatusPending,
					})
					if err != nil {
						return err
					}
					_ = s.store.AddEvent(runID, "guidance", fmt.Sprintf("%d", id), "requested", "", string(model.GuidanceStatusPending), fmt.Sprintf("%s@%s", req.Agent, agent.WorkspaceName))
					pendingByKey[key] = struct{}{}
				}
			}
		}

		if !hasCompletionSignal(workspacePath, runID, agent.Name) {
			allComplete = false
		}
	}

	pendingAfter, err := s.store.ListGuidanceRequests(runID, model.GuidanceStatusPending)
	if err != nil {
		return err
	}
	if currentStatus == model.RunStatusRunning && len(pendingAfter) > 0 {
		return s.transitionRun(runID, model.RunStatusRunning, model.RunStatusAwaitingGuidance, "awaiting operator guidance")
	}
	if currentStatus == model.RunStatusRunning && len(pendingAfter) == 0 && allComplete {
		return s.transitionRun(runID, model.RunStatusRunning, model.RunStatusComplete, "completion signal detected")
	}
	return nil
}

func (s *Service) ensureBootstrapCloseChecks(runID string, workspaceNames []string) error {
	brief, err := s.store.GetRunBrief(runID)
	if err != nil {
		return err
	}
	if brief == nil {
		return fmt.Errorf("bootstrap run %s is missing run brief; close blocked", runID)
	}
	if strings.TrimSpace(brief.DoneCriteria) == "" {
		return fmt.Errorf("bootstrap run %s has empty done criteria; close blocked", runID)
	}

	pending, err := s.store.ListGuidanceRequests(runID, model.GuidanceStatusPending)
	if err != nil {
		return err
	}
	if len(pending) > 0 {
		return fmt.Errorf("bootstrap run %s has %d pending guidance request(s); close blocked", runID, len(pending))
	}

	for _, workspaceName := range workspaceNames {
		workspacePath, err := resolveWorkspacePath(workspaceName)
		if err != nil {
			return err
		}
		result, ok := readValidationResult(workspacePath)
		if !ok {
			return fmt.Errorf("workspace %s is missing .metawsm/validation-result.json; close blocked", workspaceName)
		}
		if strings.TrimSpace(result.RunID) != "" && result.RunID != runID {
			return fmt.Errorf("workspace %s validation result run_id mismatch (%s)", workspaceName, result.RunID)
		}
		if !strings.EqualFold(strings.TrimSpace(result.Status), "passed") {
			return fmt.Errorf("workspace %s validation status=%q; close blocked", workspaceName, result.Status)
		}
		if strings.TrimSpace(result.DoneCriteria) != strings.TrimSpace(brief.DoneCriteria) {
			return fmt.Errorf("workspace %s validation done_criteria mismatch; close blocked", workspaceName)
		}
	}
	return nil
}

func (s *Service) transitionRun(runID string, from model.RunStatus, to model.RunStatus, message string) error {
	if !hsm.CanTransitionRun(from, to) {
		return fmt.Errorf("illegal run transition %s -> %s", from, to)
	}
	if err := s.store.UpdateRunStatus(runID, to, message); err != nil {
		return err
	}
	return s.store.AddEvent(runID, "run", runID, "transition", string(from), string(to), message)
}

func (s *Service) executeSteps(ctx context.Context, spec model.RunSpec, cfg policy.Config, steps []model.PlanStep) error {
	stepRetries := cfg.Execution.StepRetries
	agentCommand := map[string]string{}
	for _, agent := range spec.Agents {
		agentCommand[agent.Name] = agent.Command
	}

	for _, step := range steps {
		if step.Status == model.StepStatusDone {
			continue
		}

		currentSteps, err := s.store.GetSteps(spec.RunID)
		if err != nil {
			return err
		}
		if stepDoneOrSkipped(currentSteps, step.Index) {
			continue
		}

		attempts := stepRetries + 1
		var lastErr error
		for attempt := 1; attempt <= attempts; attempt++ {
			if err := s.store.UpdateStepStatus(spec.RunID, step.Index, model.StepStatusRunning, "", true, false); err != nil {
				return err
			}
			_ = s.store.AddEvent(spec.RunID, "step", fmt.Sprintf("%d", step.Index), "attempt", "", string(model.StepStatusRunning), fmt.Sprintf("attempt %d", attempt))

			err := s.executeSingleStep(ctx, spec, cfg, step, agentCommand)
			if err == nil {
				if err := s.store.UpdateStepStatus(spec.RunID, step.Index, model.StepStatusDone, "", false, true); err != nil {
					return err
				}
				_ = s.store.AddEvent(spec.RunID, "step", fmt.Sprintf("%d", step.Index), "done", string(model.StepStatusRunning), string(model.StepStatusDone), step.Name)
				lastErr = nil
				break
			}
			lastErr = err
			_ = s.store.UpdateStepStatus(spec.RunID, step.Index, model.StepStatusFailed, err.Error(), false, true)
			_ = s.store.AddEvent(spec.RunID, "step", fmt.Sprintf("%d", step.Index), "failed", string(model.StepStatusRunning), string(model.StepStatusFailed), err.Error())

			if attempt < attempts {
				time.Sleep(300 * time.Millisecond)
			}
		}

		if lastErr != nil {
			if step.Blocking {
				return fmt.Errorf("step %d %s failed: %w", step.Index, step.Name, lastErr)
			}
			_ = s.store.UpdateStepStatus(spec.RunID, step.Index, model.StepStatusSkipped, lastErr.Error(), false, true)
		}
	}

	return nil
}

func (s *Service) executeSingleStep(ctx context.Context, spec model.RunSpec, cfg policy.Config, step model.PlanStep, agentCommands map[string]string) error {
	switch step.Kind {
	case "shell":
		if strings.TrimSpace(step.Command) == "" {
			return fmt.Errorf("empty shell command")
		}
		if err := runShell(ctx, step.Command); err != nil {
			return err
		}
		if workspaceCreateStep(step) {
			if err := alignWorkspaceToBaseBranch(ctx, step.WorkspaceName, spec.Repos, spec.BaseBranch); err != nil {
				return err
			}
		}
		return nil
	case "tmux_start":
		workspacePath, err := resolveWorkspacePath(step.WorkspaceName)
		if err != nil {
			return err
		}
		command := agentCommands[step.Agent]
		if strings.TrimSpace(command) == "" {
			command = "bash"
		}
		command = wrapAgentCommandForTmux(command)
		sessionName := policy.RenderSessionName(cfg.Tmux.SessionPattern, step.Agent, step.WorkspaceName)

		hasSession := tmuxHasSession(ctx, sessionName)
		if !hasSession {
			tmuxCmd := fmt.Sprintf("tmux new-session -d -s %s -c %s %s", shellQuote(sessionName), shellQuote(workspacePath), shellQuote(command))
			if err := runShell(ctx, tmuxCmd); err != nil {
				return err
			}
			time.Sleep(200 * time.Millisecond)
			if !tmuxHasSession(ctx, sessionName) {
				return fmt.Errorf("tmux session %s exited immediately after start", sessionName)
			}
		}
		now := time.Now()
		return s.store.UpdateAgentStatus(spec.RunID, step.Agent, step.WorkspaceName, model.AgentStatusRunning, model.HealthStateHealthy, &now, &now)
	default:
		return fmt.Errorf("unknown step kind: %s", step.Kind)
	}
}

func (s *Service) seedAgents(runID string, steps []model.PlanStep) error {
	seen := map[string]struct{}{}
	now := time.Now()
	for _, step := range steps {
		if step.Kind != "tmux_start" {
			continue
		}
		key := fmt.Sprintf("%s|%s", step.Agent, step.WorkspaceName)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		record := model.AgentRecord{
			RunID:          runID,
			Name:           step.Agent,
			WorkspaceName:  step.WorkspaceName,
			SessionName:    policy.RenderSessionName("{agent}-{workspace}", step.Agent, step.WorkspaceName),
			Status:         model.AgentStatusPending,
			HealthState:    model.HealthStateIdle,
			LastActivityAt: &now,
			LastProgressAt: &now,
		}
		if err := s.store.UpsertAgent(record); err != nil {
			return err
		}
	}
	return nil
}

func buildPlan(spec model.RunSpec, cfg policy.Config) []model.PlanStep {
	steps := make([]model.PlanStep, 0)
	index := 1
	repoCSV := strings.Join(spec.Repos, ",")

	for _, ticket := range spec.Tickets {
		workspaceName := workspaceNameFor(ticket, spec.RunID)
		steps = append(steps, model.PlanStep{
			Index:    index,
			Name:     fmt.Sprintf("verify-doc-ticket-%s", ticket),
			Kind:     "shell",
			Command:  fmt.Sprintf("docmgr ticket list --ticket %s", shellQuote(ticket)),
			Blocking: true,
			Ticket:   ticket,
			Status:   model.StepStatusPending,
		})
		index++

		workspaceCommand := ""
		switch spec.WorkspaceStrategy {
		case model.WorkspaceStrategyCreate:
			workspaceBranch := fmt.Sprintf("%s/%s", cfg.Workspace.BranchPrefix, workspaceName)
			workspaceCommand = fmt.Sprintf(
				"wsm create %s --repos %s --branch %s",
				shellQuote(workspaceName),
				shellQuote(repoCSV),
				shellQuote(workspaceBranch),
			)
		case model.WorkspaceStrategyFork:
			workspaceCommand = fmt.Sprintf(
				"wsm fork %s --branch-prefix %s",
				shellQuote(workspaceName),
				shellQuote(cfg.Workspace.BranchPrefix),
			)
		case model.WorkspaceStrategyReuse:
			workspaceCommand = fmt.Sprintf("wsm info %s", shellQuote(workspaceName))
		default:
			workspaceBranch := fmt.Sprintf("%s/%s", cfg.Workspace.BranchPrefix, workspaceName)
			workspaceCommand = fmt.Sprintf(
				"wsm create %s --repos %s --branch %s",
				shellQuote(workspaceName),
				shellQuote(repoCSV),
				shellQuote(workspaceBranch),
			)
		}
		steps = append(steps, model.PlanStep{
			Index:         index,
			Name:          fmt.Sprintf("workspace-%s-%s", spec.WorkspaceStrategy, workspaceName),
			Kind:          "shell",
			Command:       workspaceCommand,
			Blocking:      true,
			Ticket:        ticket,
			WorkspaceName: workspaceName,
			Status:        model.StepStatusPending,
		})
		index++

		for _, agent := range spec.Agents {
			steps = append(steps, model.PlanStep{
				Index:         index,
				Name:          fmt.Sprintf("tmux-start-%s-%s", agent.Name, workspaceName),
				Kind:          "tmux_start",
				Command:       "",
				Blocking:      true,
				Ticket:        ticket,
				WorkspaceName: workspaceName,
				Agent:         agent.Name,
				Status:        model.StepStatusPending,
			})
			index++
		}
	}

	return steps
}

func stepDoneOrSkipped(steps []model.StepRecord, stepIndex int) bool {
	for _, step := range steps {
		if step.Index == stepIndex {
			return step.Status == model.StepStatusDone || step.Status == model.StepStatusSkipped
		}
	}
	return false
}

func workspaceNameFor(ticket string, runID string) string {
	prefix := strings.ToLower(strings.TrimSpace(ticket))
	prefix = strings.ReplaceAll(prefix, "/", "-")
	prefix = strings.ReplaceAll(prefix, " ", "-")
	runToken := strings.ToLower(strings.TrimSpace(runID))
	runToken = strings.TrimPrefix(runToken, "run-")
	runToken = strings.ReplaceAll(runToken, "/", "-")
	runToken = strings.ReplaceAll(runToken, " ", "-")
	runToken = strings.ReplaceAll(runToken, ":", "-")
	for strings.Contains(runToken, "--") {
		runToken = strings.ReplaceAll(runToken, "--", "-")
	}
	runToken = strings.Trim(runToken, "-")
	if runToken == "" {
		runToken = "x"
	}
	if len(runToken) > 14 {
		runToken = runToken[len(runToken)-14:]
	}
	return fmt.Sprintf("%s-%s", prefix, runToken)
}

func resolveWorkspacePath(workspaceName string) (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(configDir, "workspace-manager", "workspaces", workspaceName+".json")
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read workspace config %s: %w", path, err)
	}
	var payload struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(b, &payload); err != nil {
		return "", fmt.Errorf("parse workspace config %s: %w", path, err)
	}
	if strings.TrimSpace(payload.Path) == "" {
		return "", errors.New("workspace config missing path")
	}
	return payload.Path, nil
}

func workspaceCreateStep(step model.PlanStep) bool {
	if strings.TrimSpace(step.WorkspaceName) == "" {
		return false
	}
	command := strings.TrimSpace(step.Command)
	return strings.HasPrefix(command, "wsm create ")
}

func normalizeBaseBranch(branch string) string {
	branch = strings.TrimSpace(branch)
	branch = strings.TrimPrefix(branch, "origin/")
	return branch
}

func alignWorkspaceToBaseBranch(ctx context.Context, workspaceName string, repos []string, baseBranch string) error {
	workspacePath, err := resolveWorkspacePath(workspaceName)
	if err != nil {
		return err
	}
	repoPaths, err := workspaceRepoPaths(workspacePath, repos)
	if err != nil {
		return err
	}
	targetBranch := normalizeBaseBranch(baseBranch)
	if targetBranch == "" {
		targetBranch = "main"
	}
	for _, repoPath := range repoPaths {
		if err := resetRepoToBaseBranch(ctx, repoPath, targetBranch); err != nil {
			return err
		}
	}
	return nil
}

func workspaceRepoPaths(workspacePath string, repos []string) ([]string, error) {
	paths := make([]string, 0, len(repos))
	if len(repos) == 0 {
		if isGitRepo(workspacePath) {
			return []string{workspacePath}, nil
		}
		return nil, fmt.Errorf("workspace %s has no repository definitions", workspacePath)
	}
	for _, repo := range repos {
		repo = strings.TrimSpace(repo)
		if repo == "" {
			continue
		}
		candidate := filepath.Join(workspacePath, repo)
		if isGitRepo(candidate) {
			paths = append(paths, candidate)
			continue
		}
		if len(repos) == 1 && isGitRepo(workspacePath) {
			paths = append(paths, workspacePath)
			continue
		}
		return nil, fmt.Errorf("workspace repo path not found: %s", candidate)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no git repositories found in workspace %s", workspacePath)
	}
	return paths, nil
}

func isGitRepo(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	if err != nil {
		return false
	}
	return info.IsDir() || info.Mode().IsRegular()
}

func resetRepoToBaseBranch(ctx context.Context, repoPath string, baseBranch string) error {
	_ = exec.CommandContext(ctx, "git", "-C", repoPath, "fetch", "origin", baseBranch).Run()

	target := ""
	remoteRef := "refs/remotes/origin/" + baseBranch
	localRef := "refs/heads/" + baseBranch
	if gitRefExists(ctx, repoPath, remoteRef) {
		target = "origin/" + baseBranch
	} else if gitRefExists(ctx, repoPath, localRef) {
		target = baseBranch
	}
	if target == "" {
		return fmt.Errorf("base branch %q not found for repo %s", baseBranch, repoPath)
	}

	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "reset", "--hard", target)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("reset repo %s to %s: %w: %s", repoPath, target, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func deleteWorkspaceIfPresent(ctx context.Context, workspaceName string) error {
	command := exec.CommandContext(ctx, "zsh", "-lc", fmt.Sprintf("wsm delete %s", shellQuote(workspaceName)))
	output, err := command.CombinedOutput()
	if err == nil {
		return nil
	}
	if isWorkspaceNotFoundOutput(string(output)) {
		return nil
	}
	return fmt.Errorf("wsm delete %s failed: %w: %s", workspaceName, err, strings.TrimSpace(string(output)))
}

func isWorkspaceNotFoundOutput(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "workspace") && strings.Contains(lower, "not found")
}

func gitRefExists(ctx context.Context, repoPath string, ref string) bool {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "show-ref", "--verify", "--quiet", ref)
	return cmd.Run() == nil
}

func wrapAgentCommandForTmux(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		command = "bash"
	}
	script := command + "; status=$?; printf '[metawsm] agent command exited with status %s at %s\\n' \"$status\" \"$(date -Iseconds)\"; exec bash"
	return fmt.Sprintf("bash -lc %s", shellQuote(script))
}

func runShell(ctx context.Context, command string) error {
	cmd := exec.CommandContext(ctx, "zsh", "-lc", command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func hasDirtyGitState(ctx context.Context, repoPath string) (bool, error) {
	cmd := exec.CommandContext(ctx, "zsh", "-lc", fmt.Sprintf("git -C %s status --porcelain", shellQuote(repoPath)))
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) != "", nil
}

func evaluateHealth(ctx context.Context, cfg policy.Config, agent model.AgentRecord, now time.Time) (model.HealthState, model.AgentStatus, *time.Time) {
	hasSession := tmuxHasSession(ctx, agent.SessionName)
	if !hasSession {
		return model.HealthStateDead, model.AgentStatusDead, agent.LastActivityAt
	}

	activityEpoch := fetchSessionActivity(ctx, agent.SessionName)
	lastActivity := agent.LastActivityAt
	if activityEpoch > 0 {
		t := time.Unix(activityEpoch, 0)
		lastActivity = &t
	}

	activityAge := time.Duration(0)
	if lastActivity != nil {
		activityAge = now.Sub(*lastActivity)
	}

	progressAge := time.Duration(0)
	if agent.LastProgressAt != nil {
		progressAge = now.Sub(*agent.LastProgressAt)
	}

	idleThreshold := time.Duration(cfg.Health.IdleSeconds) * time.Second
	activityStalledThreshold := time.Duration(cfg.Health.ActivityStalledSeconds) * time.Second
	progressStalledThreshold := time.Duration(cfg.Health.ProgressStalledSeconds) * time.Second

	if activityAge >= activityStalledThreshold || (progressAge >= progressStalledThreshold && progressAge > 0) {
		return model.HealthStateStalled, model.AgentStatusStalled, lastActivity
	}
	if activityAge >= idleThreshold {
		return model.HealthStateIdle, model.AgentStatusIdle, lastActivity
	}
	return model.HealthStateHealthy, model.AgentStatusRunning, lastActivity
}

func fetchSessionActivity(ctx context.Context, sessionName string) int64 {
	cmd := exec.CommandContext(ctx, "zsh", "-lc", fmt.Sprintf("tmux display-message -p -t %s '#{session_activity}'", shellQuote(sessionName)))
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	value := strings.TrimSpace(string(out))
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func tmuxHasSession(ctx context.Context, sessionName string) bool {
	cmd := exec.CommandContext(ctx, "zsh", "-lc", fmt.Sprintf("tmux has-session -t %s", shellQuote(sessionName)))
	var stderr strings.Builder
	cmd.Stderr = &stderr
	return cmd.Run() == nil
}

func tmuxKillSession(ctx context.Context, sessionName string) error {
	cmd := exec.CommandContext(ctx, "zsh", "-lc", fmt.Sprintf("tmux kill-session -t %s", shellQuote(sessionName)))
	var stderr strings.Builder
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return nil
	}
	message := stderr.String()
	if strings.Contains(message, "no server running") || strings.Contains(message, "can't find session") || strings.Contains(message, "error connecting") {
		return nil
	}
	return err
}

func guidanceKey(workspaceName string, agentName string, question string) string {
	return workspaceName + "|" + agentName + "|" + strings.TrimSpace(question)
}

func readGuidanceRequestFile(workspacePath string) (model.GuidanceRequestPayload, bool) {
	path := filepath.Join(workspacePath, ".metawsm", "guidance-request.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return model.GuidanceRequestPayload{}, false
	}
	var payload model.GuidanceRequestPayload
	if err := json.Unmarshal(b, &payload); err != nil {
		return model.GuidanceRequestPayload{}, false
	}
	if strings.TrimSpace(payload.Question) == "" {
		return model.GuidanceRequestPayload{}, false
	}
	return payload, true
}

func hasCompletionSignal(workspacePath string, runID string, agentName string) bool {
	path := filepath.Join(workspacePath, ".metawsm", "implementation-complete.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var payload model.CompletionSignalPayload
	if err := json.Unmarshal(b, &payload); err != nil {
		return false
	}
	if strings.TrimSpace(payload.RunID) != "" && payload.RunID != runID {
		return false
	}
	if strings.TrimSpace(payload.Agent) != "" && payload.Agent != agentName {
		return false
	}
	return true
}

func readValidationResult(workspacePath string) (struct {
	RunID        string `json:"run_id,omitempty"`
	Status       string `json:"status"`
	DoneCriteria string `json:"done_criteria"`
}, bool) {
	path := filepath.Join(workspacePath, ".metawsm", "validation-result.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return struct {
			RunID        string `json:"run_id,omitempty"`
			Status       string `json:"status"`
			DoneCriteria string `json:"done_criteria"`
		}{}, false
	}
	var payload struct {
		RunID        string `json:"run_id,omitempty"`
		Status       string `json:"status"`
		DoneCriteria string `json:"done_criteria"`
	}
	if err := json.Unmarshal(b, &payload); err != nil {
		return payload, false
	}
	return payload, true
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func generateRunID() string {
	return "run-" + time.Now().Format("20060102-150405")
}

func normalizeTokens(values []string) []string {
	out := []string{}
	seen := map[string]struct{}{}
	for _, value := range values {
		for _, token := range strings.Split(value, ",") {
			token = strings.TrimSpace(token)
			if token == "" {
				continue
			}
			if _, ok := seen[token]; ok {
				continue
			}
			seen[token] = struct{}{}
			out = append(out, token)
		}
	}
	return out
}

func isActiveRunStatus(status model.RunStatus) bool {
	switch status {
	case model.RunStatusPlanning, model.RunStatusRunning, model.RunStatusAwaitingGuidance, model.RunStatusPaused, model.RunStatusStopping, model.RunStatusClosing:
		return true
	default:
		return false
	}
}
