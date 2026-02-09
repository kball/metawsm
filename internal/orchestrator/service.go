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

	"metawsm/internal/forumbus"
	"metawsm/internal/hsm"
	"metawsm/internal/model"
	"metawsm/internal/policy"
	"metawsm/internal/store"
)

type Service struct {
	store           *store.SQLiteStore
	forumBus        *forumbus.Runtime
	forumDispatcher forumCommandDispatcher
	forumTopics     model.ForumTopicRegistry
}

type RunMutationInProgressError struct {
	RunID     string
	Operation string
	LockPath  string
	Holder    string
}

func (e *RunMutationInProgressError) Error() string {
	base := fmt.Sprintf("run %s %s operation is already in progress", strings.TrimSpace(e.RunID), strings.TrimSpace(e.Operation))
	if strings.TrimSpace(e.LockPath) != "" {
		base += fmt.Sprintf(" (lock=%s)", strings.TrimSpace(e.LockPath))
	}
	if strings.TrimSpace(e.Holder) != "" {
		base += fmt.Sprintf("; holder=%s", strings.TrimSpace(e.Holder))
	}
	return base
}

func NewService(dbPath string) (*Service, error) {
	sqliteStore := store.NewSQLiteStore(dbPath)
	if err := sqliteStore.Init(); err != nil {
		return nil, err
	}
	cfg, _, err := policy.Load("")
	if err != nil {
		cfg = policy.Default()
	}
	busRuntime := forumbus.NewRuntime(sqliteStore, cfg)
	if err := busRuntime.Start(context.Background()); err != nil {
		return nil, err
	}
	topics := model.ForumTopicRegistry{
		CommandPrefix:     strings.TrimSpace(cfg.Forum.Topics.CommandPrefix),
		EventPrefix:       strings.TrimSpace(cfg.Forum.Topics.EventPrefix),
		IntegrationPrefix: strings.TrimSpace(cfg.Forum.Topics.IntegrationPrefix),
	}
	if topics.CommandPrefix == "" || topics.EventPrefix == "" || topics.IntegrationPrefix == "" {
		topics = model.DefaultForumTopicRegistry()
	}
	service := &Service{
		store:           sqliteStore,
		forumBus:        busRuntime,
		forumDispatcher: newBusForumCommandDispatcher(busRuntime),
		forumTopics:     topics,
	}
	if err := service.registerForumBusHandlers(); err != nil {
		return nil, err
	}
	return service, nil
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

type CommitOptions struct {
	Ticket  string
	RunID   string
	Message string
	Actor   string
	DryRun  bool
}

type CommitResult struct {
	RunID string
	Repos []CommitRepoResult
}

type CommitRepoResult struct {
	Ticket        string
	WorkspaceName string
	Repo          string
	RepoPath      string
	BaseBranch    string
	BaseRef       string
	Branch        string
	CommitMessage string
	CommitSHA     string
	Actor         string
	ActorSource   string
	Dirty         bool
	SkippedReason string
	Preflight     []string
	Actions       []string
}

type PullRequestOptions struct {
	Ticket string
	RunID  string
	Title  string
	Body   string
	Actor  string
	DryRun bool
}

type PullRequestResult struct {
	RunID string
	Repos []PullRequestRepoResult
}

type PullRequestRepoResult struct {
	Ticket        string
	WorkspaceName string
	Repo          string
	RepoPath      string
	HeadBranch    string
	BaseBranch    string
	Title         string
	Body          string
	Actor         string
	ActorSource   string
	PRNumber      int
	PRURL         string
	PRState       model.PullRequestState
	SkippedReason string
	Preflight     []string
	Actions       []string
}

type ReviewFeedbackSyncOptions struct {
	Ticket   string
	RunID    string
	MaxItems int
	DryRun   bool
}

type ReviewFeedbackSyncResult struct {
	RunID   string
	Repos   []ReviewFeedbackSyncRepoResult
	Added   int
	Updated int
}

type ReviewFeedbackSyncRepoResult struct {
	Ticket        string
	Repo          string
	PRNumber      int
	PRURL         string
	Fetched       int
	Added         int
	Updated       int
	SkippedReason string
	Actions       []string
}

type ReviewFeedbackDispatchOptions struct {
	Ticket   string
	RunID    string
	MaxItems int
	DryRun   bool
}

type ReviewFeedbackDispatchResult struct {
	RunID       string
	QueuedCount int
	Feedback    string
	Actions     []string
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
		if tmuxSessionProbe(ctx, sessionName) == tmuxSessionMissing {
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

func (s *Service) Commit(ctx context.Context, options CommitOptions) (CommitResult, error) {
	runID, err := s.resolveRunID(options.RunID, options.Ticket)
	if err != nil {
		return CommitResult{}, err
	}

	record, specJSON, policyJSON, err := s.store.GetRun(runID)
	if err != nil {
		return CommitResult{}, err
	}
	if record.Status != model.RunStatusComplete {
		return CommitResult{}, fmt.Errorf("run %s must be completed before commit (current: %s)", runID, record.Status)
	}

	var spec model.RunSpec
	if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
		return CommitResult{}, fmt.Errorf("unmarshal run spec: %w", err)
	}
	cfg := policy.Default()
	if strings.TrimSpace(policyJSON) != "" {
		if err := json.Unmarshal([]byte(policyJSON), &cfg); err != nil {
			return CommitResult{}, fmt.Errorf("unmarshal run policy: %w", err)
		}
	}
	if strings.EqualFold(strings.TrimSpace(cfg.GitPR.Mode), "off") {
		return CommitResult{}, fmt.Errorf("git_pr.mode is off; commit workflow disabled")
	}
	if !options.DryRun {
		releaseLock, err := s.acquireRunMutationLock(runID, "commit")
		if err != nil {
			return CommitResult{}, err
		}
		defer releaseLock()
	}

	tickets := normalizeTokens(spec.Tickets)
	if len(tickets) == 0 {
		return CommitResult{}, fmt.Errorf("run %s has no tickets", runID)
	}
	selectedTickets := append([]string(nil), tickets...)
	if commitTicket := strings.TrimSpace(options.Ticket); commitTicket != "" {
		if !containsToken(tickets, commitTicket) {
			return CommitResult{}, fmt.Errorf("ticket %q is not part of run %s", commitTicket, runID)
		}
		selectedTickets = []string{commitTicket}
	}

	repos := normalizeTokens(spec.Repos)
	if len(repos) == 0 && strings.TrimSpace(effectiveDocHomeRepo(spec)) != "" {
		repos = []string{strings.TrimSpace(effectiveDocHomeRepo(spec))}
	}
	repos = filterReposByAllowedList(repos, cfg.GitPR.AllowedRepos)
	if len(repos) == 0 {
		return CommitResult{}, fmt.Errorf("no repositories are eligible for commit under current policy")
	}

	baseBranch := normalizeBaseBranch(spec.BaseBranch)
	if baseBranch == "" {
		baseBranch = normalizeBaseBranch(cfg.Workspace.BaseBranch)
	}
	if baseBranch == "" {
		baseBranch = "main"
	}

	workspaces, err := s.store.GetAgents(runID)
	if err != nil {
		return CommitResult{}, err
	}
	workspaceNames := workspaceNamesFromAgents(workspaces)
	if len(workspaceNames) == 0 {
		return CommitResult{}, fmt.Errorf("run %s has no workspaces to commit", runID)
	}
	workspaceTickets, err := s.resolveWorkspaceTickets(runID)
	if err != nil {
		return CommitResult{}, err
	}

	credentialMode := strings.TrimSpace(cfg.GitPR.CredentialMode)
	if credentialMode == "" {
		credentialMode = "local_user_auth"
	}
	requestedMessage := strings.TrimSpace(options.Message)

	existingPRs, err := s.store.ListRunPullRequests(runID)
	if err != nil {
		return CommitResult{}, err
	}
	existingByKey := map[string]model.RunPullRequest{}
	for _, item := range existingPRs {
		key := strings.TrimSpace(item.Ticket) + "|" + strings.TrimSpace(item.Repo)
		existingByKey[key] = item
	}

	results := make([]CommitRepoResult, 0, len(workspaceNames)*len(repos))
	now := time.Now()
	for _, workspaceName := range workspaceNames {
		workspaceTicket := strings.TrimSpace(workspaceTickets[workspaceName])
		if workspaceTicket == "" {
			if len(selectedTickets) == 1 {
				workspaceTicket = selectedTickets[0]
			} else {
				return CommitResult{}, fmt.Errorf("workspace %s is missing a ticket mapping for run %s", workspaceName, runID)
			}
		}
		if !containsToken(selectedTickets, workspaceTicket) {
			continue
		}
		commitMessage := requestedMessage
		if commitMessage == "" {
			commitMessage = s.defaultCommitMessage(runID, workspaceTicket)
		}

		workspacePath, err := resolveWorkspacePath(workspaceName)
		if err != nil {
			return CommitResult{}, err
		}
		targets, err := resolveWorkspaceCommitRepoTargets(workspacePath, repos)
		if err != nil {
			return CommitResult{}, err
		}
		for _, target := range targets {
			resolvedActor, actorSource := resolveOperationActor(ctx, options.Actor, target.RepoPath)
			result := CommitRepoResult{
				Ticket:        workspaceTicket,
				WorkspaceName: workspaceName,
				Repo:          target.Repo,
				RepoPath:      target.RepoPath,
				BaseBranch:    baseBranch,
				CommitMessage: commitMessage,
				Actor:         resolvedActor,
				ActorSource:   actorSource,
			}
			dirty, err := hasDirtyGitState(ctx, target.RepoPath)
			if err != nil {
				return CommitResult{}, err
			}
			result.Dirty = dirty
			if !dirty {
				result.SkippedReason = "clean working tree"
				results = append(results, result)
				continue
			}
			validationReport, err := runGitPRValidations(ctx, cfg, gitPRValidationInput{
				Operation:     gitPRValidationOperationCommit,
				RunID:         runID,
				Ticket:        workspaceTicket,
				WorkspaceName: workspaceName,
				Repo:          target.Repo,
				RepoPath:      target.RepoPath,
				BaseBranch:    baseBranch,
			})
			if err != nil {
				return CommitResult{}, fmt.Errorf("commit validation failed for ticket=%s workspace=%s repo=%s: %w", workspaceTicket, workspaceName, target.Repo, err)
			}
			validationJSON := marshalGitPRValidationReport(validationReport)

			baseRef, err := resolveCommitBaseRef(ctx, target.RepoPath, baseBranch)
			if err != nil {
				return CommitResult{}, err
			}
			branchName := policy.RenderGitBranch(cfg.GitPR.BranchTemplate, workspaceTicket, target.Repo, runID)
			result.BaseRef = baseRef
			result.Branch = branchName
			preflight, err := collectCommitPreflight(ctx, target.RepoPath, baseRef)
			if err != nil {
				return CommitResult{}, err
			}
			result.Preflight = preflight
			result.Actions = commitActionsPreview(target.RepoPath, branchName, baseRef, commitMessage, true)
			if options.DryRun {
				results = append(results, result)
				continue
			}

			if err := prepareCommitBranch(ctx, target.RepoPath, branchName, baseRef, true); err != nil {
				return CommitResult{}, err
			}
			if _, err := runGitCommand(ctx, target.RepoPath, "add", "-A"); err != nil {
				return CommitResult{}, err
			}
			if _, err := runGitCommand(ctx, target.RepoPath, "commit", "-m", commitMessage); err != nil {
				return CommitResult{}, err
			}
			sha, err := runGitCommand(ctx, target.RepoPath, "rev-parse", "HEAD")
			if err != nil {
				return CommitResult{}, err
			}
			result.CommitSHA = sha
			results = append(results, result)

			key := workspaceTicket + "|" + target.Repo
			row := existingByKey[key]
			if row.CreatedAt.IsZero() {
				row.CreatedAt = now
			}
			row.RunID = runID
			row.Ticket = workspaceTicket
			row.Repo = target.Repo
			row.WorkspaceName = workspaceName
			row.HeadBranch = branchName
			row.BaseBranch = baseBranch
			row.CommitSHA = sha
			row.CredentialMode = credentialMode
			row.Actor = resolvedActor
			row.ValidationJSON = validationJSON
			if row.PRState == "" {
				row.PRState = model.PullRequestStateDraft
			}
			row.ErrorText = ""
			row.UpdatedAt = now
			if err := s.store.UpsertRunPullRequest(row); err != nil {
				return CommitResult{}, err
			}
			existingByKey[key] = row

			message := fmt.Sprintf("ticket=%s workspace=%s repo=%s branch=%s commit=%s credential_mode=%s actor=%s actor_source=%s",
				workspaceTicket, workspaceName, target.Repo, branchName, sha, credentialMode, resolvedActor, actorSource)
			_ = s.store.AddEvent(runID, "repo", target.Repo, "commit_created", "", sha, message)
			if err := s.transitionReviewFeedbackStatus(runID, workspaceTicket, target.Repo, model.ReviewFeedbackStatusQueued, model.ReviewFeedbackStatusNew, nil); err != nil {
				return CommitResult{}, err
			}
		}
	}

	return CommitResult{
		RunID: runID,
		Repos: results,
	}, nil
}

func (s *Service) OpenPullRequests(ctx context.Context, options PullRequestOptions) (PullRequestResult, error) {
	runID, err := s.resolveRunID(options.RunID, options.Ticket)
	if err != nil {
		return PullRequestResult{}, err
	}

	record, specJSON, policyJSON, err := s.store.GetRun(runID)
	if err != nil {
		return PullRequestResult{}, err
	}
	if record.Status != model.RunStatusComplete {
		return PullRequestResult{}, fmt.Errorf("run %s must be completed before opening pull requests (current: %s)", runID, record.Status)
	}

	var spec model.RunSpec
	if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
		return PullRequestResult{}, fmt.Errorf("unmarshal run spec: %w", err)
	}
	cfg := policy.Default()
	if strings.TrimSpace(policyJSON) != "" {
		if err := json.Unmarshal([]byte(policyJSON), &cfg); err != nil {
			return PullRequestResult{}, fmt.Errorf("unmarshal run policy: %w", err)
		}
	}
	if strings.EqualFold(strings.TrimSpace(cfg.GitPR.Mode), "off") {
		return PullRequestResult{}, fmt.Errorf("git_pr.mode is off; pull request workflow disabled")
	}
	if !options.DryRun {
		releaseLock, err := s.acquireRunMutationLock(runID, "pr")
		if err != nil {
			return PullRequestResult{}, err
		}
		defer releaseLock()
	}

	tickets := normalizeTokens(spec.Tickets)
	if len(tickets) == 0 {
		return PullRequestResult{}, fmt.Errorf("run %s has no tickets", runID)
	}
	selectedTickets := append([]string(nil), tickets...)
	if prTicket := strings.TrimSpace(options.Ticket); prTicket != "" {
		if !containsToken(tickets, prTicket) {
			return PullRequestResult{}, fmt.Errorf("ticket %q is not part of run %s", prTicket, runID)
		}
		selectedTickets = []string{prTicket}
	}

	allowedRepos := normalizeTokens(cfg.GitPR.AllowedRepos)
	credentialMode := strings.TrimSpace(cfg.GitPR.CredentialMode)
	if credentialMode == "" {
		credentialMode = "local_user_auth"
	}

	brief, _ := s.store.GetRunBrief(runID)
	summary := defaultPRSummary(runID, brief)

	rows, err := s.store.ListRunPullRequests(runID)
	if err != nil {
		return PullRequestResult{}, err
	}
	candidates := make([]model.RunPullRequest, 0, len(rows))
	for _, row := range rows {
		if !containsToken(selectedTickets, row.Ticket) {
			continue
		}
		if len(allowedRepos) > 0 && !containsToken(allowedRepos, row.Repo) {
			continue
		}
		candidates = append(candidates, row)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Ticket == candidates[j].Ticket {
			if candidates[i].WorkspaceName == candidates[j].WorkspaceName {
				return candidates[i].Repo < candidates[j].Repo
			}
			return candidates[i].WorkspaceName < candidates[j].WorkspaceName
		}
		return candidates[i].Ticket < candidates[j].Ticket
	})
	if len(candidates) == 0 {
		if len(selectedTickets) == 1 {
			return PullRequestResult{}, fmt.Errorf("no prepared commit metadata found for ticket %s; run commit first", selectedTickets[0])
		}
		return PullRequestResult{}, fmt.Errorf("no prepared commit metadata found for selected tickets; run commit first")
	}

	repoResults := make([]PullRequestRepoResult, 0, len(candidates))
	now := time.Now()
	for _, row := range candidates {
		rowTicket := strings.TrimSpace(row.Ticket)
		if rowTicket == "" {
			return PullRequestResult{}, fmt.Errorf("run pull request row has empty ticket")
		}
		repo := strings.TrimSpace(row.Repo)
		if repo == "" {
			return PullRequestResult{}, fmt.Errorf("run pull request row has empty repo for ticket %s", rowTicket)
		}
		workspaceName := strings.TrimSpace(row.WorkspaceName)
		if workspaceName == "" {
			return PullRequestResult{}, fmt.Errorf("run pull request row for ticket %s repo %s is missing workspace_name", rowTicket, repo)
		}

		workspacePath, err := resolveWorkspacePath(workspaceName)
		if err != nil {
			return PullRequestResult{}, err
		}
		targets, err := resolveWorkspaceCommitRepoTargets(workspacePath, []string{repo})
		if err != nil {
			return PullRequestResult{}, err
		}
		repoPath := targets[0].RepoPath

		baseBranch := normalizeBaseBranch(row.BaseBranch)
		if baseBranch == "" {
			baseBranch = normalizeBaseBranch(spec.BaseBranch)
		}
		if baseBranch == "" {
			baseBranch = normalizeBaseBranch(cfg.Workspace.BaseBranch)
		}
		if baseBranch == "" {
			baseBranch = "main"
		}
		headBranch := strings.TrimSpace(row.HeadBranch)
		if headBranch == "" {
			return PullRequestResult{}, fmt.Errorf("run pull request row for ticket %s repo %s is missing head branch", rowTicket, repo)
		}

		title := strings.TrimSpace(options.Title)
		if title == "" {
			title = defaultPRTitle(rowTicket, summary, repo, len(candidates) > 1)
		}
		body := strings.TrimSpace(options.Body)
		if body == "" {
			body = defaultPRBody(runID, rowTicket, repo, headBranch, baseBranch, row.CommitSHA, brief)
		}

		args := []string{
			"pr", "create",
			"--base", baseBranch,
			"--head", headBranch,
			"--title", title,
			"--body", body,
		}
		for _, label := range normalizeTokens(cfg.GitPR.DefaultLabels) {
			args = append(args, "--label", label)
		}
		for _, reviewer := range normalizeTokens(cfg.GitPR.DefaultReviewers) {
			args = append(args, "--reviewer", reviewer)
		}
		pushPreview := commandPreview("git", "-C", repoPath, "push", "--set-upstream", "origin", headBranch)
		preview := fmt.Sprintf("cd %s && %s", shellQuote(repoPath), commandPreview("gh", args...))
		resolvedActor, actorSource := resolveOperationActor(ctx, options.Actor, repoPath)
		preflight := collectPullRequestPreflight(ctx, repoPath, headBranch, baseBranch)

		repoResult := PullRequestRepoResult{
			Ticket:        rowTicket,
			WorkspaceName: workspaceName,
			Repo:          repo,
			RepoPath:      repoPath,
			HeadBranch:    headBranch,
			BaseBranch:    baseBranch,
			Title:         title,
			Body:          body,
			Actor:         resolvedActor,
			ActorSource:   actorSource,
			Preflight:     preflight,
			Actions:       []string{pushPreview, preview},
		}
		if strings.TrimSpace(row.PRURL) != "" {
			repoResult.SkippedReason = "pull request already exists: " + strings.TrimSpace(row.PRURL)
			repoResult.PRURL = strings.TrimSpace(row.PRURL)
			repoResult.PRNumber = row.PRNumber
			repoResult.PRState = row.PRState
			if !options.DryRun {
				addressedAt := time.Now()
				if err := s.transitionReviewFeedbackStatus(runID, rowTicket, repo, model.ReviewFeedbackStatusNew, model.ReviewFeedbackStatusAddressed, &addressedAt); err != nil {
					return PullRequestResult{}, err
				}
			}
			repoResults = append(repoResults, repoResult)
			continue
		}
		validationReport, err := runGitPRValidations(ctx, cfg, gitPRValidationInput{
			Operation:     gitPRValidationOperationPR,
			RunID:         runID,
			Ticket:        rowTicket,
			WorkspaceName: workspaceName,
			Repo:          repo,
			RepoPath:      repoPath,
			BaseBranch:    baseBranch,
			HeadBranch:    headBranch,
		})
		if err != nil {
			return PullRequestResult{}, fmt.Errorf("pull request validation failed for ticket=%s workspace=%s repo=%s: %w", rowTicket, workspaceName, repo, err)
		}
		validationJSON := marshalGitPRValidationReport(validationReport)
		if options.DryRun {
			repoResults = append(repoResults, repoResult)
			continue
		}

		if _, err := runGitCommand(ctx, repoPath, "push", "--set-upstream", "origin", headBranch); err != nil {
			return PullRequestResult{}, err
		}
		output, err := runCommandInDir(ctx, repoPath, "gh", args...)
		if err != nil {
			return PullRequestResult{}, err
		}
		prURL, prNumber, err := parsePRCreateOutput(output)
		if err != nil {
			return PullRequestResult{}, fmt.Errorf("parse gh pr create output for %s/%s: %w", rowTicket, repo, err)
		}

		row.BaseBranch = baseBranch
		row.HeadBranch = headBranch
		row.PRURL = prURL
		row.PRNumber = prNumber
		row.PRState = model.PullRequestStateOpen
		row.CredentialMode = credentialMode
		row.Actor = resolvedActor
		row.ValidationJSON = validationJSON
		row.ErrorText = ""
		if row.CreatedAt.IsZero() {
			row.CreatedAt = now
		}
		row.UpdatedAt = now
		if err := s.store.UpsertRunPullRequest(row); err != nil {
			return PullRequestResult{}, err
		}

		message := fmt.Sprintf("ticket=%s workspace=%s repo=%s pr=%s credential_mode=%s actor=%s actor_source=%s",
			rowTicket, workspaceName, repo, prURL, credentialMode, resolvedActor, actorSource)
		_ = s.store.AddEvent(runID, "repo", repo, "pr_created", "", strconv.Itoa(prNumber), message)
		addressedAt := time.Now()
		if err := s.transitionReviewFeedbackStatus(runID, rowTicket, repo, model.ReviewFeedbackStatusNew, model.ReviewFeedbackStatusAddressed, &addressedAt); err != nil {
			return PullRequestResult{}, err
		}

		repoResult.PRURL = prURL
		repoResult.PRNumber = prNumber
		repoResult.PRState = model.PullRequestStateOpen
		repoResults = append(repoResults, repoResult)
	}

	return PullRequestResult{
		RunID: runID,
		Repos: repoResults,
	}, nil
}

func (s *Service) SyncReviewFeedback(ctx context.Context, options ReviewFeedbackSyncOptions) (ReviewFeedbackSyncResult, error) {
	runID, err := s.resolveRunID(options.RunID, options.Ticket)
	if err != nil {
		return ReviewFeedbackSyncResult{}, err
	}

	record, specJSON, policyJSON, err := s.store.GetRun(runID)
	if err != nil {
		return ReviewFeedbackSyncResult{}, err
	}
	if record.Status != model.RunStatusComplete {
		return ReviewFeedbackSyncResult{}, fmt.Errorf("run %s must be completed before syncing review feedback (current: %s)", runID, record.Status)
	}

	var spec model.RunSpec
	if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
		return ReviewFeedbackSyncResult{}, fmt.Errorf("unmarshal run spec: %w", err)
	}
	cfg := policy.Default()
	if strings.TrimSpace(policyJSON) != "" {
		if err := json.Unmarshal([]byte(policyJSON), &cfg); err != nil {
			return ReviewFeedbackSyncResult{}, fmt.Errorf("unmarshal run policy: %w", err)
		}
	}
	if !cfg.GitPR.ReviewFeedback.Enabled {
		return ReviewFeedbackSyncResult{}, fmt.Errorf("git_pr.review_feedback.enabled is false; review feedback sync disabled")
	}
	if !cfg.GitPR.ReviewFeedback.IncludeReviewComments {
		return ReviewFeedbackSyncResult{}, fmt.Errorf("git_pr.review_feedback.include_review_comments must be true for V1")
	}

	tickets := normalizeTokens(spec.Tickets)
	if len(tickets) == 0 {
		return ReviewFeedbackSyncResult{}, fmt.Errorf("run %s has no tickets", runID)
	}
	selectedTickets := append([]string(nil), tickets...)
	if selectedTicket := strings.TrimSpace(options.Ticket); selectedTicket != "" {
		if !containsToken(tickets, selectedTicket) {
			return ReviewFeedbackSyncResult{}, fmt.Errorf("ticket %q is not part of run %s", selectedTicket, runID)
		}
		selectedTickets = []string{selectedTicket}
	}

	allowedRepos := normalizeTokens(cfg.GitPR.AllowedRepos)
	rows, err := s.store.ListRunPullRequests(runID)
	if err != nil {
		return ReviewFeedbackSyncResult{}, err
	}
	candidates := make([]model.RunPullRequest, 0, len(rows))
	for _, row := range rows {
		if !containsToken(selectedTickets, row.Ticket) {
			continue
		}
		if len(allowedRepos) > 0 && !containsToken(allowedRepos, row.Repo) {
			continue
		}
		if row.PRState != model.PullRequestStateOpen {
			continue
		}
		if strings.TrimSpace(row.PRURL) == "" || row.PRNumber <= 0 {
			continue
		}
		candidates = append(candidates, row)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Ticket == candidates[j].Ticket {
			if candidates[i].Repo == candidates[j].Repo {
				return candidates[i].PRNumber < candidates[j].PRNumber
			}
			return candidates[i].Repo < candidates[j].Repo
		}
		return candidates[i].Ticket < candidates[j].Ticket
	})
	if len(candidates) == 0 {
		return ReviewFeedbackSyncResult{}, fmt.Errorf("no open pull requests with metadata found for selected run/tickets")
	}

	maxItems := cfg.GitPR.ReviewFeedback.MaxItemsPerSync
	if options.MaxItems > 0 && options.MaxItems < maxItems {
		maxItems = options.MaxItems
	}
	if maxItems <= 0 {
		return ReviewFeedbackSyncResult{}, fmt.Errorf("review feedback max items must be > 0")
	}
	remaining := maxItems
	ignoredAuthors := normalizeIgnoreAuthorSet(cfg.GitPR.ReviewFeedback.IgnoreAuthors)

	existingRows, err := s.store.ListRunReviewFeedback(runID)
	if err != nil {
		return ReviewFeedbackSyncResult{}, err
	}
	existingByKey := map[string]model.RunReviewFeedback{}
	for _, row := range existingRows {
		key := runReviewFeedbackKey(row.Ticket, row.Repo, row.PRNumber, row.SourceType, row.SourceID)
		existingByKey[key] = row
	}

	now := time.Now()
	result := ReviewFeedbackSyncResult{
		RunID: runID,
		Repos: make([]ReviewFeedbackSyncRepoResult, 0, len(candidates)),
	}

	for _, row := range candidates {
		repoResult := ReviewFeedbackSyncRepoResult{
			Ticket:   strings.TrimSpace(row.Ticket),
			Repo:     strings.TrimSpace(row.Repo),
			PRNumber: row.PRNumber,
			PRURL:    strings.TrimSpace(row.PRURL),
		}
		if remaining <= 0 {
			repoResult.SkippedReason = "max_items_per_sync limit reached"
			result.Repos = append(result.Repos, repoResult)
			continue
		}

		ownerRepo, prNumber, err := parseGitHubPullURL(row.PRURL)
		if err != nil {
			repoResult.SkippedReason = err.Error()
			result.Repos = append(result.Repos, repoResult)
			continue
		}
		commentsEndpoint := fmt.Sprintf("repos/%s/pulls/%d/comments", ownerRepo, prNumber)
		reviewsEndpoint := fmt.Sprintf("repos/%s/pulls/%d/reviews", ownerRepo, prNumber)
		repoResult.Actions = []string{
			commandPreview("gh", "api", commentsEndpoint, "--paginate"),
			commandPreview("gh", "api", reviewsEndpoint, "--paginate"),
		}

		output, err := runCommandInDir(ctx, "", "gh", "api", commentsEndpoint, "--paginate")
		if err != nil {
			return ReviewFeedbackSyncResult{}, err
		}
		comments, err := parsePRReviewComments(output)
		if err != nil {
			return ReviewFeedbackSyncResult{}, fmt.Errorf("parse review comments for %s: %w", row.PRURL, err)
		}
		output, err = runCommandInDir(ctx, "", "gh", "api", reviewsEndpoint, "--paginate")
		if err != nil {
			return ReviewFeedbackSyncResult{}, err
		}
		reviews, err := parsePRTopLevelReviews(output)
		if err != nil {
			return ReviewFeedbackSyncResult{}, fmt.Errorf("parse pull request reviews for %s: %w", row.PRURL, err)
		}
		repoResult.Fetched = len(comments) + len(reviews)

		for _, comment := range comments {
			if reviewFeedbackAuthorIgnored(ignoredAuthors, comment.User.Login) {
				continue
			}
			if remaining <= 0 {
				break
			}
			sourceID := strconv.FormatInt(comment.ID, 10)
			key := runReviewFeedbackKey(row.Ticket, row.Repo, row.PRNumber, model.ReviewFeedbackSourceTypePRReviewComment, sourceID)
			existing, exists := existingByKey[key]

			status := model.ReviewFeedbackStatusQueued
			if exists && strings.TrimSpace(string(existing.Status)) != "" {
				status = existing.Status
			}
			createdAt := now
			if exists && !existing.CreatedAt.IsZero() {
				createdAt = existing.CreatedAt
			}
			record := model.RunReviewFeedback{
				RunID:         runID,
				Ticket:        row.Ticket,
				Repo:          row.Repo,
				WorkspaceName: row.WorkspaceName,
				PRNumber:      row.PRNumber,
				PRURL:         row.PRURL,
				SourceType:    model.ReviewFeedbackSourceTypePRReviewComment,
				SourceID:      sourceID,
				SourceURL:     comment.HTMLURL,
				Author:        strings.TrimSpace(comment.User.Login),
				Body:          strings.TrimSpace(comment.Body),
				FilePath:      strings.TrimSpace(comment.Path),
				Line:          comment.EffectiveLine(),
				Status:        status,
				ErrorText:     "",
				CreatedAt:     createdAt,
				UpdatedAt:     now,
				LastSeenAt:    now,
				AddressedAt:   existing.AddressedAt,
			}
			if !options.DryRun {
				if err := s.store.UpsertRunReviewFeedback(record); err != nil {
					return ReviewFeedbackSyncResult{}, err
				}
			}
			if exists {
				if runReviewFeedbackChanged(existing, record) {
					repoResult.Updated++
					result.Updated++
				}
			} else {
				repoResult.Added++
				result.Added++
			}
			existingByKey[key] = record
			remaining--
			if remaining <= 0 {
				break
			}
		}
		for _, review := range reviews {
			if reviewFeedbackAuthorIgnored(ignoredAuthors, review.User.Login) {
				continue
			}
			if remaining <= 0 {
				break
			}
			sourceID := strconv.FormatInt(review.ID, 10)
			key := runReviewFeedbackKey(row.Ticket, row.Repo, row.PRNumber, model.ReviewFeedbackSourceTypePRReview, sourceID)
			existing, exists := existingByKey[key]

			status := model.ReviewFeedbackStatusQueued
			if exists && strings.TrimSpace(string(existing.Status)) != "" {
				status = existing.Status
			}
			createdAt := review.SubmittedAt
			if createdAt.IsZero() {
				createdAt = now
			}
			if exists && !existing.CreatedAt.IsZero() {
				createdAt = existing.CreatedAt
			}
			record := model.RunReviewFeedback{
				RunID:         runID,
				Ticket:        row.Ticket,
				Repo:          row.Repo,
				WorkspaceName: row.WorkspaceName,
				PRNumber:      row.PRNumber,
				PRURL:         row.PRURL,
				SourceType:    model.ReviewFeedbackSourceTypePRReview,
				SourceID:      sourceID,
				SourceURL:     strings.TrimSpace(review.HTMLURL),
				Author:        strings.TrimSpace(review.User.Login),
				Body:          formatTopLevelReviewBody(review),
				FilePath:      "",
				Line:          0,
				Status:        status,
				ErrorText:     "",
				CreatedAt:     createdAt,
				UpdatedAt:     now,
				LastSeenAt:    now,
				AddressedAt:   existing.AddressedAt,
			}
			if !options.DryRun {
				if err := s.store.UpsertRunReviewFeedback(record); err != nil {
					return ReviewFeedbackSyncResult{}, err
				}
			}
			if exists {
				if runReviewFeedbackChanged(existing, record) {
					repoResult.Updated++
					result.Updated++
				}
			} else {
				repoResult.Added++
				result.Added++
			}
			existingByKey[key] = record
			remaining--
			if remaining <= 0 {
				break
			}
		}

		if !options.DryRun {
			message := fmt.Sprintf("ticket=%s repo=%s pr=%d fetched=%d added=%d updated=%d",
				repoResult.Ticket, repoResult.Repo, repoResult.PRNumber, repoResult.Fetched, repoResult.Added, repoResult.Updated)
			_ = s.store.AddEvent(runID, "repo", repoResult.Repo, "review_feedback_synced", "", "", message)
		}
		result.Repos = append(result.Repos, repoResult)
	}

	return result, nil
}

func (s *Service) DispatchQueuedReviewFeedback(ctx context.Context, options ReviewFeedbackDispatchOptions) (ReviewFeedbackDispatchResult, error) {
	runID, err := s.resolveRunID(options.RunID, options.Ticket)
	if err != nil {
		return ReviewFeedbackDispatchResult{}, err
	}
	queuedRows, err := s.store.ListRunReviewFeedbackByStatus(runID, model.ReviewFeedbackStatusQueued)
	if err != nil {
		return ReviewFeedbackDispatchResult{}, err
	}
	filtered := make([]model.RunReviewFeedback, 0, len(queuedRows))
	selectedTicket := strings.TrimSpace(options.Ticket)
	for _, row := range queuedRows {
		if selectedTicket != "" && !strings.EqualFold(strings.TrimSpace(row.Ticket), selectedTicket) {
			continue
		}
		filtered = append(filtered, row)
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].Ticket == filtered[j].Ticket {
			if filtered[i].Repo == filtered[j].Repo {
				if filtered[i].PRNumber == filtered[j].PRNumber {
					return filtered[i].SourceID < filtered[j].SourceID
				}
				return filtered[i].PRNumber < filtered[j].PRNumber
			}
			return filtered[i].Repo < filtered[j].Repo
		}
		return filtered[i].Ticket < filtered[j].Ticket
	})
	if len(filtered) == 0 {
		if selectedTicket != "" {
			return ReviewFeedbackDispatchResult{}, fmt.Errorf("no queued review feedback found for ticket %s", selectedTicket)
		}
		return ReviewFeedbackDispatchResult{}, fmt.Errorf("no queued review feedback found for run %s", runID)
	}
	maxItems := len(filtered)
	if options.MaxItems > 0 && options.MaxItems < maxItems {
		maxItems = options.MaxItems
	}
	feedback := renderQueuedReviewFeedback(filtered[:maxItems])
	iterateResult, err := s.Iterate(ctx, IterateOptions{
		RunID:    runID,
		Ticket:   selectedTicket,
		Feedback: feedback,
		DryRun:   options.DryRun,
	})
	if err != nil {
		return ReviewFeedbackDispatchResult{}, err
	}
	return ReviewFeedbackDispatchResult{
		RunID:       iterateResult.RunID,
		QueuedCount: maxItems,
		Feedback:    feedback,
		Actions:     iterateResult.Actions,
	}, nil
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

func (s *Service) UpsertRunReviewFeedback(record model.RunReviewFeedback) error {
	return s.store.UpsertRunReviewFeedback(record)
}

func (s *Service) ListRunReviewFeedback(runID string) ([]model.RunReviewFeedback, error) {
	return s.store.ListRunReviewFeedback(runID)
}

func (s *Service) ListRunReviewFeedbackByStatus(runID string, status model.ReviewFeedbackStatus) ([]model.RunReviewFeedback, error) {
	return s.store.ListRunReviewFeedbackByStatus(runID, status)
}

func (s *Service) resolveWorkspaceTickets(runID string) (map[string]string, error) {
	steps, err := s.store.GetSteps(runID)
	if err != nil {
		return nil, err
	}
	workspaceTickets := map[string]string{}
	for _, step := range steps {
		workspaceName := strings.TrimSpace(step.WorkspaceName)
		ticket := strings.TrimSpace(step.Ticket)
		if workspaceName == "" || ticket == "" {
			continue
		}
		if _, exists := workspaceTickets[workspaceName]; exists {
			continue
		}
		workspaceTickets[workspaceName] = ticket
	}
	return workspaceTickets, nil
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
	forumThreads, _ := s.store.ListForumThreads(model.ForumThreadFilter{RunID: runID, Limit: 200})
	brief, _ := s.store.GetRunBrief(runID)
	docSyncStates, _ := s.store.ListDocSyncStates(runID)
	runPullRequests, _ := s.store.ListRunPullRequests(runID)
	runReviewFeedback, _ := s.store.ListRunReviewFeedback(runID)

	slaMinutes := cfg.Forum.SLA.EscalationMinutes
	if slaMinutes <= 0 {
		slaMinutes = 30
	}
	slaThreshold := time.Duration(slaMinutes) * time.Minute
	forumEscalations := []model.ForumThreadView{}
	for _, thread := range forumThreads {
		if thread.State != model.ForumThreadStateNew && thread.State != model.ForumThreadStateWaitingHuman {
			continue
		}
		age := now.Sub(thread.UpdatedAt)
		if thread.Priority == model.ForumPriorityUrgent || thread.Priority == model.ForumPriorityHigh || age >= slaThreshold {
			forumEscalations = append(forumEscalations, thread)
		}
	}

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
	if len(pendingGuidance) > 0 || len(forumEscalations) > 0 {
		b.WriteString("Guidance:\n")
		for _, item := range pendingGuidance {
			b.WriteString(fmt.Sprintf("  - id=%d %s@%s question=%s\n", item.ID, item.AgentName, item.WorkspaceName, item.Question))
		}
		for _, thread := range forumEscalations {
			b.WriteString(fmt.Sprintf("  - forum thread=%s state=%s priority=%s title=%s\n", thread.ThreadID, thread.State, thread.Priority, thread.Title))
		}
	}
	if len(forumThreads) > 0 {
		total := len(forumThreads)
		newCount := 0
		waitingOperatorCount := 0
		waitingHumanCount := 0
		answeredCount := 0
		closedCount := 0
		for _, thread := range forumThreads {
			switch thread.State {
			case model.ForumThreadStateNew:
				newCount++
			case model.ForumThreadStateWaitingOperator:
				waitingOperatorCount++
			case model.ForumThreadStateWaitingHuman:
				waitingHumanCount++
			case model.ForumThreadStateAnswered:
				answeredCount++
			case model.ForumThreadStateClosed:
				closedCount++
			}
		}
		b.WriteString("Forum:\n")
		b.WriteString(fmt.Sprintf("  - total=%d new=%d waiting_operator=%d waiting_human=%d answered=%d closed=%d escalations=%d\n",
			total,
			newCount,
			waitingOperatorCount,
			waitingHumanCount,
			answeredCount,
			closedCount,
			len(forumEscalations),
		))
		limit := len(forumThreads)
		if limit > 5 {
			limit = 5
		}
		for i := 0; i < limit; i++ {
			item := forumThreads[i]
			b.WriteString(fmt.Sprintf("  - thread=%s state=%s priority=%s assignee=%s/%s posts=%d updated_at=%s title=%s\n",
				item.ThreadID,
				item.State,
				item.Priority,
				valueOrDefault(string(item.AssigneeType), "-"),
				valueOrDefault(item.AssigneeName, "-"),
				item.PostsCount,
				item.UpdatedAt.Format(time.RFC3339),
				item.Title,
			))
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
	if len(runReviewFeedback) > 0 {
		queued := 0
		newCount := 0
		addressed := 0
		ignored := 0
		for _, item := range runReviewFeedback {
			switch item.Status {
			case model.ReviewFeedbackStatusQueued:
				queued++
			case model.ReviewFeedbackStatusNew:
				newCount++
			case model.ReviewFeedbackStatusAddressed:
				addressed++
			case model.ReviewFeedbackStatusIgnored:
				ignored++
			}
		}
		b.WriteString("Review Feedback:\n")
		b.WriteString(fmt.Sprintf("  - status=queued count=%d\n", queued))
		b.WriteString(fmt.Sprintf("  - status=new count=%d\n", newCount))
		b.WriteString(fmt.Sprintf("  - status=addressed count=%d\n", addressed))
		b.WriteString(fmt.Sprintf("  - status=ignored count=%d\n", ignored))
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
		if tmuxSessionProbe(ctx, sessionName) == tmuxSessionMissing {
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

type workspaceCommitRepoTarget struct {
	Repo     string
	RepoPath string
}

func resolveWorkspaceCommitRepoTargets(workspacePath string, repos []string) ([]workspaceCommitRepoTarget, error) {
	repos = normalizeTokens(repos)
	if len(repos) == 0 {
		if isGitRepo(workspacePath) {
			return []workspaceCommitRepoTarget{{
				Repo:     filepath.Base(workspacePath),
				RepoPath: workspacePath,
			}}, nil
		}
		return nil, fmt.Errorf("workspace %s has no repository definitions", workspacePath)
	}

	targets := make([]workspaceCommitRepoTarget, 0, len(repos))
	for _, repo := range repos {
		repo = strings.TrimSpace(repo)
		if repo == "" {
			continue
		}
		candidate := filepath.Join(workspacePath, repo)
		if isGitRepo(candidate) {
			targets = append(targets, workspaceCommitRepoTarget{
				Repo:     repo,
				RepoPath: candidate,
			})
			continue
		}
		if len(repos) == 1 && isGitRepo(workspacePath) {
			targets = append(targets, workspaceCommitRepoTarget{
				Repo:     repo,
				RepoPath: workspacePath,
			})
			continue
		}
		return nil, fmt.Errorf("workspace repo path not found: %s", candidate)
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("no git repositories found in workspace %s", workspacePath)
	}
	return targets, nil
}

func filterReposByAllowedList(repos []string, allowed []string) []string {
	repos = normalizeTokens(repos)
	allowed = normalizeTokens(allowed)
	if len(allowed) == 0 {
		return repos
	}
	filtered := []string{}
	for _, repo := range repos {
		if containsToken(allowed, repo) {
			filtered = append(filtered, repo)
		}
	}
	return filtered
}

func resolveCommitBaseRef(ctx context.Context, repoPath string, baseBranch string) (string, error) {
	baseBranch = normalizeBaseBranch(baseBranch)
	if baseBranch == "" {
		baseBranch = "main"
	}
	_, _ = runGitCommand(ctx, repoPath, "fetch", "origin", baseBranch)

	remoteRef := "refs/remotes/origin/" + baseBranch
	localRef := "refs/heads/" + baseBranch
	switch {
	case gitRefExists(ctx, repoPath, remoteRef):
		return "origin/" + baseBranch, nil
	case gitRefExists(ctx, repoPath, localRef):
		return baseBranch, nil
	default:
		return "", fmt.Errorf("base branch %q not found for repo %s", baseBranch, repoPath)
	}
}

func collectCommitPreflight(ctx context.Context, repoPath string, baseRef string) ([]string, error) {
	currentBranch, err := runGitCommand(ctx, repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return nil, err
	}
	currentHead, err := runGitCommand(ctx, repoPath, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	baseHead, err := runGitCommand(ctx, repoPath, "rev-parse", baseRef)
	if err != nil {
		return nil, err
	}
	drift := strings.TrimSpace(currentHead) != strings.TrimSpace(baseHead)
	return []string{
		fmt.Sprintf("current_branch=%s", strings.TrimSpace(currentBranch)),
		fmt.Sprintf("current_head=%s", strings.TrimSpace(currentHead)),
		fmt.Sprintf("base_ref=%s", strings.TrimSpace(baseRef)),
		fmt.Sprintf("base_head=%s", strings.TrimSpace(baseHead)),
		fmt.Sprintf("base_drift=%t", drift),
	}, nil
}

func collectPullRequestPreflight(ctx context.Context, repoPath string, headBranch string, baseBranch string) []string {
	headRef := strings.TrimSpace(headBranch)
	if headRef == "" {
		headRef = "HEAD"
	}
	lines := []string{
		fmt.Sprintf("head_branch=%s", strings.TrimSpace(headBranch)),
	}
	headSHA, err := runGitCommand(ctx, repoPath, "rev-parse", headRef)
	if err != nil {
		lines = append(lines, fmt.Sprintf("head_resolve_error=%s", compactErrorText(err)))
		return lines
	}
	lines = append(lines, fmt.Sprintf("head_sha=%s", strings.TrimSpace(headSHA)))
	baseRef, err := resolveCommitBaseRef(ctx, repoPath, baseBranch)
	if err != nil {
		lines = append(lines, fmt.Sprintf("base_resolve_error=%s", compactErrorText(err)))
		return lines
	}
	lines = append(lines, fmt.Sprintf("base_ref=%s", strings.TrimSpace(baseRef)))
	baseSHA, err := runGitCommand(ctx, repoPath, "rev-parse", baseRef)
	if err != nil {
		lines = append(lines, fmt.Sprintf("base_sha_error=%s", compactErrorText(err)))
		return lines
	}
	drift := strings.TrimSpace(headSHA) != strings.TrimSpace(baseSHA)
	lines = append(lines,
		fmt.Sprintf("base_sha=%s", strings.TrimSpace(baseSHA)),
		fmt.Sprintf("base_drift=%t", drift),
	)
	return lines
}

func commitActionsPreview(repoPath string, branchName string, baseRef string, commitMessage string, dirty bool) []string {
	actions := []string{}
	if dirty {
		actions = append(actions,
			fmt.Sprintf("git -C %s stash push -u -m %s", shellQuote(repoPath), shellQuote("metawsm-commit-snapshot")),
		)
	}
	actions = append(actions,
		fmt.Sprintf("git -C %s checkout -B %s %s", shellQuote(repoPath), shellQuote(branchName), shellQuote(baseRef)),
	)
	if dirty {
		actions = append(actions,
			fmt.Sprintf("git -C %s stash apply --index %s", shellQuote(repoPath), shellQuote("stash@{0}")),
			fmt.Sprintf("git -C %s stash drop %s", shellQuote(repoPath), shellQuote("stash@{0}")),
		)
	}
	actions = append(actions,
		fmt.Sprintf("git -C %s add -A", shellQuote(repoPath)),
		fmt.Sprintf("git -C %s commit -m %s", shellQuote(repoPath), shellQuote(commitMessage)),
	)
	return actions
}

func prepareCommitBranch(ctx context.Context, repoPath string, branchName string, baseRef string, dirty bool) error {
	if !dirty {
		_, err := runGitCommand(ctx, repoPath, "checkout", "-B", branchName, baseRef)
		return err
	}

	snapshotName := fmt.Sprintf("metawsm-commit-snapshot-%d", time.Now().UnixNano())
	snapshotRef, err := captureWorkspaceSnapshot(ctx, repoPath, snapshotName)
	if err != nil {
		return err
	}
	if _, err := runGitCommand(ctx, repoPath, "checkout", "-B", branchName, baseRef); err != nil {
		return err
	}
	if strings.TrimSpace(snapshotRef) == "" {
		return nil
	}
	if _, err := runGitCommand(ctx, repoPath, "stash", "apply", "--index", snapshotRef); err != nil {
		return fmt.Errorf("failed to reapply workspace snapshot after checkout to %s in %s; resolve conflicts and continue manually (snapshot retained at %s): %w",
			baseRef, repoPath, snapshotRef, err)
	}
	if _, err := runGitCommand(ctx, repoPath, "stash", "drop", snapshotRef); err != nil {
		return fmt.Errorf("failed to drop temporary workspace snapshot %s in %s: %w", snapshotRef, repoPath, err)
	}
	return nil
}

func captureWorkspaceSnapshot(ctx context.Context, repoPath string, snapshotName string) (string, error) {
	out, err := runGitCommand(ctx, repoPath, "stash", "push", "-u", "-m", snapshotName)
	if err != nil {
		return "", err
	}
	if strings.Contains(out, "No local changes to save") {
		return "", nil
	}
	top, err := runGitCommand(ctx, repoPath, "stash", "list", "-n", "1", "--format=%gd|%gs")
	if err != nil {
		return "", err
	}
	parts := strings.SplitN(strings.TrimSpace(top), "|", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("unable to identify snapshot ref after stash push in %s: %s", repoPath, top)
	}
	ref := strings.TrimSpace(parts[0])
	summary := strings.TrimSpace(parts[1])
	if ref == "" {
		return "", fmt.Errorf("stash snapshot ref is empty after stash push in %s", repoPath)
	}
	if !strings.Contains(summary, snapshotName) {
		return "", fmt.Errorf("unexpected stash entry after snapshot in %s: %s", repoPath, summary)
	}
	return ref, nil
}

func resolveOperationActor(ctx context.Context, explicit string, repoPath string) (string, string) {
	explicit = strings.TrimSpace(explicit)
	if explicit != "" {
		return explicit, "flag"
	}
	if actor, ok := resolveGitHubActor(ctx); ok {
		return actor, "gh"
	}
	if identity, ok := resolveGitIdentity(ctx, repoPath); ok {
		return identity, "git"
	}
	return "unknown", "none"
}

func resolveGitHubActor(ctx context.Context) (string, bool) {
	if _, err := exec.LookPath("gh"); err != nil {
		return "", false
	}
	out, err := runCommandInDir(ctx, "", "gh", "api", "user", "--jq", ".login")
	if err != nil {
		return "", false
	}
	actor := strings.TrimSpace(out)
	if actor == "" {
		return "", false
	}
	return actor, true
}

func resolveGitIdentity(ctx context.Context, repoPath string) (string, bool) {
	name, err := runGitCommand(ctx, repoPath, "config", "--get", "user.name")
	if err != nil {
		return "", false
	}
	email, err := runGitCommand(ctx, repoPath, "config", "--get", "user.email")
	if err != nil {
		if strings.TrimSpace(name) == "" {
			return "", false
		}
		return strings.TrimSpace(name), true
	}
	name = strings.TrimSpace(name)
	email = strings.TrimSpace(email)
	switch {
	case name != "" && email != "":
		return fmt.Sprintf("%s <%s>", name, email), true
	case name != "":
		return name, true
	case email != "":
		return email, true
	default:
		return "", false
	}
}

func compactErrorText(err error) string {
	if err == nil {
		return ""
	}
	text := strings.TrimSpace(err.Error())
	text = strings.ReplaceAll(text, "\n", " | ")
	return text
}

func runGitCommand(ctx context.Context, repoPath string, args ...string) (string, error) {
	cmdArgs := append([]string{"-C", repoPath}, args...)
	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return "", fmt.Errorf("git %s failed in %s: %s", strings.Join(args, " "), repoPath, text)
	}
	return text, nil
}

func (s *Service) defaultCommitMessage(runID string, ticket string) string {
	summary := fmt.Sprintf("apply run %s updates", runID)
	brief, err := s.store.GetRunBrief(runID)
	if err == nil && brief != nil {
		goal := firstNonEmptyLine(brief.Goal)
		if goal != "" {
			summary = goal
		}
	}
	summary = strings.TrimSpace(summary)
	if len(summary) > 72 {
		summary = strings.TrimSpace(summary[:72])
	}
	ticket = strings.TrimSpace(ticket)
	if ticket == "" {
		return summary
	}
	return fmt.Sprintf("%s: %s", ticket, summary)
}

func firstNonEmptyLine(value string) string {
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func defaultPRSummary(runID string, brief *model.RunBrief) string {
	summary := fmt.Sprintf("Automated updates from %s", runID)
	if brief != nil {
		goal := firstNonEmptyLine(brief.Goal)
		if goal != "" {
			summary = goal
		}
	}
	summary = strings.TrimSpace(summary)
	if len(summary) > 72 {
		summary = strings.TrimSpace(summary[:72])
	}
	return summary
}

func defaultPRTitle(ticket string, summary string, repo string, includeRepo bool) string {
	title := fmt.Sprintf("[%s] %s", strings.TrimSpace(ticket), strings.TrimSpace(summary))
	if includeRepo {
		title = fmt.Sprintf("%s (%s)", title, strings.TrimSpace(repo))
	}
	return strings.TrimSpace(title)
}

func defaultPRBody(runID string, ticket string, repo string, headBranch string, baseBranch string, commitSHA string, brief *model.RunBrief) string {
	var body strings.Builder
	body.WriteString("Automated pull request generated by metawsm.\n\n")
	body.WriteString(fmt.Sprintf("- Ticket: %s\n", strings.TrimSpace(ticket)))
	body.WriteString(fmt.Sprintf("- Run: %s\n", strings.TrimSpace(runID)))
	body.WriteString(fmt.Sprintf("- Repo: %s\n", strings.TrimSpace(repo)))
	body.WriteString(fmt.Sprintf("- Head branch: %s\n", strings.TrimSpace(headBranch)))
	body.WriteString(fmt.Sprintf("- Base branch: %s\n", strings.TrimSpace(baseBranch)))
	if strings.TrimSpace(commitSHA) != "" {
		body.WriteString(fmt.Sprintf("- Commit: %s\n", strings.TrimSpace(commitSHA)))
	}
	if brief != nil {
		goal := firstNonEmptyLine(brief.Goal)
		if goal != "" {
			body.WriteString("\n## Goal\n\n")
			body.WriteString(goal)
			body.WriteString("\n")
		}
		doneCriteria := firstNonEmptyLine(brief.DoneCriteria)
		if doneCriteria != "" {
			body.WriteString("\n## Done Criteria\n\n")
			body.WriteString(doneCriteria)
			body.WriteString("\n")
		}
	}
	return strings.TrimSpace(body.String())
}

func commandPreview(name string, args ...string) string {
	var preview strings.Builder
	preview.WriteString(name)
	for _, arg := range args {
		preview.WriteString(" ")
		preview.WriteString(shellQuote(arg))
	}
	return preview.String()
}

func runCommandInDir(ctx context.Context, dir string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return "", fmt.Errorf("%s %s failed in %s: %s", name, strings.Join(args, " "), dir, text)
	}
	return text, nil
}

func parsePRCreateOutput(output string) (string, int, error) {
	lines := strings.Fields(output)
	for i := len(lines) - 1; i >= 0; i-- {
		token := strings.Trim(lines[i], "\"'")
		if !strings.HasPrefix(token, "http://") && !strings.HasPrefix(token, "https://") {
			continue
		}
		if !strings.Contains(token, "/pull/") {
			continue
		}
		matches := pullURLNumberRegex.FindStringSubmatch(token)
		if len(matches) < 2 {
			return "", 0, fmt.Errorf("pull request URL missing numeric identifier: %s", token)
		}
		number, err := strconv.Atoi(matches[1])
		if err != nil {
			return "", 0, fmt.Errorf("parse pull request number from %s: %w", token, err)
		}
		return token, number, nil
	}
	return "", 0, fmt.Errorf("no pull request URL found in output")
}

type ghPRReviewComment struct {
	ID      int64  `json:"id"`
	HTMLURL string `json:"html_url"`
	Body    string `json:"body"`
	Path    string `json:"path"`
	Line    *int   `json:"line"`
	User    struct {
		Login string `json:"login"`
	} `json:"user"`
}

func (c ghPRReviewComment) EffectiveLine() int {
	if c.Line == nil {
		return 0
	}
	return *c.Line
}

func parsePRReviewComments(output string) ([]ghPRReviewComment, error) {
	text := strings.TrimSpace(output)
	if text == "" {
		return nil, nil
	}
	comments := []ghPRReviewComment{}
	if err := json.Unmarshal([]byte(text), &comments); err != nil {
		return nil, err
	}
	return comments, nil
}

type ghPRReview struct {
	ID          int64  `json:"id"`
	HTMLURL     string `json:"html_url"`
	Body        string `json:"body"`
	State       string `json:"state"`
	SubmittedAt string `json:"submitted_at"`
	User        struct {
		Login string `json:"login"`
	} `json:"user"`
}

type normalizedPRReview struct {
	ID          int64
	HTMLURL     string
	Body        string
	State       string
	SubmittedAt time.Time
	User        struct {
		Login string
	}
}

func parsePRTopLevelReviews(output string) ([]normalizedPRReview, error) {
	text := strings.TrimSpace(output)
	if text == "" {
		return nil, nil
	}
	reviews := []ghPRReview{}
	if err := json.Unmarshal([]byte(text), &reviews); err != nil {
		return nil, err
	}
	out := make([]normalizedPRReview, 0, len(reviews))
	for _, review := range reviews {
		state := strings.ToUpper(strings.TrimSpace(review.State))
		body := strings.TrimSpace(review.Body)
		if body == "" || state == "" || state == "PENDING" {
			continue
		}
		submittedAt := time.Time{}
		if ts := strings.TrimSpace(review.SubmittedAt); ts != "" {
			parsed, err := time.Parse(time.RFC3339, ts)
			if err != nil {
				return nil, fmt.Errorf("parse review submitted_at for review id %d: %w", review.ID, err)
			}
			submittedAt = parsed
		}
		normalized := normalizedPRReview{
			ID:          review.ID,
			HTMLURL:     strings.TrimSpace(review.HTMLURL),
			Body:        body,
			State:       state,
			SubmittedAt: submittedAt,
		}
		normalized.User.Login = strings.TrimSpace(review.User.Login)
		out = append(out, normalized)
	}
	return out, nil
}

func formatTopLevelReviewBody(review normalizedPRReview) string {
	body := strings.TrimSpace(review.Body)
	if body == "" {
		return body
	}
	metadata := []string{}
	if state := strings.TrimSpace(strings.ToLower(review.State)); state != "" {
		metadata = append(metadata, "state="+state)
	}
	if !review.SubmittedAt.IsZero() {
		metadata = append(metadata, "submitted_at="+review.SubmittedAt.Format(time.RFC3339))
	}
	if len(metadata) == 0 {
		return body
	}
	return fmt.Sprintf("[%s]\n%s", strings.Join(metadata, " "), body)
}

func normalizeIgnoreAuthorSet(authors []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, author := range authors {
		normalized := strings.ToLower(strings.TrimSpace(author))
		if normalized == "" {
			continue
		}
		out[normalized] = struct{}{}
	}
	return out
}

func reviewFeedbackAuthorIgnored(ignoreAuthors map[string]struct{}, author string) bool {
	if len(ignoreAuthors) == 0 {
		return false
	}
	normalized := strings.ToLower(strings.TrimSpace(author))
	if normalized == "" {
		return false
	}
	_, ignored := ignoreAuthors[normalized]
	return ignored
}

func parseGitHubPullURL(prURL string) (string, int, error) {
	matches := pullURLRepoNumberRegex.FindStringSubmatch(strings.TrimSpace(prURL))
	if len(matches) < 3 {
		return "", 0, fmt.Errorf("unsupported pull request URL: %s", strings.TrimSpace(prURL))
	}
	prNumber, err := strconv.Atoi(strings.TrimSpace(matches[2]))
	if err != nil {
		return "", 0, fmt.Errorf("parse pull request number from URL %s: %w", strings.TrimSpace(prURL), err)
	}
	return strings.TrimSpace(matches[1]), prNumber, nil
}

func runReviewFeedbackKey(ticket string, repo string, prNumber int, sourceType model.ReviewFeedbackSourceType, sourceID string) string {
	return strings.Join([]string{
		strings.TrimSpace(ticket),
		strings.TrimSpace(repo),
		strconv.Itoa(prNumber),
		string(sourceType),
		strings.TrimSpace(sourceID),
	}, "|")
}

func runReviewFeedbackChanged(existing model.RunReviewFeedback, next model.RunReviewFeedback) bool {
	if strings.TrimSpace(existing.PRURL) != strings.TrimSpace(next.PRURL) {
		return true
	}
	if strings.TrimSpace(existing.SourceURL) != strings.TrimSpace(next.SourceURL) {
		return true
	}
	if strings.TrimSpace(existing.Author) != strings.TrimSpace(next.Author) {
		return true
	}
	if strings.TrimSpace(existing.Body) != strings.TrimSpace(next.Body) {
		return true
	}
	if strings.TrimSpace(existing.FilePath) != strings.TrimSpace(next.FilePath) {
		return true
	}
	if existing.Line != next.Line {
		return true
	}
	if strings.TrimSpace(string(existing.Status)) != strings.TrimSpace(string(next.Status)) {
		return true
	}
	if strings.TrimSpace(existing.ErrorText) != strings.TrimSpace(next.ErrorText) {
		return true
	}
	if !existing.LastSeenAt.Equal(next.LastSeenAt) {
		return true
	}
	if !equalTimePtr(existing.AddressedAt, next.AddressedAt) {
		return true
	}
	return false
}

func equalTimePtr(a *time.Time, b *time.Time) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Equal(*b)
}

func renderQueuedReviewFeedback(rows []model.RunReviewFeedback) string {
	var b strings.Builder
	b.WriteString("GitHub PR review feedback to address:\n\n")
	for _, row := range rows {
		b.WriteString("- ")
		b.WriteString(fmt.Sprintf("[%s/%s PR #%d]", strings.TrimSpace(row.Ticket), strings.TrimSpace(row.Repo), row.PRNumber))
		if strings.TrimSpace(row.Author) != "" {
			b.WriteString(" @")
			b.WriteString(strings.TrimSpace(row.Author))
		}
		if strings.TrimSpace(row.FilePath) != "" {
			b.WriteString(" ")
			b.WriteString(strings.TrimSpace(row.FilePath))
			if row.Line > 0 {
				b.WriteString(fmt.Sprintf(":%d", row.Line))
			}
		}
		if strings.TrimSpace(row.SourceURL) != "" {
			b.WriteString(" ")
			b.WriteString(strings.TrimSpace(row.SourceURL))
		}
		b.WriteString("\n")
		body := strings.TrimSpace(row.Body)
		if body != "" {
			b.WriteString("  ")
			b.WriteString(strings.ReplaceAll(body, "\n", "\n  "))
			b.WriteString("\n")
		}
	}
	return strings.TrimSpace(b.String())
}

func (s *Service) transitionReviewFeedbackStatus(
	runID string,
	ticket string,
	repo string,
	fromStatus model.ReviewFeedbackStatus,
	toStatus model.ReviewFeedbackStatus,
	addressedAt *time.Time,
) error {
	rows, err := s.store.ListRunReviewFeedbackByStatus(runID, fromStatus)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if !strings.EqualFold(strings.TrimSpace(row.Ticket), strings.TrimSpace(ticket)) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(row.Repo), strings.TrimSpace(repo)) {
			continue
		}
		if err := s.store.UpdateRunReviewFeedbackStatus(
			runID,
			row.Ticket,
			row.Repo,
			row.PRNumber,
			row.SourceType,
			row.SourceID,
			toStatus,
			"",
			addressedAt,
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) acquireRunMutationLock(runID string, operation string) (func(), error) {
	lockPath := runMutationLockPath(s.store.DBPath, runID)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, fmt.Errorf("create mutation lock dir: %w", err)
	}
	payload := fmt.Sprintf("pid=%d operation=%s at=%s", os.Getpid(), strings.TrimSpace(operation), time.Now().Format(time.RFC3339))
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			holderBytes, _ := os.ReadFile(lockPath)
			return nil, &RunMutationInProgressError{
				RunID:     runID,
				Operation: operation,
				LockPath:  lockPath,
				Holder:    strings.TrimSpace(string(holderBytes)),
			}
		}
		return nil, fmt.Errorf("acquire mutation lock %s: %w", lockPath, err)
	}
	if _, err := lockFile.WriteString(payload + "\n"); err != nil {
		_ = lockFile.Close()
		_ = os.Remove(lockPath)
		return nil, fmt.Errorf("write mutation lock %s: %w", lockPath, err)
	}
	if err := lockFile.Close(); err != nil {
		_ = os.Remove(lockPath)
		return nil, fmt.Errorf("close mutation lock %s: %w", lockPath, err)
	}
	return func() {
		_ = os.Remove(lockPath)
	}, nil
}

func runMutationLockPath(dbPath string, runID string) string {
	lockDir := filepath.Join(filepath.Dir(strings.TrimSpace(dbPath)), "locks")
	return filepath.Join(lockDir, fmt.Sprintf("run-%s.gitpr.lock", sanitizeLockToken(runID)))
}

func sanitizeLockToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	var out strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			out.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			out.WriteRune(r)
		case r >= '0' && r <= '9':
			out.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			out.WriteRune(r)
		default:
			out.WriteRune('_')
		}
	}
	return out.String()
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
	sessionState := tmuxSessionProbe(ctx, agent.SessionName)
	if sessionState == tmuxSessionMissing {
		return model.HealthStateDead, model.AgentStatusDead, agent.LastActivityAt, agent.LastProgressAt
	}
	if sessionState == tmuxSessionUnknown {
		status := agent.Status
		health := agent.HealthState
		if status == model.AgentStatusDead || status == model.AgentStatusFailed || status == model.AgentStatusStopped || strings.TrimSpace(string(status)) == "" {
			status = model.AgentStatusIdle
		}
		if health == model.HealthStateDead || strings.TrimSpace(string(health)) == "" {
			health = model.HealthStateIdle
		}
		return health, status, agent.LastActivityAt, agent.LastProgressAt
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

type tmuxSessionState int

const (
	tmuxSessionUnknown tmuxSessionState = iota
	tmuxSessionPresent
	tmuxSessionMissing
)

func tmuxSessionProbe(ctx context.Context, sessionName string) tmuxSessionState {
	cmd := exec.CommandContext(ctx, "zsh", "-lc", fmt.Sprintf("tmux has-session -t %s", shellQuote(sessionName)))
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err == nil {
		return tmuxSessionPresent
	}
	if isTmuxSessionMissingMessage(stderr.String()) {
		return tmuxSessionMissing
	}
	if listState, ok := tmuxSessionProbeViaList(ctx, sessionName); ok {
		return listState
	}
	return tmuxSessionUnknown
}

func tmuxSessionProbeViaList(ctx context.Context, sessionName string) (tmuxSessionState, bool) {
	cmd := exec.CommandContext(ctx, "zsh", "-lc", "tmux ls -F '#S'")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if isTmuxSessionMissingMessage(stderr.String()) {
			return tmuxSessionMissing, true
		}
		return tmuxSessionUnknown, false
	}
	target := strings.TrimSpace(sessionName)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) == target {
			return tmuxSessionPresent, true
		}
	}
	return tmuxSessionMissing, true
}

func isTmuxSessionMissingMessage(stderr string) bool {
	message := strings.ToLower(strings.TrimSpace(stderr))
	if strings.Contains(message, "can't find session") {
		return true
	}
	if strings.Contains(message, "no server running") {
		return true
	}
	if strings.Contains(message, "no such file or directory") {
		return true
	}
	if strings.Contains(message, "no sessions") {
		return true
	}
	return false
}

func tmuxHasSession(ctx context.Context, sessionName string) bool {
	return tmuxSessionProbe(ctx, sessionName) == tmuxSessionPresent
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
var pullURLNumberRegex = regexp.MustCompile(`/pull/([0-9]+)`)
var pullURLRepoNumberRegex = regexp.MustCompile(`https?://[^/]+/([^/]+/[^/]+)/pull/([0-9]+)`)

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
		if tmuxSessionProbe(ctx, sessionName) == tmuxSessionMissing {
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
