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
	record, _, _, err := s.store.GetRun(options.RunID)
	if err != nil {
		return err
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
		return runShell(ctx, step.Command)
	case "tmux_start":
		workspacePath, err := resolveWorkspacePath(step.WorkspaceName)
		if err != nil {
			return err
		}
		command := agentCommands[step.Agent]
		if strings.TrimSpace(command) == "" {
			command = "bash"
		}
		sessionName := policy.RenderSessionName(cfg.Tmux.SessionPattern, step.Agent, step.WorkspaceName)

		hasSession := tmuxHasSession(ctx, sessionName)
		if !hasSession {
			tmuxCmd := fmt.Sprintf("tmux new-session -d -s %s -c %s %s", shellQuote(sessionName), shellQuote(workspacePath), shellQuote(command))
			if err := runShell(ctx, tmuxCmd); err != nil {
				return err
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
			workspaceCommand = fmt.Sprintf(
				"wsm create %s --repos %s --branch-prefix %s",
				shellQuote(workspaceName),
				shellQuote(repoCSV),
				shellQuote(cfg.Workspace.BranchPrefix),
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
			workspaceCommand = fmt.Sprintf(
				"wsm create %s --repos %s --branch-prefix %s",
				shellQuote(workspaceName),
				shellQuote(repoCSV),
				shellQuote(cfg.Workspace.BranchPrefix),
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
	shortID := runID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	return fmt.Sprintf("%s-%s", prefix, shortID)
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
