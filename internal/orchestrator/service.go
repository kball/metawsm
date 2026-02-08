package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	DocRepo           string
	DocHomeRepo       string
	DocAuthorityMode  string
	DocSeedMode       string
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

type MergeOptions struct {
	Ticket string
	RunID  string
	DryRun bool
}

type IterateOptions struct {
	Ticket   string
	RunID    string
	Feedback string
	DryRun   bool
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

type MergeResult struct {
	RunID   string
	Actions []string
}

type IterateResult struct {
	RunID   string
	Actions []string
}

type ActiveDocContext struct {
	RunID       string
	Ticket      string
	DocHomeRepo string
}

type OperatorRunContext struct {
	RunID       string
	Tickets     []string
	DocHomeRepo string
	Repos       []string
	Agents      []model.AgentRecord
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
	docHomeRepo, err := resolveDocHomeRepo(options.DocHomeRepo, options.DocRepo, repos)
	if err != nil {
		return RunResult{}, err
	}
	if len(repos) > 0 && strings.TrimSpace(docHomeRepo) != "" && !containsToken(repos, docHomeRepo) {
		return RunResult{}, fmt.Errorf("doc home repo %q must be one of --repos (%s)", docHomeRepo, strings.Join(repos, ","))
	}
	docAuthorityMode := normalizeDocAuthorityMode(options.DocAuthorityMode)
	if docAuthorityMode == "" {
		docAuthorityMode = normalizeDocAuthorityMode(cfg.Docs.AuthorityMode)
	}
	if docAuthorityMode == "" {
		docAuthorityMode = model.DocAuthorityModeWorkspaceActive
	}
	if !isValidDocAuthorityMode(docAuthorityMode) {
		return RunResult{}, fmt.Errorf("doc authority mode %q is invalid", docAuthorityMode)
	}
	docSeedMode := normalizeDocSeedMode(options.DocSeedMode)
	if docSeedMode == "" {
		docSeedMode = normalizeDocSeedMode(cfg.Docs.SeedMode)
	}
	if docSeedMode == "" {
		docSeedMode = model.DocSeedModeCopyFromRepoOnStart
	}
	if !isValidDocSeedMode(docSeedMode) {
		return RunResult{}, fmt.Errorf("doc seed mode %q is invalid", docSeedMode)
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

	agents, err := policy.ResolveAgents(cfg, normalizeTokens(options.AgentNames), policyPath)
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
		RunID:                runID,
		Mode:                 mode,
		Tickets:              tickets,
		Repos:                repos,
		DocRepo:              docHomeRepo,
		DocHomeRepo:          docHomeRepo,
		DocAuthorityMode:     docAuthorityMode,
		DocSeedMode:          docSeedMode,
		DocFreshnessRevision: "",
		BaseBranch:           baseBranch,
		WorkspaceStrategy:    strategy,
		Agents:               agents,
		PolicyPath:           policyPath,
		DryRun:               options.DryRun,
		CreatedAt:            time.Now(),
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
		agentWorkdir, err := resolveDocRepoPath(workspacePath, effectiveDocHomeRepo(spec), spec.Repos)
		if err != nil {
			return RestartResult{}, err
		}
		command := strings.TrimSpace(agentCommand[agent.Name])
		if command == "" {
			command = "bash"
		}
		command = normalizeAgentCommand(command)
		command = wrapAgentCommandForTmux(command)
		sessionName := strings.TrimSpace(agent.SessionName)
		if sessionName == "" {
			sessionName = policy.RenderSessionName("{agent}-{workspace}", agent.Name, agent.WorkspaceName)
		}

		killCmd := fmt.Sprintf("tmux kill-session -t %s", shellQuote(sessionName))
		startCmd := fmt.Sprintf("tmux new-session -d -s %s -c %s %s", shellQuote(sessionName), shellQuote(agentWorkdir), shellQuote(command))
		actions = append(actions, killCmd, startCmd)

		if options.DryRun {
			continue
		}
		_ = tmuxKillSession(ctx, sessionName)
		if err := runShell(ctx, startCmd); err != nil {
			return RestartResult{}, err
		}
		time.Sleep(200 * time.Millisecond)
		if !tmuxHasSession(ctx, sessionName) {
			return RestartResult{}, fmt.Errorf("tmux session %s exited immediately after restart", sessionName)
		}
		if err := waitForAgentStartup(ctx, sessionName, 4*time.Second); err != nil {
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

func (s *Service) Merge(ctx context.Context, options MergeOptions) (MergeResult, error) {
	runID, err := s.resolveRunID(options.RunID, options.Ticket)
	if err != nil {
		return MergeResult{}, err
	}

	record, specJSON, _, err := s.store.GetRun(runID)
	if err != nil {
		return MergeResult{}, err
	}
	if record.Status != model.RunStatusComplete && record.Status != model.RunStatusClosed {
		return MergeResult{}, fmt.Errorf("run %s must be completed before merge (current: %s)", runID, record.Status)
	}

	var spec model.RunSpec
	if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
		return MergeResult{}, fmt.Errorf("unmarshal run spec: %w", err)
	}

	agents, err := s.store.GetAgents(runID)
	if err != nil {
		return MergeResult{}, err
	}
	workspaces := workspaceNamesFromAgents(agents)
	if len(workspaces) == 0 {
		return MergeResult{}, fmt.Errorf("run %s has no workspaces to merge", runID)
	}

	actions := make([]string, 0, len(workspaces))
	for _, workspaceName := range workspaces {
		workspacePath, err := resolveWorkspacePath(workspaceName)
		if err != nil {
			return MergeResult{}, err
		}
		repoStates, err := workspaceRepoDirtyStates(ctx, workspacePath, spec.Repos)
		if err != nil {
			return MergeResult{}, err
		}
		dirty := false
		for _, state := range repoStates {
			if state.Dirty {
				dirty = true
				break
			}
		}
		if !dirty {
			continue
		}
		actions = append(actions, fmt.Sprintf("wsm merge %s", shellQuote(workspaceName)))
	}

	if options.DryRun || record.Status == model.RunStatusClosed {
		return MergeResult{RunID: runID, Actions: actions}, nil
	}

	for _, action := range actions {
		if err := runShell(ctx, action); err != nil {
			return MergeResult{}, err
		}
	}
	_ = s.store.AddEvent(runID, "run", runID, "merge", "", "", fmt.Sprintf("merged %d workspace(s)", len(actions)))
	return MergeResult{RunID: runID, Actions: actions}, nil
}

func (s *Service) Iterate(ctx context.Context, options IterateOptions) (IterateResult, error) {
	feedback := strings.TrimSpace(options.Feedback)
	if feedback == "" {
		return IterateResult{}, fmt.Errorf("feedback cannot be empty")
	}
	runID, err := s.resolveRunID(options.RunID, options.Ticket)
	if err != nil {
		return IterateResult{}, err
	}

	record, specJSON, _, err := s.store.GetRun(runID)
	if err != nil {
		return IterateResult{}, err
	}
	if record.Status == model.RunStatusClosed {
		return IterateResult{}, fmt.Errorf("run %s is closed and cannot iterate", runID)
	}

	var spec model.RunSpec
	if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
		return IterateResult{}, fmt.Errorf("unmarshal run spec: %w", err)
	}

	agents, err := s.store.GetAgents(runID)
	if err != nil {
		return IterateResult{}, err
	}
	workspaces := workspaceNamesFromAgents(agents)
	if len(workspaces) == 0 {
		return IterateResult{}, fmt.Errorf("run %s has no workspaces for iteration", runID)
	}

	now := time.Now()
	actions := []string{}
	for _, workspaceName := range workspaces {
		workspacePath, err := resolveWorkspacePath(workspaceName)
		if err != nil {
			return IterateResult{}, err
		}
		workspaceActions, err := recordIterationFeedback(workspacePath, effectiveDocHomeRepo(spec), spec.Repos, spec.Tickets, feedback, now, options.DryRun)
		if err != nil {
			return IterateResult{}, err
		}
		actions = append(actions, workspaceActions...)
	}

	restartResult, err := s.Restart(ctx, RestartOptions{
		RunID:  runID,
		DryRun: options.DryRun,
	})
	if err != nil {
		return IterateResult{}, err
	}
	actions = append(actions, restartResult.Actions...)

	if options.DryRun {
		return IterateResult{RunID: runID, Actions: actions}, nil
	}
	_ = s.store.AddEvent(runID, "run", runID, "iterate", "", "", fmt.Sprintf("iteration feedback recorded for %d workspace(s)", len(workspaces)))
	return IterateResult{RunID: runID, Actions: actions}, nil
}

func (s *Service) ResolveRunID(explicitRunID string, ticket string) (string, error) {
	return s.resolveRunID(explicitRunID, ticket)
}

func (s *Service) GetOperatorRunState(runID string) (*model.OperatorRunState, error) {
	return s.store.GetOperatorRunState(runID)
}

func (s *Service) UpsertOperatorRunState(state model.OperatorRunState) error {
	return s.store.UpsertOperatorRunState(state)
}

func (s *Service) UpsertRunPullRequest(record model.RunPullRequest) error {
	return s.store.UpsertRunPullRequest(record)
}

func (s *Service) ListRunPullRequests(runID string) ([]model.RunPullRequest, error) {
	return s.store.ListRunPullRequests(runID)
}

func (s *Service) OperatorRunContext(runID string) (OperatorRunContext, error) {
	_, specJSON, _, err := s.store.GetRun(runID)
	if err != nil {
		return OperatorRunContext{}, err
	}
	var spec model.RunSpec
	if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
		return OperatorRunContext{}, fmt.Errorf("unmarshal run spec: %w", err)
	}
	tickets, err := s.store.GetTickets(runID)
	if err != nil {
		return OperatorRunContext{}, err
	}
	agents, err := s.store.GetAgents(runID)
	if err != nil {
		return OperatorRunContext{}, err
	}
	return OperatorRunContext{
		RunID:       runID,
		Tickets:     tickets,
		DocHomeRepo: effectiveDocHomeRepo(spec),
		Repos:       append([]string(nil), spec.Repos...),
		Agents:      agents,
	}, nil
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

	record, specJSON, _, err := s.store.GetRun(runID)
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
	var spec model.RunSpec
	if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
		return GuideResult{}, fmt.Errorf("unmarshal run spec: %w", err)
	}

	req := pending[0]
	workspacePath, err := resolveWorkspacePath(req.WorkspaceName)
	if err != nil {
		return GuideResult{}, err
	}
	signalRoots := bootstrapSignalRoots(workspacePath, spec)
	responseRoot := firstSignalRootWithFile(signalRoots, "guidance-request.json")
	if responseRoot == "" && len(signalRoots) > 0 {
		responseRoot = signalRoots[0]
	}
	if responseRoot == "" {
		responseRoot = workspacePath
	}
	responsePath := filepath.Join(responseRoot, ".metawsm", "guidance-response.json")
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

	for _, root := range signalRoots {
		requestPath := filepath.Join(root, ".metawsm", "guidance-request.json")
		_ = os.Remove(requestPath)
	}

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

	workspaces := workspaceNamesFromAgents(agents)
	if err := s.ensureWorkspaceDocCloseChecks(ctx, options.RunID, spec, workspaces, tickets); err != nil {
		return err
	}

	for _, workspaceName := range workspaces {
		path, err := resolveWorkspacePath(workspaceName)
		if err != nil {
			return err
		}
		repoStates, err := workspaceRepoDirtyStates(ctx, path, spec.Repos)
		if err != nil {
			return err
		}
		for _, state := range repoStates {
			if state.Dirty {
				return fmt.Errorf("workspace %s repo %s has uncommitted changes; close blocked", workspaceName, state.RepoPath)
			}
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
		health, status, lastActivity, lastProgress := evaluateHealth(ctx, cfg, agent, now)
		_ = s.store.UpdateAgentStatus(runID, agent.Name, agent.WorkspaceName, status, health, lastActivity, lastProgress)
	}
	agents, _ = s.store.GetAgents(runID)
	if spec.Mode == model.RunModeBootstrap {
		if err := s.syncBootstrapSignals(ctx, runID, record.Status, spec, agents); err != nil {
			return "", err
		}
		record, _, _, _ = s.store.GetRun(runID)
	}
	pendingGuidance, _ := s.store.ListGuidanceRequests(runID, model.GuidanceStatusPending)
	brief, _ := s.store.GetRunBrief(runID)
	docSyncStates, _ := s.store.ListDocSyncStates(runID)
	runPullRequests, _ := s.store.ListRunPullRequests(runID)

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
	docHomeRepo := effectiveDocHomeRepo(spec)
	docAuthorityMode := normalizeDocAuthorityMode(string(spec.DocAuthorityMode))
	if docAuthorityMode == "" {
		docAuthorityMode = model.DocAuthorityModeWorkspaceActive
	}
	docSeedMode := normalizeDocSeedMode(string(spec.DocSeedMode))
	if docSeedMode == "" {
		docSeedMode = model.DocSeedModeCopyFromRepoOnStart
	}
	freshnessRevision := strings.TrimSpace(spec.DocFreshnessRevision)
	if freshnessRevision == "" {
		freshnessRevision = latestDocFreshnessRevision(docSyncStates)
	}
	b.WriteString("Docs:\n")
	b.WriteString(fmt.Sprintf("  home_repo=%s\n", emptyAsUnknown(docHomeRepo)))
	b.WriteString(fmt.Sprintf("  authority=%s\n", docAuthorityMode))
	b.WriteString(fmt.Sprintf("  seed_mode=%s\n", docSeedMode))
	b.WriteString(fmt.Sprintf("  freshness_revision=%s\n", emptyAsUnknown(freshnessRevision)))
	if len(docSyncStates) == 0 {
		b.WriteString("  sync=none\n")
	} else {
		for _, state := range docSyncStates {
			b.WriteString(fmt.Sprintf(
				"  sync ticket=%s workspace=%s status=%s revision=%s updated_at=%s\n",
				state.Ticket,
				state.WorkspaceName,
				state.Status,
				emptyAsUnknown(state.Revision),
				state.UpdatedAt.Format(time.RFC3339),
			))
		}
	}
	for _, warning := range docFreshnessWarnings(docSyncStates, docSeedMode, cfg.Docs.StaleWarningSeconds, now) {
		b.WriteString(fmt.Sprintf("  warning=%s (warning-only)\n", warning))
	}
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
	if len(workspaceDiffs) > 0 {
		b.WriteString("Diffs:\n")
		for _, diff := range workspaceDiffs {
			if diff.Error != nil {
				b.WriteString(fmt.Sprintf("  - %s error=%v\n", diff.WorkspaceName, diff.Error))
				continue
			}
			b.WriteString(fmt.Sprintf("  - %s path=%s\n", diff.WorkspaceName, diff.WorkspacePath))
			for _, repo := range diff.Repos {
				repoLabel := repo.RepoLabel
				if repo.Error != nil {
					b.WriteString(fmt.Sprintf("    * %s error=%v\n", repoLabel, repo.Error))
					continue
				}
				if len(repo.StatusLines) == 0 {
					b.WriteString(fmt.Sprintf("    * %s clean\n", repoLabel))
					continue
				}
				b.WriteString(fmt.Sprintf("    * %s dirty files=%d\n", repoLabel, len(repo.StatusLines)))
				limit := len(repo.StatusLines)
				if limit > 8 {
					limit = 8
				}
				for i := 0; i < limit; i++ {
					b.WriteString(fmt.Sprintf("      %s\n", repo.StatusLines[i]))
				}
				if len(repo.StatusLines) > limit {
					b.WriteString(fmt.Sprintf("      ... (%d more)\n", len(repo.StatusLines)-limit))
				}
			}
		}
	}
	if len(runPullRequests) > 0 {
		b.WriteString("Pull Requests:\n")
		for _, item := range runPullRequests {
			repoLabel := strings.TrimSpace(item.Repo)
			if repoLabel == "" {
				repoLabel = "unknown-repo"
			}
			ticketLabel := strings.TrimSpace(item.Ticket)
			if ticketLabel == "" {
				ticketLabel = "unknown-ticket"
			}
			b.WriteString(fmt.Sprintf("  - %s/%s state=%s head=%s base=%s number=%d url=%s actor=%s\n",
				ticketLabel,
				repoLabel,
				valueOrDefault(string(item.PRState), "unknown"),
				valueOrDefault(item.HeadBranch, "-"),
				valueOrDefault(item.BaseBranch, "-"),
				item.PRNumber,
				valueOrDefault(item.PRURL, "-"),
				valueOrDefault(item.Actor, "-"),
			))
		}
	}

	if record.Status == model.RunStatusComplete {
		b.WriteString("Next:\n")
		if len(tickets) == 1 {
			b.WriteString(fmt.Sprintf("  - metawsm iterate --ticket %s --feedback \"<feedback from diff review>\"\n", tickets[0]))
			b.WriteString(fmt.Sprintf("  - metawsm merge --ticket %s --dry-run\n", tickets[0]))
			b.WriteString(fmt.Sprintf("  - metawsm merge --ticket %s\n", tickets[0]))
			b.WriteString(fmt.Sprintf("  - metawsm close --ticket %s\n", tickets[0]))
		} else {
			b.WriteString(fmt.Sprintf("  - metawsm iterate --run-id %s --feedback \"<feedback from diff review>\"\n", runID))
			b.WriteString(fmt.Sprintf("  - metawsm merge --run-id %s --dry-run\n", runID))
			b.WriteString(fmt.Sprintf("  - metawsm merge --run-id %s\n", runID))
			b.WriteString(fmt.Sprintf("  - metawsm close --run-id %s\n", runID))
		}
	}

	b.WriteString(fmt.Sprintf("Steps: total=%d done=%d running=%d pending=%d failed=%d\n", len(steps), doneCount, runningCount, pendingCount, failedCount))
	b.WriteString("Agents:\n")
	if len(agents) == 0 {
		b.WriteString("  - none\n")
	} else {
		for _, agent := range agents {
			b.WriteString(fmt.Sprintf("  - %s@%s session=%s status=%s health=%s last_activity=%s activity_age=%s last_progress=%s progress_age=%s\n",
				agent.Name,
				agent.WorkspaceName,
				agent.SessionName,
				agent.Status,
				agent.HealthState,
				formatTimeOrDash(agent.LastActivityAt),
				formatAgeOrDash(now, agent.LastActivityAt),
				formatTimeOrDash(agent.LastProgressAt),
				formatAgeOrDash(now, agent.LastProgressAt),
			))
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

func (s *Service) ActiveDocContexts() ([]ActiveDocContext, error) {
	runs, err := s.ActiveRuns()
	if err != nil {
		return nil, err
	}
	out := []ActiveDocContext{}
	for _, run := range runs {
		_, specJSON, _, err := s.store.GetRun(run.RunID)
		if err != nil {
			return nil, err
		}
		var spec model.RunSpec
		if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
			return nil, fmt.Errorf("unmarshal run spec: %w", err)
		}
		docHomeRepo := effectiveDocHomeRepo(spec)
		for _, ticket := range spec.Tickets {
			ticket = strings.TrimSpace(ticket)
			if ticket == "" {
				continue
			}
			out = append(out, ActiveDocContext{
				RunID:       run.RunID,
				Ticket:      ticket,
				DocHomeRepo: docHomeRepo,
			})
		}
	}
	return out, nil
}

func (s *Service) syncBootstrapSignals(ctx context.Context, runID string, currentStatus model.RunStatus, spec model.RunSpec, agents []model.AgentRecord) error {
	if currentStatus != model.RunStatusRunning && currentStatus != model.RunStatusAwaitingGuidance {
		return nil
	}
	for _, agent := range agents {
		if agent.Status == model.AgentStatusFailed {
			return s.transitionRun(runID, currentStatus, model.RunStatusFailed, fmt.Sprintf("agent %s@%s failed", agent.Name, agent.WorkspaceName))
		}
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
		signalRoots := bootstrapSignalRoots(workspacePath, spec)

		if req, ok := readGuidanceRequestFileFromRoots(signalRoots); ok {
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

		if !hasCompletionSignalFromRoots(signalRoots, runID, agent.Name) {
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
	_, specJSON, _, err := s.store.GetRun(runID)
	if err != nil {
		return err
	}
	var spec model.RunSpec
	if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
		return fmt.Errorf("unmarshal run spec: %w", err)
	}

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
		result, ok := readValidationResultFromRoots(bootstrapSignalRoots(workspacePath, spec))
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

func (s *Service) ensureWorkspaceDocCloseChecks(ctx context.Context, runID string, spec model.RunSpec, workspaceNames []string, tickets []string) error {
	docSeedMode := normalizeDocSeedMode(string(spec.DocSeedMode))
	if docSeedMode == "" {
		docSeedMode = model.DocSeedModeCopyFromRepoOnStart
	}
	if docSeedMode != model.DocSeedModeCopyFromRepoOnStart {
		return nil
	}

	docSyncStates, err := s.store.ListDocSyncStates(runID)
	if err != nil {
		return err
	}
	stateByTicketWorkspace := map[string]model.DocSyncState{}
	for _, state := range docSyncStates {
		key := state.Ticket + "|" + state.WorkspaceName
		stateByTicketWorkspace[key] = state
	}

	docHomeRepo := effectiveDocHomeRepo(spec)
	for _, workspaceName := range workspaceNames {
		workspacePath, err := resolveWorkspacePath(workspaceName)
		if err != nil {
			return err
		}
		docRootPath, err := resolveDocRepoPath(workspacePath, docHomeRepo, spec.Repos)
		if err != nil {
			return err
		}
		for _, ticket := range tickets {
			key := ticket + "|" + workspaceName
			state, ok := stateByTicketWorkspace[key]
			if !ok {
				return fmt.Errorf("workspace %s ticket %s missing doc sync state; close blocked", workspaceName, ticket)
			}
			if state.Status != model.DocSyncStatusSynced {
				return fmt.Errorf("workspace %s ticket %s doc sync status=%s; close blocked", workspaceName, ticket, state.Status)
			}
			if strings.TrimSpace(state.Revision) == "" {
				return fmt.Errorf("workspace %s ticket %s missing doc sync revision; close blocked", workspaceName, ticket)
			}
			ticketPaths, err := locateTicketDocDirsInWorkspace(docRootPath, ticket)
			if err != nil {
				return err
			}
			if len(ticketPaths) == 0 {
				return fmt.Errorf("workspace %s ticket %s docs missing under %s; close blocked", workspaceName, ticket, filepath.Join(docRootPath, "ttmp"))
			}
		}
		dirty, err := hasDirtyGitState(ctx, docRootPath)
		if err != nil {
			return err
		}
		if dirty {
			return fmt.Errorf("workspace %s doc home repo %s has uncommitted changes; close blocked", workspaceName, docRootPath)
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
		agentWorkdir, err := resolveDocRepoPath(workspacePath, effectiveDocHomeRepo(spec), spec.Repos)
		if err != nil {
			return err
		}
		command := agentCommands[step.Agent]
		if strings.TrimSpace(command) == "" {
			command = "bash"
		}
		command = normalizeAgentCommand(command)
		command = wrapAgentCommandForTmux(command)
		sessionName := policy.RenderSessionName(cfg.Tmux.SessionPattern, step.Agent, step.WorkspaceName)

		_ = tmuxKillSession(ctx, sessionName)
		tmuxCmd := fmt.Sprintf("tmux new-session -d -s %s -c %s %s", shellQuote(sessionName), shellQuote(agentWorkdir), shellQuote(command))
		if err := runShell(ctx, tmuxCmd); err != nil {
			return err
		}
		time.Sleep(200 * time.Millisecond)
		if !tmuxHasSession(ctx, sessionName) {
			return fmt.Errorf("tmux session %s exited immediately after start", sessionName)
		}
		if err := waitForAgentStartup(ctx, sessionName, 4*time.Second); err != nil {
			return err
		}
		now := time.Now()
		return s.store.UpdateAgentStatus(spec.RunID, step.Agent, step.WorkspaceName, model.AgentStatusRunning, model.HealthStateHealthy, &now, &now)
	case "ticket_context_sync":
		workspacePath, err := resolveWorkspacePath(step.WorkspaceName)
		if err != nil {
			return err
		}
		docHomeRepo := effectiveDocHomeRepo(spec)
		_ = s.store.UpsertDocSyncState(model.DocSyncState{
			RunID:            spec.RunID,
			Ticket:           step.Ticket,
			WorkspaceName:    step.WorkspaceName,
			DocHomeRepo:      docHomeRepo,
			DocAuthorityMode: string(spec.DocAuthorityMode),
			DocSeedMode:      string(spec.DocSeedMode),
			Status:           model.DocSyncStatusPending,
			UpdatedAt:        time.Now(),
		})
		revision, err := syncTicketDocsToWorkspace(ctx, step.Ticket, workspacePath, docHomeRepo, spec.Repos)
		if err != nil {
			_ = s.store.UpsertDocSyncState(model.DocSyncState{
				RunID:            spec.RunID,
				Ticket:           step.Ticket,
				WorkspaceName:    step.WorkspaceName,
				DocHomeRepo:      docHomeRepo,
				DocAuthorityMode: string(spec.DocAuthorityMode),
				DocSeedMode:      string(spec.DocSeedMode),
				Status:           model.DocSyncStatusFailed,
				ErrorText:        err.Error(),
				UpdatedAt:        time.Now(),
			})
			return err
		}
		if err := s.store.UpsertDocSyncState(model.DocSyncState{
			RunID:            spec.RunID,
			Ticket:           step.Ticket,
			WorkspaceName:    step.WorkspaceName,
			DocHomeRepo:      docHomeRepo,
			DocAuthorityMode: string(spec.DocAuthorityMode),
			DocSeedMode:      string(spec.DocSeedMode),
			Status:           model.DocSyncStatusSynced,
			Revision:         revision,
			UpdatedAt:        time.Now(),
		}); err != nil {
			return err
		}
		return s.store.UpdateRunDocFreshnessRevision(spec.RunID, revision)
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

		if normalizeDocSeedMode(string(spec.DocSeedMode)) == model.DocSeedModeCopyFromRepoOnStart {
			steps = append(steps, model.PlanStep{
				Index:         index,
				Name:          fmt.Sprintf("sync-ticket-context-%s", workspaceName),
				Kind:          "ticket_context_sync",
				Command:       "",
				Blocking:      true,
				Ticket:        ticket,
				WorkspaceName: workspaceName,
				Status:        model.StepStatusPending,
			})
			index++
		}

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

func resolveDocRepoPath(workspacePath string, docRepo string, repos []string) (string, error) {
	workspacePath = strings.TrimSpace(workspacePath)
	if workspacePath == "" {
		return "", fmt.Errorf("workspace path is required")
	}
	docRepo = normalizeDocRepo(docRepo, repos)
	if docRepo == "" {
		return workspacePath, nil
	}

	candidate := filepath.Join(workspacePath, docRepo)
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return candidate, nil
	}
	if len(repos) == 1 && isGitRepo(workspacePath) {
		return workspacePath, nil
	}
	return "", fmt.Errorf("doc repo path not found in workspace: %s", candidate)
}

func isGitRepo(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	if err != nil {
		return false
	}
	return info.IsDir() || info.Mode().IsRegular()
}

func syncTicketDocsToWorkspace(ctx context.Context, ticket string, workspacePath string, docRepo string, repos []string) (string, error) {
	sourcePath, relativePath, err := resolveTicketDocPath(ctx, ticket)
	if err != nil {
		return "", err
	}
	docRootPath, err := resolveDocRepoPath(workspacePath, docRepo, repos)
	if err != nil {
		return "", err
	}
	if err := syncTicketDocsDirectory(sourcePath, relativePath, docRootPath); err != nil {
		return "", err
	}
	return strconv.FormatInt(time.Now().UTC().UnixNano(), 10), nil
}

func resolveTicketDocPath(ctx context.Context, ticket string) (string, string, error) {
	ticket = strings.TrimSpace(ticket)
	if ticket == "" {
		return "", "", fmt.Errorf("ticket is required for context sync")
	}
	cmd := exec.CommandContext(ctx, "docmgr", "ticket", "list", "--ticket", ticket)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("docmgr ticket list --ticket %s failed: %w: %s", ticket, err, strings.TrimSpace(string(out)))
	}
	docsRoot, ticketRelativePath, err := parseDocmgrTicketListPaths(out)
	if err != nil {
		return "", "", err
	}
	ticketPath := filepath.Join(docsRoot, ticketRelativePath)
	info, err := os.Stat(ticketPath)
	if err != nil {
		return "", "", fmt.Errorf("ticket path %s: %w", ticketPath, err)
	}
	if !info.IsDir() {
		return "", "", fmt.Errorf("ticket path %s is not a directory", ticketPath)
	}
	return ticketPath, ticketRelativePath, nil
}

func parseDocmgrTicketListPaths(output []byte) (string, string, error) {
	text := string(output)
	docsRootMatch := docmgrDocsRootRegex.FindStringSubmatch(text)
	if len(docsRootMatch) < 2 {
		return "", "", fmt.Errorf("unable to parse docs root from docmgr ticket list output")
	}
	ticketPathMatch := docmgrTicketPathRegex.FindStringSubmatch(text)
	if len(ticketPathMatch) < 2 {
		return "", "", fmt.Errorf("unable to parse ticket path from docmgr ticket list output")
	}

	docsRoot := filepath.Clean(strings.TrimSpace(docsRootMatch[1]))
	relativePath := filepath.Clean(filepath.FromSlash(strings.TrimSpace(ticketPathMatch[1])))
	if relativePath == "." || relativePath == "" {
		return "", "", fmt.Errorf("parsed empty ticket path from docmgr ticket list output")
	}
	if relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(os.PathSeparator)) {
		return "", "", fmt.Errorf("unsafe ticket path parsed from docmgr output: %s", relativePath)
	}
	return docsRoot, relativePath, nil
}

func syncTicketDocsDirectory(sourcePath string, ticketRelativePath string, docRootPath string) error {
	if strings.TrimSpace(sourcePath) == "" {
		return fmt.Errorf("source ticket path is required")
	}
	if strings.TrimSpace(ticketRelativePath) == "" {
		return fmt.Errorf("ticket relative path is required")
	}
	if strings.TrimSpace(docRootPath) == "" {
		return fmt.Errorf("doc root path is required")
	}
	destinationPath := filepath.Join(docRootPath, "ttmp", ticketRelativePath)
	if err := os.RemoveAll(destinationPath); err != nil {
		return fmt.Errorf("remove destination ticket path %s: %w", destinationPath, err)
	}
	if err := copyDirectoryTree(sourcePath, destinationPath); err != nil {
		return fmt.Errorf("copy ticket docs %s -> %s: %w", sourcePath, destinationPath, err)
	}
	return nil
}

func copyDirectoryTree(sourcePath string, destinationPath string) error {
	sourcePath = filepath.Clean(sourcePath)
	destinationPath = filepath.Clean(destinationPath)
	return filepath.WalkDir(sourcePath, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relativePath, err := filepath.Rel(sourcePath, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(destinationPath, relativePath)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(targetPath, info.Mode().Perm())
		}
		if entry.Type()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(linkTarget, targetPath)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("unsupported file type at %s", path)
		}
		return copyFile(path, targetPath, info.Mode().Perm())
	})
}

func copyFile(sourcePath string, destinationPath string, mode os.FileMode) error {
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
		return err
	}

	destinationFile, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(destinationFile, sourceFile); err != nil {
		_ = destinationFile.Close()
		return err
	}
	if err := destinationFile.Close(); err != nil {
		return err
	}
	return nil
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

func normalizeAgentCommand(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return "bash"
	}
	if strings.Contains(command, "codex exec") && !strings.Contains(command, "--skip-git-repo-check") {
		command = strings.Replace(command, "codex exec", "codex exec --skip-git-repo-check", 1)
	}
	return command
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

type repoDirtyState struct {
	RepoPath string
	Dirty    bool
}

func workspaceRepoDirtyStates(ctx context.Context, workspacePath string, repos []string) ([]repoDirtyState, error) {
	repoPaths, err := workspaceRepoPaths(workspacePath, repos)
	if err != nil {
		return nil, err
	}
	states := make([]repoDirtyState, 0, len(repoPaths))
	for _, repoPath := range repoPaths {
		dirty, err := hasDirtyGitState(ctx, repoPath)
		if err != nil {
			return nil, err
		}
		states = append(states, repoDirtyState{RepoPath: repoPath, Dirty: dirty})
	}
	return states, nil
}

type workspaceDiff struct {
	WorkspaceName string
	WorkspacePath string
	Repos         []repoDiff
	Error         error
}

type repoDiff struct {
	RepoPath    string
	RepoLabel   string
	StatusLines []string
	Error       error
}

func collectWorkspaceDiffs(ctx context.Context, workspaceNames []string, repos []string) []workspaceDiff {
	out := make([]workspaceDiff, 0, len(workspaceNames))
	for _, workspaceName := range workspaceNames {
		diff := workspaceDiff{WorkspaceName: workspaceName}
		workspacePath, err := resolveWorkspacePath(workspaceName)
		if err != nil {
			diff.Error = err
			out = append(out, diff)
			continue
		}
		diff.WorkspacePath = workspacePath
		repoPaths, err := workspaceRepoPaths(workspacePath, repos)
		if err != nil {
			diff.Error = err
			out = append(out, diff)
			continue
		}
		sort.Strings(repoPaths)
		repoDiffs := make([]repoDiff, 0, len(repoPaths))
		for _, repoPath := range repoPaths {
			label := repoLabelForWorkspace(workspacePath, repoPath)
			lines, err := gitStatusShortLines(ctx, repoPath)
			repoDiffs = append(repoDiffs, repoDiff{
				RepoPath:    repoPath,
				RepoLabel:   label,
				StatusLines: lines,
				Error:       err,
			})
		}
		diff.Repos = repoDiffs
		out = append(out, diff)
	}
	return out
}

func latestProgressFromWorkspaceDiffs(diffs []workspaceDiff) map[string]time.Time {
	out := map[string]time.Time{}
	for _, diff := range diffs {
		if diff.Error != nil {
			continue
		}
		latest, ok := latestProgressFromRepoDiffs(diff.Repos)
		if !ok {
			continue
		}
		existing, exists := out[diff.WorkspaceName]
		if !exists || latest.After(existing) {
			out[diff.WorkspaceName] = latest
		}
	}
	return out
}

func latestProgressFromRepoDiffs(repos []repoDiff) (time.Time, bool) {
	latest := time.Time{}
	found := false
	for _, repo := range repos {
		if repo.Error != nil {
			continue
		}
		for _, line := range repo.StatusLines {
			relPath := parseGitStatusPath(line)
			if relPath == "" {
				continue
			}
			path := filepath.Join(repo.RepoPath, relPath)
			info, err := os.Stat(path)
			if err != nil {
				continue
			}
			modifiedAt := info.ModTime()
			if !found || modifiedAt.After(latest) {
				latest = modifiedAt
				found = true
			}
		}
	}
	return latest, found
}

func parseGitStatusPath(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	if strings.Contains(line, " -> ") {
		parts := strings.Split(line, " -> ")
		return strings.TrimSpace(parts[len(parts)-1])
	}
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return ""
	}
	return strings.Join(parts[1:], " ")
}

func gitStatusShortLines(ctx context.Context, repoPath string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "zsh", "-lc", fmt.Sprintf("git -C %s status --short", shellQuote(repoPath)))
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(string(out))
	if text == "" {
		return nil, nil
	}
	lines := strings.Split(text, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	return result, nil
}

func repoLabelForWorkspace(workspacePath string, repoPath string) string {
	rel, err := filepath.Rel(workspacePath, repoPath)
	if err == nil && strings.TrimSpace(rel) != "" && rel != "." {
		return rel
	}
	return filepath.Base(repoPath)
}

func workspaceNamesFromAgents(agents []model.AgentRecord) []string {
	workspaceSet := map[string]struct{}{}
	for _, agent := range agents {
		name := strings.TrimSpace(agent.WorkspaceName)
		if name == "" {
			continue
		}
		workspaceSet[name] = struct{}{}
	}
	workspaces := make([]string, 0, len(workspaceSet))
	for workspaceName := range workspaceSet {
		workspaces = append(workspaces, workspaceName)
	}
	sort.Strings(workspaces)
	return workspaces
}

func recordIterationFeedback(workspacePath string, docRepo string, repos []string, tickets []string, feedback string, at time.Time, dryRun bool) ([]string, error) {
	actions := []string{}
	signalFiles := []string{
		filepath.Join(workspacePath, ".metawsm", "implementation-complete.json"),
		filepath.Join(workspacePath, ".metawsm", "validation-result.json"),
		filepath.Join(workspacePath, ".metawsm", "guidance-request.json"),
		filepath.Join(workspacePath, ".metawsm", "guidance-response.json"),
	}
	for _, path := range signalFiles {
		actions = append(actions, fmt.Sprintf("rm -f %s", shellQuote(path)))
		if dryRun {
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}

	mainFeedbackPath := filepath.Join(workspacePath, ".metawsm", "operator-feedback.md")
	actions = append(actions, fmt.Sprintf("append feedback %s", shellQuote(mainFeedbackPath)))
	if !dryRun {
		if err := appendIterationFeedback(mainFeedbackPath, feedback, at); err != nil {
			return nil, err
		}
	}

	docRootPath, err := resolveDocRepoPath(workspacePath, docRepo, repos)
	if err != nil {
		return nil, err
	}
	for _, ticket := range tickets {
		ticketPaths, err := locateTicketDocDirsInWorkspace(docRootPath, ticket)
		if err != nil {
			return nil, err
		}
		for _, ticketPath := range ticketPaths {
			referencePath := filepath.Join(ticketPath, "reference", "99-operator-feedback.md")
			actions = append(actions, fmt.Sprintf("append feedback %s", shellQuote(referencePath)))
			if dryRun {
				continue
			}
			if err := appendIterationFeedback(referencePath, feedback, at); err != nil {
				return nil, err
			}
		}
	}
	return actions, nil
}

func appendIterationFeedback(path string, feedback string, at time.Time) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	_, statErr := os.Stat(path)
	exists := statErr == nil
	if statErr != nil && !os.IsNotExist(statErr) {
		return statErr
	}

	var b strings.Builder
	if !exists {
		b.WriteString("# Operator Feedback\n\n")
	}
	b.WriteString("## ")
	b.WriteString(at.Format(time.RFC3339))
	b.WriteString("\n\n")
	b.WriteString(feedback)
	if !strings.HasSuffix(feedback, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("\n")

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(b.String()); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func locateTicketDocDirsInWorkspace(docRootPath string, ticket string) ([]string, error) {
	root := filepath.Join(docRootPath, "ttmp")
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, nil
	}

	prefix := strings.ToLower(strings.TrimSpace(ticket))
	if prefix == "" {
		return nil, nil
	}
	prefix += "--"

	paths := []string{}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}
		if strings.HasPrefix(strings.ToLower(entry.Name()), prefix) {
			paths = append(paths, path)
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

func evaluateHealth(ctx context.Context, cfg policy.Config, agent model.AgentRecord, now time.Time) (model.HealthState, model.AgentStatus, *time.Time, *time.Time) {
	hasSession := tmuxHasSession(ctx, agent.SessionName)
	if !hasSession {
		return model.HealthStateDead, model.AgentStatusDead, agent.LastActivityAt, agent.LastProgressAt
	}
	if exitCode, found := readAgentExitCode(ctx, agent.SessionName); found {
		if exitCode != 0 {
			return model.HealthStateDead, model.AgentStatusFailed, agent.LastActivityAt, agent.LastProgressAt
		}
		return model.HealthStateIdle, model.AgentStatusIdle, agent.LastActivityAt, agent.LastProgressAt
	}

	activityEpoch := fetchSessionActivity(ctx, agent.SessionName)
	lastActivity := agent.LastActivityAt
	if activityEpoch > 0 {
		t := time.Unix(activityEpoch, 0)
		lastActivity = &t
	}
	lastProgress := agent.LastProgressAt
	if lastActivity != nil {
		if lastProgress == nil || lastActivity.After(*lastProgress) {
			t := *lastActivity
			lastProgress = &t
		}
	}

	activityAge := time.Duration(0)
	if lastActivity != nil {
		activityAge = now.Sub(*lastActivity)
	}

	progressAge := time.Duration(0)
	if lastProgress != nil {
		progressAge = now.Sub(*lastProgress)
	}

	idleThreshold := time.Duration(cfg.Health.IdleSeconds) * time.Second
	activityStalledThreshold := time.Duration(cfg.Health.ActivityStalledSeconds) * time.Second
	progressStalledThreshold := time.Duration(cfg.Health.ProgressStalledSeconds) * time.Second

	if activityAge >= activityStalledThreshold || (progressAge >= progressStalledThreshold && progressAge > 0) {
		return model.HealthStateStalled, model.AgentStatusStalled, lastActivity, lastProgress
	}
	if activityAge >= idleThreshold {
		return model.HealthStateIdle, model.AgentStatusIdle, lastActivity, lastProgress
	}
	return model.HealthStateHealthy, model.AgentStatusRunning, lastActivity, lastProgress
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

var agentExitRegex = regexp.MustCompile(`\[metawsm\] agent command exited with status ([0-9]+)`)
var docmgrDocsRootRegex = regexp.MustCompile("Docs root:\\s+`([^`]+)`")
var docmgrTicketPathRegex = regexp.MustCompile("Path:\\s+`([^`]+)`")

func readAgentExitCode(ctx context.Context, sessionName string) (int, bool) {
	cmd := exec.CommandContext(ctx, "zsh", "-lc", fmt.Sprintf("tmux capture-pane -p -t %s:0 | tail -n 200", shellQuote(sessionName)))
	out, err := cmd.Output()
	if err != nil {
		return 0, false
	}
	return parseAgentExitCode(string(out))
}

func parseAgentExitCode(output string) (int, bool) {
	matches := agentExitRegex.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return 0, false
	}
	last := matches[len(matches)-1]
	if len(last) < 2 {
		return 0, false
	}
	code, err := strconv.Atoi(strings.TrimSpace(last[1]))
	if err != nil {
		return 0, false
	}
	return code, true
}

func waitForAgentStartup(ctx context.Context, sessionName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if !tmuxHasSession(ctx, sessionName) {
			return fmt.Errorf("tmux session %s exited during startup", sessionName)
		}
		if exitCode, found := readAgentExitCode(ctx, sessionName); found {
			if exitCode != 0 {
				return fmt.Errorf("agent command in %s exited with status %d", sessionName, exitCode)
			}
			return nil
		}
		if time.Now().After(deadline) {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func guidanceKey(workspaceName string, agentName string, question string) string {
	return workspaceName + "|" + agentName + "|" + strings.TrimSpace(question)
}

func bootstrapSignalRoots(workspacePath string, spec model.RunSpec) []string {
	roots := []string{workspacePath}
	docRoot, err := resolveDocRepoPath(workspacePath, effectiveDocHomeRepo(spec), spec.Repos)
	if err != nil {
		return roots
	}
	docRoot = filepath.Clean(strings.TrimSpace(docRoot))
	workspacePath = filepath.Clean(strings.TrimSpace(workspacePath))
	if docRoot != "" && docRoot != workspacePath {
		roots = append(roots, docRoot)
	}
	return roots
}

func firstSignalRootWithFile(roots []string, filename string) string {
	for _, root := range roots {
		path := filepath.Join(root, ".metawsm", filename)
		if _, err := os.Stat(path); err == nil {
			return root
		}
	}
	return ""
}

func readGuidanceRequestFile(workspacePath string) (model.GuidanceRequestPayload, bool) {
	path := filepath.Join(workspacePath, ".metawsm", "guidance-request.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return model.GuidanceRequestPayload{}, false
	}
	var raw struct {
		RunID    string          `json:"run_id,omitempty"`
		Agent    string          `json:"agent,omitempty"`
		Question string          `json:"question"`
		Context  json.RawMessage `json:"context,omitempty"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return model.GuidanceRequestPayload{}, false
	}
	payload := model.GuidanceRequestPayload{
		RunID:    strings.TrimSpace(raw.RunID),
		Agent:    strings.TrimSpace(raw.Agent),
		Question: strings.TrimSpace(raw.Question),
		Context:  parseGuidanceContext(raw.Context),
	}
	if strings.TrimSpace(payload.Question) == "" {
		return model.GuidanceRequestPayload{}, false
	}
	return payload, true
}

func readGuidanceRequestFileFromRoots(roots []string) (model.GuidanceRequestPayload, bool) {
	for _, root := range roots {
		if payload, ok := readGuidanceRequestFile(root); ok {
			return payload, true
		}
	}
	return model.GuidanceRequestPayload{}, false
}

func parseGuidanceContext(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return strings.TrimSpace(asString)
	}

	var generic any
	if err := json.Unmarshal(raw, &generic); err == nil {
		encoded, err := json.Marshal(generic)
		if err == nil {
			return strings.TrimSpace(string(encoded))
		}
	}

	return trimmed
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

func hasCompletionSignalFromRoots(roots []string, runID string, agentName string) bool {
	for _, root := range roots {
		if hasCompletionSignal(root, runID, agentName) {
			return true
		}
	}
	return false
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

func readValidationResultFromRoots(roots []string) (struct {
	RunID        string `json:"run_id,omitempty"`
	Status       string `json:"status"`
	DoneCriteria string `json:"done_criteria"`
}, bool) {
	for _, root := range roots {
		if payload, ok := readValidationResult(root); ok {
			return payload, true
		}
	}
	return struct {
		RunID        string `json:"run_id,omitempty"`
		Status       string `json:"status"`
		DoneCriteria string `json:"done_criteria"`
	}{}, false
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func generateRunID() string {
	return "run-" + time.Now().Format("20060102-150405")
}

func resolveDocHomeRepo(docHomeRepo string, legacyDocRepo string, repos []string) (string, error) {
	docHomeRepo = strings.TrimSpace(docHomeRepo)
	legacyDocRepo = strings.TrimSpace(legacyDocRepo)
	if docHomeRepo != "" && legacyDocRepo != "" && !strings.EqualFold(docHomeRepo, legacyDocRepo) {
		return "", fmt.Errorf("doc home repo %q conflicts with --doc-repo value %q", docHomeRepo, legacyDocRepo)
	}
	switch {
	case docHomeRepo != "":
		return docHomeRepo, nil
	case legacyDocRepo != "":
		return legacyDocRepo, nil
	default:
		return normalizeDocRepo("", repos), nil
	}
}

func normalizeDocRepo(docRepo string, repos []string) string {
	docRepo = strings.TrimSpace(docRepo)
	if docRepo != "" {
		return docRepo
	}
	if len(repos) == 0 {
		return ""
	}
	return strings.TrimSpace(repos[0])
}

func effectiveDocHomeRepo(spec model.RunSpec) string {
	docHomeRepo := strings.TrimSpace(spec.DocHomeRepo)
	if docHomeRepo != "" {
		return docHomeRepo
	}
	return normalizeDocRepo(spec.DocRepo, spec.Repos)
}

func normalizeDocAuthorityMode(value string) model.DocAuthorityMode {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return model.DocAuthorityMode(value)
}

func normalizeDocSeedMode(value string) model.DocSeedMode {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return model.DocSeedMode(value)
}

func isValidDocAuthorityMode(mode model.DocAuthorityMode) bool {
	return mode == model.DocAuthorityModeWorkspaceActive
}

func isValidDocSeedMode(mode model.DocSeedMode) bool {
	switch mode {
	case model.DocSeedModeNone, model.DocSeedModeCopyFromRepoOnStart:
		return true
	default:
		return false
	}
}

func latestDocFreshnessRevision(states []model.DocSyncState) string {
	latestRevision := ""
	latestUpdated := time.Time{}
	for _, state := range states {
		if strings.TrimSpace(state.Revision) == "" {
			continue
		}
		if state.UpdatedAt.After(latestUpdated) {
			latestUpdated = state.UpdatedAt
			latestRevision = state.Revision
		}
	}
	return latestRevision
}

func docFreshnessWarnings(states []model.DocSyncState, seedMode model.DocSeedMode, staleWarningSeconds int, now time.Time) []string {
	if staleWarningSeconds <= 0 {
		staleWarningSeconds = 900
	}
	if seedMode != model.DocSeedModeCopyFromRepoOnStart {
		return nil
	}
	warnings := []string{}
	haveSynced := false
	latestSyncedAt := time.Time{}
	for _, state := range states {
		if state.Status == model.DocSyncStatusFailed {
			warnings = append(warnings, fmt.Sprintf("doc seed failed for %s/%s", state.Ticket, state.WorkspaceName))
		}
		if state.Status != model.DocSyncStatusSynced {
			continue
		}
		haveSynced = true
		if state.UpdatedAt.After(latestSyncedAt) {
			latestSyncedAt = state.UpdatedAt
		}
	}
	if !haveSynced {
		return append(warnings, "docmgr index freshness unavailable for copy_from_repo_on_start")
	}
	if now.Sub(latestSyncedAt) > time.Duration(staleWarningSeconds)*time.Second {
		warnings = append(warnings, fmt.Sprintf("docmgr index freshness stale (last seed %s)", latestSyncedAt.Format(time.RFC3339)))
	}
	return warnings
}

func emptyAsUnknown(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	return value
}

func valueOrDefault(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func formatTimeOrDash(value *time.Time) string {
	if value == nil || value.IsZero() {
		return "-"
	}
	return value.Format(time.RFC3339)
}

func formatAgeOrDash(now time.Time, value *time.Time) string {
	if value == nil || value.IsZero() {
		return "-"
	}
	age := now.Sub(*value)
	if age < 0 {
		age = 0
	}
	return age.Truncate(time.Second).String()
}

func containsToken(values []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), target) {
			return true
		}
	}
	return false
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
